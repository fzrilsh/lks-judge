package realtime

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// newTestClient registers a client with no real connection. Safe because the pumps,
// the only things that touch conn, are never started here.
func newTestClient(t *testing.T, h *Hub, authenticated bool, buffer int) *Client {
	t.Helper()
	c := &Client{send: make(chan WSMessage, buffer), authenticated: authenticated}
	h.register <- c
	return c
}

// recv waits briefly for one frame. ok is false when nothing arrived in time.
func recv(t *testing.T, c *Client) (WSMessage, bool) {
	t.Helper()
	select {
	case msg, open := <-c.send:
		return msg, open
	case <-time.After(time.Second):
		return WSMessage{}, false
	}
}

func TestBroadcastScope(t *testing.T) {
	h := NewHub()
	go h.Run(t.Context())

	auth := newTestClient(t, h, true, 4)
	anon := newTestClient(t, h, false, 4)

	h.Broadcast(EvFormOpened, map[string]bool{"status": true})
	if msg, ok := recv(t, auth); !ok || msg.Event != EvFormOpened {
		t.Fatalf("authenticated client missed FormOpened: %+v ok=%v", msg, ok)
	}

	h.Broadcast(EvCountdownTick, map[string]int{"seconds": 42})
	if msg, ok := recv(t, anon); !ok || msg.Event != EvCountdownTick {
		t.Fatalf("anonymous client got %+v ok=%v, want CountdownTick as the first frame", msg, ok)
	}
	if msg, ok := recv(t, auth); !ok || msg.Event != EvCountdownTick {
		t.Fatalf("authenticated client missed CountdownTick: %+v ok=%v", msg, ok)
	}
}

func TestSlowClientEvicted(t *testing.T) {
	h := NewHub()
	go h.Run(t.Context())

	slow := newTestClient(t, h, true, 1)
	healthy := newTestClient(t, h, true, 8)

	for i := range 3 {
		h.Broadcast(EvCountdownTick, map[string]int{"seconds": i})
	}

	// The healthy client still receives everything: the slow one never blocked the loop.
	for i := range 3 {
		if _, ok := recv(t, healthy); !ok {
			t.Fatalf("healthy client missed frame %d", i)
		}
	}

	// slow's channel is closed once evicted; drain the one buffered frame first.
	<-slow.send
	if _, open := recv(t, slow); open {
		t.Fatal("slow client should have been evicted and its channel closed")
	}
}

func TestUnregisterClosesChannel(t *testing.T) {
	h := NewHub()
	go h.Run(t.Context())

	c := newTestClient(t, h, true, 4)
	h.unregister <- c

	if _, open := recv(t, c); open {
		t.Fatal("unregister should close send")
	}
}

func TestRunStopsOnContextCancel(t *testing.T) {
	h := NewHub()
	ctx, cancel := context.WithCancel(context.Background())
	go h.Run(ctx)

	c := newTestClient(t, h, true, 4)
	cancel()

	if _, open := recv(t, c); open {
		t.Fatal("shutdown should close every client's send")
	}
	select {
	case <-h.done:
	case <-time.After(time.Second):
		t.Fatal("Run did not return after context cancel")
	}
}

// TestServeWSAnonymousScope exercises the real upgrade path end to end.
func TestServeWSAnonymousScope(t *testing.T) {
	h := NewHub()
	go h.Run(t.Context())

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ServeWS(h, false, w, r)
	}))
	defer srv.Close()

	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http"), nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = conn.Close() }()

	// Wait for the hub goroutine to pick up the registration before broadcasting.
	deadline := time.Now().Add(time.Second)
	for len(h.register) > 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	time.Sleep(50 * time.Millisecond)

	h.Broadcast(EvFormOpened, map[string]bool{"status": true}) // must be filtered out
	h.Broadcast(EvCountdownTick, map[string]int{"seconds": 7})

	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var got WSMessage
	if err := conn.ReadJSON(&got); err != nil {
		t.Fatalf("read: %v", err)
	}
	if got.Event != EvCountdownTick {
		t.Fatalf("anonymous connection received %q, want CountdownTick", got.Event)
	}
}
