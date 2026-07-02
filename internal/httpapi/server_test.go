package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"islands/internal/auth"
	"islands/internal/game"
	"islands/internal/realtime"
)

func newTestServer() (*Server, *auth.Manager) {
	hub := realtime.NewHub()
	gameService := game.NewService(hub, realtime.Config{VisibleChunkRadius: 1})
	gameService.SeedDemoWorld(1)
	manager := auth.NewManager("test-secret", time.Hour)
	return NewServer(manager, gameService, hub), manager
}

func TestActionsRejectUnauthenticatedRequests(t *testing.T) {
	server, _ := newTestServer()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/worlds/1/actions", strings.NewReader(`{"action_type":"harvest"}`))
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestStreamRejectsUnauthenticatedRequests(t *testing.T) {
	server, _ := newTestServer()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/worlds/1/stream", nil)
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestLoginDoesNotMintArbitraryActorTokens(t *testing.T) {
	server, _ := newTestServer()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"user_id":1,"actor_id":999,"world_id":1}`))
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestActionsUseActorFromToken(t *testing.T) {
	server, manager := newTestServer()
	token, _, err := manager.Issue(1, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	body := bytes.NewBufferString(`{"action_type":"move","actor_id":999,"x":32,"y":0}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/worlds/1/actions", body)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, body=%s", rec.Code, rec.Body.String())
	}
	var result game.ActionResult
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Actor.ID != 1 {
		t.Fatalf("actor id: got %d, want token actor 1", result.Actor.ID)
	}
	if result.Actor.X != 32 {
		t.Fatalf("actor x: got %d, want 32", result.Actor.X)
	}
}

func TestActionsRejectWrongWorldForToken(t *testing.T) {
	server, manager := newTestServer()
	token, _, err := manager.Issue(1, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/worlds/2/actions", strings.NewReader(`{"action_type":"harvest"}`))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusForbidden)
	}
}
