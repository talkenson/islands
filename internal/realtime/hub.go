package realtime

import (
	"sync"

	"islands/internal/world"
)

type Event struct {
	ID            uint64             `json:"id"`
	Type          string             `json:"type"`
	WorldID       uint64             `json:"world_id"`
	ChangedChunks []world.ChunkCoord `json:"changed_chunks,omitempty"`
	Data          any                `json:"data,omitempty"`
}

type Client struct {
	ID       uint64
	ActorID  uint64
	WorldID  uint64
	Interest map[world.ChunkCoord]struct{}
	Events   <-chan Event

	events chan Event
}

func (c *Client) Close() {
	close(c.events)
}

type Hub struct {
	mu      sync.RWMutex
	nextID  uint64
	clients map[uint64]*Client
	history []Event
	limit   int
	closed  bool
}

func NewHub() *Hub {
	return &Hub{
		clients: make(map[uint64]*Client),
		limit:   256,
	}
}

func (h *Hub) Subscribe(actorID, worldID uint64, interest map[world.ChunkCoord]struct{}) *Client {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.nextID++
	events := make(chan Event, 32)
	client := &Client{
		ID:       h.nextID,
		ActorID:  actorID,
		WorldID:  worldID,
		Interest: copyInterest(interest),
		Events:   events,
		events:   events,
	}
	if h.closed {
		close(events)
		return client
	}
	h.clients[client.ID] = client
	return client
}

func (h *Hub) Unsubscribe(clientID uint64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	client, ok := h.clients[clientID]
	if !ok {
		return
	}
	delete(h.clients, clientID)
	close(client.events)
}

func (h *Hub) SetActorInterest(worldID, actorID uint64, interest map[world.ChunkCoord]struct{}) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, client := range h.clients {
		if client.WorldID == worldID && client.ActorID == actorID {
			client.Interest = copyInterest(interest)
		}
	}
}

func (h *Hub) Publish(event Event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return
	}

	h.history = append(h.history, event)
	if len(h.history) > h.limit {
		h.history = h.history[len(h.history)-h.limit:]
	}

	for _, client := range h.clients {
		if client.WorldID != event.WorldID || !Intersects(client.Interest, event.ChangedChunks) {
			continue
		}
		select {
		case client.events <- event:
		default:
		}
	}
}

func (h *Hub) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return
	}
	h.closed = true
	for id, client := range h.clients {
		delete(h.clients, id)
		close(client.events)
	}
}

func (h *Hub) Replay(client *Client, afterID uint64) []Event {
	h.mu.RLock()
	defer h.mu.RUnlock()
	events := make([]Event, 0)
	for _, event := range h.history {
		if event.ID > afterID && event.WorldID == client.WorldID && Intersects(client.Interest, event.ChangedChunks) {
			events = append(events, event)
		}
	}
	return events
}

func copyInterest(src map[world.ChunkCoord]struct{}) map[world.ChunkCoord]struct{} {
	dst := make(map[world.ChunkCoord]struct{}, len(src))
	for coord := range src {
		dst[coord] = struct{}{}
	}
	return dst
}
