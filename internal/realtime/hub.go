package realtime

import (
	"context"

	"github.com/gorilla/websocket"
)

// Events pushed to connected clients (spec §8).
const (
	EvModuleChanged   = "ModuleChanged"
	EvFileListUpdated = "FileListUpdated"
	EvFormOpened      = "FormOpened"
	EvCountdownTick   = "CountdownTick"
	EvScoreUpdated    = "ScoreUpdated" // Phase 11: broadcast on score upsert; no emitter yet.
)

// WSMessage is one frame pushed to clients.
type WSMessage struct {
	Event   string      `json:"event"`
	Payload interface{} `json:"payload"`
}

// anonymousEvents are the only events an unauthenticated (public page) client receives.
var anonymousEvents = map[string]bool{
	EvCountdownTick: true,
	EvScoreUpdated:  true,
}

// Client is one WebSocket connection. Only the hub goroutine touches the map that holds it;
// send is the single point of contact for the connection's own write pump.
type Client struct {
	conn          *websocket.Conn
	send          chan WSMessage
	authenticated bool
}

// Hub owns the client set in a single goroutine, so no mutex is needed.
type Hub struct {
	clients    map[*Client]bool
	broadcast  chan WSMessage
	register   chan *Client
	unregister chan *Client
	done       chan struct{} // closed when Run returns, so pumps never block on a dead hub
}

func NewHub() *Hub {
	return &Hub{
		clients:    make(map[*Client]bool),
		broadcast:  make(chan WSMessage, 64),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		done:       make(chan struct{}),
	}
}

// Run owns the client map until ctx is done.
func (h *Hub) Run(ctx context.Context) {
	defer close(h.done)
	for {
		select {
		case <-ctx.Done():
			for c := range h.clients {
				close(c.send)
				delete(h.clients, c)
			}
			return

		case c := <-h.register:
			h.clients[c] = true

		case c := <-h.unregister:
			if h.clients[c] {
				delete(h.clients, c)
				close(c.send)
			}

		case msg := <-h.broadcast:
			for c := range h.clients {
				if !c.authenticated && !anonymousEvents[msg.Event] {
					continue
				}
				select {
				case c.send <- msg:
				default:
					// slow or dead client: drop it rather than stall every other client
					delete(h.clients, c)
					close(c.send)
				}
			}
		}
	}
}

// Broadcast queues an event for every eligible client. It never blocks: if the queue is
// full the event is dropped, because a stalled caller (the countdown ticker, a jury POST)
// is worse than a missed tick.
func (h *Hub) Broadcast(event string, payload interface{}) {
	select {
	case h.broadcast <- WSMessage{Event: event, Payload: payload}:
	default:
	}
}
