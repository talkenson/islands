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
	"islands/internal/world"
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

func TestLoginReturnsDemoActorPosition(t *testing.T) {
	server, _ := newTestServer()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"user_id":1,"actor_id":1,"world_id":1}`))
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, body=%s", rec.Code, rec.Body.String())
	}
	var result loginResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Actors) != 1 {
		t.Fatalf("actors count: got %d, want 1", len(result.Actors))
	}
	if result.Actors[0].X != game.DemoActorStartX || result.Actors[0].Y != game.DemoActorStartY {
		t.Fatalf("actor position: got %d,%d want %d,%d", result.Actors[0].X, result.Actors[0].Y, game.DemoActorStartX, game.DemoActorStartY)
	}
	if result.Actors[0].InventoryID == 0 {
		t.Fatalf("actor inventory id was not returned")
	}
}

func TestActionsUseActorFromToken(t *testing.T) {
	server, manager := newTestServer()
	token, _, err := manager.Issue(1, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	body := bytes.NewBufferString(`{"action_type":"move","actor_id":999,"x":901,"y":1900}`)
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
	if !result.Accepted {
		t.Fatalf("action was not accepted")
	}
	if result.EventID == 0 {
		t.Fatalf("event id was not returned")
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

func TestWriteSSEOmitsRoutingMetadataFromData(t *testing.T) {
	rec := httptest.NewRecorder()
	err := writeSSE(rec, realtime.Event{
		ID:            7,
		Type:          "entity_patch",
		WorldID:       1,
		ChangedChunks: []world.ChunkCoord{{X: 1, Y: 2}},
		Data:          map[string]any{"ok": true},
	})
	if err != nil {
		t.Fatal(err)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "id: 7\n") || !strings.Contains(body, "event: entity_patch\n") {
		t.Fatalf("sse headers missing: %q", body)
	}
	if !strings.Contains(body, `data: {"ok":true}`) {
		t.Fatalf("sse data payload: %q", body)
	}
	if strings.Contains(body, "changed_chunks") || strings.Contains(body, "world_id") || strings.Contains(body, `"type"`) {
		t.Fatalf("sse leaked routing metadata: %q", body)
	}
}
