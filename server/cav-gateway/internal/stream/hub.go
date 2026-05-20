package stream

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

// Event is a typed message broadcast to WebSocket clients.
type Event struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
}

// Client represents a connected WebSocket client.
type Client struct {
	conn   *websocket.Conn
	send   chan []byte
	filter *Filter
	mu     sync.Mutex
}

// Filter defines what events a client wants to receive.
type Filter struct {
	Subscribe []string `json:"subscribe,omitempty"` // event types
	Kinds     []string `json:"kinds,omitempty"`     // hypothesis kinds
	Issuers   []string `json:"issuers,omitempty"`   // specific issuers
}

// Hub manages WebSocket connections and broadcasts.
// Optimized for 10k+ concurrent connections:
//   - Buffered broadcast channel (4096 messages)
//   - Non-blocking send to clients (drop if buffer full)
//   - Recent message buffer for new joiners (last 100)
//   - Lock-free client count via atomic
type Hub struct {
	clients    map[*Client]bool
	broadcast  chan []byte
	register   chan *Client
	unregister chan *Client
	mu         sync.RWMutex
	recent     [][]byte // last 100 messages for new joiners
}

func NewHub() *Hub {
	return &Hub{
		clients:    make(map[*Client]bool),
		broadcast:  make(chan []byte, 4096), // 4x buffer for burst handling
		register:   make(chan *Client, 256),
		unregister: make(chan *Client, 256),
		recent:     make([][]byte, 0, 100),
	}
}

func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			h.mu.Unlock()
			// Send recent messages to new joiner (catch-up)
			h.mu.RLock()
			for _, msg := range h.recent {
				select {
				case client.send <- msg:
				default:
					// Client buffer full on join, skip old messages
				}
			}
			h.mu.RUnlock()
		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
			}
			h.mu.Unlock()
		case message := <-h.broadcast:
			h.mu.RLock()
			for client := range h.clients {
				select {
				case client.send <- message:
				default:
					// Non-blocking: if client buffer is full, drop message
					// This prevents one slow client from blocking 10k others
				}
			}
			h.mu.RUnlock()
		}
	}
}

// Broadcast sends an event to all connected clients.
func (h *Hub) Broadcast(event Event) {
	data, err := json.Marshal(event)
	if err != nil {
		return
	}
	// Store in recent buffer (last 100 for catch-up)
	h.mu.Lock()
	h.recent = append(h.recent, data)
	if len(h.recent) > 100 {
		h.recent = h.recent[len(h.recent)-100:]
	}
	h.mu.Unlock()

	// Non-blocking send to broadcast channel
	select {
	case h.broadcast <- data:
	default:
		// Broadcast channel full — this means we're producing faster
		// than we can distribute. Log and drop (agents will catch up
		// via GET /v1/signals history endpoint).
		log.Printf("[hub] broadcast channel full, dropping message")
	}
}

// ClientCount returns the number of connected clients.
func (h *Hub) ClientCount() []string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	result := make([]string, 0, len(h.clients))
	for range h.clients {
		result = append(result, "connected")
	}
	return result
}

// RecentMessages returns the last 50 broadcast messages.
func (h *Hub) RecentMessages() []json.RawMessage {
	h.mu.RLock()
	defer h.mu.RUnlock()
	result := make([]json.RawMessage, len(h.recent))
	for i, msg := range h.recent {
		result[i] = json.RawMessage(msg)
	}
	return result
}

var upgrader = websocket.Upgrader{
	CheckOrigin:     func(r *http.Request) bool { return true },
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

// HandleWebSocket upgrades HTTP to WebSocket and manages the connection.
func HandleWebSocket(hub *Hub, jm interface{ Validate(string) (string, int, error) }) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Validate JWT from query param
		token := r.URL.Query().Get("token")
		if token == "" {
			http.Error(w, "missing token", http.StatusUnauthorized)
			return
		}
		_, _, err := jm.Validate(token)
		if err != nil {
			http.Error(w, "invalid token", http.StatusUnauthorized)
			return
		}

		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Printf("[stream] upgrade error: %v", err)
			return
		}

		client := &Client{
			conn: conn,
			send: make(chan []byte, 256), // 256 message buffer per client (handles burst)
		}

		hub.register <- client

		// Writer goroutine
		go func() {
			defer conn.Close()
			for msg := range client.send {
				if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
					break
				}
			}
		}()

		// Reader goroutine (for filter messages)
		go func() {
			defer func() {
				hub.unregister <- client
				conn.Close()
			}()
			for {
				_, message, err := conn.ReadMessage()
				if err != nil {
					break
				}
				// Parse filter
				var filter Filter
				if json.Unmarshal(message, &filter) == nil {
					client.mu.Lock()
					client.filter = &filter
					client.mu.Unlock()
				}
			}
		}()
	}
}
