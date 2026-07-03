package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"islands/internal/auth"
	"islands/internal/game"
	"islands/internal/mapgen"
	"islands/internal/realtime"
)

type Server struct {
	auth *auth.Manager
	game *game.Service
	hub  *realtime.Hub
	mux  *http.ServeMux
}

func NewServer(authManager *auth.Manager, gameService *game.Service, hub *realtime.Hub) *Server {
	s := &Server{
		auth: authManager,
		game: gameService,
		hub:  hub,
		mux:  http.NewServeMux(),
	}
	s.routes()
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func (s *Server) routes() {
	s.mux.HandleFunc("POST /api/v1/auth/login", s.login)
	protected := auth.Middleware(s.auth)
	s.mux.Handle("GET /api/v1/worlds/{worldID}/stream", protected(http.HandlerFunc(s.stream)))
	s.mux.Handle("POST /api/v1/worlds/{worldID}/actions", protected(http.HandlerFunc(s.actions)))
	s.mux.Handle("GET /assets/", http.StripPrefix("/assets/", http.FileServer(http.Dir("assets"))))
	s.mux.Handle("GET /", http.FileServer(http.Dir("client/dist")))
}

type loginRequest struct {
	UserID  uint64 `json:"user_id"`
	ActorID uint64 `json:"actor_id"`
	WorldID uint64 `json:"world_id"`
}

type loginResponse struct {
	Token  string     `json:"token"`
	UserID uint64     `json:"user_id"`
	Actors []actorRef `json:"actors"`
	Worlds []worldRef `json:"worlds"`
}

type actorRef struct {
	ID      uint64 `json:"id"`
	WorldID uint64 `json:"world_id"`
	X       int32  `json:"x"`
	Y       int32  `json:"y"`
}

type worldRef struct {
	ID uint64 `json:"id"`
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}
	if req.UserID == 0 {
		req.UserID = 1
	}
	if req.ActorID == 0 {
		req.ActorID = 1
	}
	if req.WorldID == 0 {
		req.WorldID = 1
	}
	if req.UserID != 1 || req.ActorID != 1 || req.WorldID != 1 {
		auth.WriteError(w, http.StatusForbidden, "forbidden", "demo login only grants actor 1 in world 1")
		return
	}
	token, _, err := s.auth.Issue(req.UserID, req.ActorID, req.WorldID)
	if err != nil {
		auth.WriteError(w, http.StatusInternalServerError, "conflict", err.Error())
		return
	}
	act, err := s.game.Actor(r.Context(), req.WorldID, req.ActorID)
	if err != nil {
		writeGameError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, loginResponse{
		Token:  token,
		UserID: req.UserID,
		Actors: []actorRef{{ID: req.ActorID, WorldID: req.WorldID, X: act.X, Y: act.Y}},
		Worlds: []worldRef{{ID: req.WorldID}},
	})
}

func (s *Server) stream(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.FromContext(r.Context())
	if !ok {
		auth.WriteError(w, http.StatusUnauthorized, "unauthorized", "missing auth context")
		return
	}
	worldID, err := pathWorldID(r)
	if err != nil {
		auth.WriteError(w, http.StatusNotFound, "not_found", "world route not found")
		return
	}
	if err := auth.RequireWorld(claims, worldID); err != nil {
		auth.WriteError(w, http.StatusForbidden, "forbidden", err.Error())
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		auth.WriteError(w, http.StatusInternalServerError, "conflict", "streaming is not supported")
		return
	}
	interest, err := s.game.VisibleChunksForActor(r.Context(), worldID, claims.ActorID)
	if err != nil {
		writeGameError(w, err)
		return
	}
	act, err := s.game.Actor(r.Context(), worldID, claims.ActorID)
	if err != nil {
		writeGameError(w, err)
		return
	}
	client := s.hub.Subscribe(claims.ActorID, worldID, interest)
	defer s.hub.Unsubscribe(client.ID)

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	_ = writeSSE(w, realtime.Event{Type: "hello", WorldID: worldID, Data: map[string]any{
		"actor_id":      claims.ActorID,
		"world_id":      worldID,
		"actor":         act,
		"render_config": mapgen.DefaultRenderConfig(s.game.WorldRenderSeed(worldID)),
	}})
	for _, snapshot := range s.game.ChunkSnapshots(r.Context(), worldID, interest) {
		_ = writeSSE(w, realtime.Event{
			Type:    "chunk_snapshot",
			WorldID: worldID,
			Data:    snapshot,
		})
	}
	if afterID := lastEventID(r); afterID > 0 {
		for _, event := range s.hub.Replay(client, afterID) {
			_ = writeSSE(w, event)
		}
	}
	flusher.Flush()

	for {
		select {
		case <-r.Context().Done():
			return
		case event, ok := <-client.Events:
			if !ok {
				return
			}
			if err := writeSSE(w, event); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func (s *Server) actions(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.FromContext(r.Context())
	if !ok {
		auth.WriteError(w, http.StatusUnauthorized, "unauthorized", "missing auth context")
		return
	}
	worldID, err := pathWorldID(r)
	if err != nil {
		auth.WriteError(w, http.StatusNotFound, "not_found", "world route not found")
		return
	}
	if err := auth.RequireWorld(claims, worldID); err != nil {
		auth.WriteError(w, http.StatusForbidden, "forbidden", err.Error())
		return
	}
	var req game.ActionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		auth.WriteError(w, http.StatusBadRequest, "invalid_action", "invalid action payload")
		return
	}
	result, err := s.game.ApplyAction(r.Context(), worldID, claims.ActorID, req)
	if err != nil {
		writeGameError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func pathWorldID(r *http.Request) (uint64, error) {
	raw := r.PathValue("worldID")
	id, err := strconv.ParseUint(raw, 10, 64)
	if err != nil || id == 0 {
		return 0, fmt.Errorf("invalid world id %q", raw)
	}
	return id, nil
}

func lastEventID(r *http.Request) uint64 {
	raw := strings.TrimSpace(r.Header.Get("Last-Event-ID"))
	if raw == "" {
		return 0
	}
	id, _ := strconv.ParseUint(raw, 10, 64)
	return id
}

func writeGameError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, game.ErrForbidden):
		auth.WriteError(w, http.StatusForbidden, "forbidden", "forbidden")
	case errors.Is(err, game.ErrNotVisible):
		auth.WriteError(w, http.StatusForbidden, "not_visible", "target is not visible")
	case errors.Is(err, game.ErrInvalidAction):
		auth.WriteError(w, http.StatusBadRequest, "invalid_action", "invalid action")
	case errors.Is(err, game.ErrConflict):
		auth.WriteError(w, http.StatusConflict, "conflict", "action conflicts with current state")
	default:
		auth.WriteError(w, http.StatusInternalServerError, "conflict", err.Error())
	}
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeSSE(w http.ResponseWriter, event realtime.Event) error {
	if event.ID != 0 {
		if _, err := fmt.Fprintf(w, "id: %d\n", event.ID); err != nil {
			return err
		}
	}
	if event.Type != "" {
		if _, err := fmt.Fprintf(w, "event: %s\n", event.Type); err != nil {
			return err
		}
	}
	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "data: %s\n\n", payload)
	return err
}
