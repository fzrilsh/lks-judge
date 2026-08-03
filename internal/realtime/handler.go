package realtime

import (
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

const (
	writeWait  = 5 * time.Second  // spec §11a: a write that stalls this long means a dead client
	pongWait   = 60 * time.Second // no pong within this window and the read pump gives up
	pingPeriod = 30 * time.Second
	sendBuffer = 32
)

// upgrader accepts any origin: this server only ever runs on a closed competition LAN.
var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

// ServeWS upgrades the request and attaches the connection to the hub. The caller decides
// authenticated (this package must stay free of store and session logic).
func ServeWS(h *Hub, authenticated bool, w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("ws upgrade: %v", err)
		return
	}

	c := &Client{
		conn:          conn,
		send:          make(chan WSMessage, sendBuffer),
		authenticated: authenticated,
	}
	select {
	case h.register <- c:
	case <-h.done: // hub already stopped
		_ = conn.Close()
		return
	}

	go c.writePump()
	go c.readPump(h)
}

// writePump owns every write on the connection, including pings.
func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		_ = c.conn.Close()
	}()

	for {
		select {
		case msg, ok := <-c.send:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok { // hub closed the channel: evicted or shutting down
				_ = c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.conn.WriteJSON(msg); err != nil {
				return
			}

		case <-ticker.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// readPump exists to notice pongs and closes. Clients never send commands, so frames are discarded.
func (c *Client) readPump(h *Hub) {
	defer func() {
		select {
		case h.unregister <- c:
		case <-h.done:
		}
		_ = c.conn.Close()
	}()

	c.conn.SetReadLimit(512)
	_ = c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		return c.conn.SetReadDeadline(time.Now().Add(pongWait))
	})

	for {
		if _, _, err := c.conn.ReadMessage(); err != nil {
			return
		}
	}
}
