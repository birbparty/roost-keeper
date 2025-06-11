package websocket

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

// Hub manages WebSocket connections and message broadcasting
type Hub struct {
	// Connected clients
	clients map[*Client]bool

	// Channel for registering clients
	register chan *Client

	// Channel for unregistering clients
	unregister chan *Client

	// Channel for broadcasting messages
	broadcast chan Message

	// Logger instance
	logger *zap.Logger

	// Mutex for thread-safe operations
	mutex sync.RWMutex

	// Stop channel for graceful shutdown
	stopCh chan struct{}

	// Client connection settings
	upgrader websocket.Upgrader
}

// Client represents a WebSocket client connection
type Client struct {
	// The websocket connection
	conn *websocket.Conn

	// Buffered channel of outbound messages
	send chan Message

	// Hub instance
	hub *Hub

	// User information
	userID   string
	username string
	groups   []string

	// Subscription filters
	subscriptions map[string]bool

	// Connection metadata
	clientIP    string
	userAgent   string
	connectedAt time.Time

	// Graceful shutdown
	done chan struct{}
}

// Message represents a WebSocket message
type Message struct {
	Type      string                 `json:"type"`
	Data      interface{}            `json:"data"`
	Timestamp time.Time              `json:"timestamp"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// NewHub creates a new WebSocket hub
func NewHub(logger *zap.Logger) *Hub {
	return &Hub{
		clients:    make(map[*Client]bool),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		broadcast:  make(chan Message, 256),
		logger:     logger,
		stopCh:     make(chan struct{}),
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin: func(r *http.Request) bool {
				// In production, implement proper CORS checking
				return true
			},
		},
	}
}

// Run starts the hub's main event loop
func (h *Hub) Run(ctx context.Context) {
	h.logger.Info("Starting WebSocket hub")

	defer func() {
		h.logger.Info("WebSocket hub stopped")
	}()

	for {
		select {
		case client := <-h.register:
			h.registerClient(client)

		case client := <-h.unregister:
			h.unregisterClient(client)

		case message := <-h.broadcast:
			h.broadcastMessage(message)

		case <-ctx.Done():
			h.logger.Info("WebSocket hub context cancelled")
			return

		case <-h.stopCh:
			h.logger.Info("WebSocket hub stop requested")
			return
		}
	}
}

// Stop gracefully stops the hub
func (h *Hub) Stop() {
	h.logger.Info("Stopping WebSocket hub")
	close(h.stopCh)

	// Close all client connections
	h.mutex.Lock()
	defer h.mutex.Unlock()

	for client := range h.clients {
		close(client.send)
		client.conn.Close()
	}
}

// Broadcast sends a message to all connected clients
func (h *Hub) Broadcast(message Message) {
	message.Timestamp = time.Now()
	select {
	case h.broadcast <- message:
	default:
		h.logger.Warn("Broadcast channel full, dropping message")
	}
}

// BroadcastToUser sends a message to a specific user
func (h *Hub) BroadcastToUser(userID string, message Message) {
	message.Timestamp = time.Now()
	h.mutex.RLock()
	defer h.mutex.RUnlock()

	for client := range h.clients {
		if client.userID == userID {
			select {
			case client.send <- message:
			default:
				h.logger.Warn("Client send channel full",
					zap.String("user_id", userID))
				close(client.send)
				delete(h.clients, client)
			}
		}
	}
}

// BroadcastToGroup sends a message to all users in a group
func (h *Hub) BroadcastToGroup(group string, message Message) {
	message.Timestamp = time.Now()
	h.mutex.RLock()
	defer h.mutex.RUnlock()

	for client := range h.clients {
		for _, clientGroup := range client.groups {
			if clientGroup == group {
				select {
				case client.send <- message:
				default:
					h.logger.Warn("Client send channel full",
						zap.String("user_id", client.userID),
						zap.String("group", group))
					close(client.send)
					delete(h.clients, client)
				}
				break
			}
		}
	}
}

// GetStats returns statistics about the hub
func (h *Hub) GetStats() map[string]interface{} {
	h.mutex.RLock()
	defer h.mutex.RUnlock()

	return map[string]interface{}{
		"connected_clients": len(h.clients),
		"broadcast_backlog": len(h.broadcast),
	}
}

// registerClient handles client registration
func (h *Hub) registerClient(client *Client) {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	h.clients[client] = true
	h.logger.Info("WebSocket client connected",
		zap.String("user_id", client.userID),
		zap.String("username", client.username),
		zap.String("client_ip", client.clientIP),
		zap.Int("total_clients", len(h.clients)))

	// Send welcome message
	welcomeMsg := Message{
		Type: "welcome",
		Data: map[string]interface{}{
			"message":      "Connected to Roost-Keeper WebSocket",
			"user_id":      client.userID,
			"connected_at": client.connectedAt,
		},
		Timestamp: time.Now(),
	}

	select {
	case client.send <- welcomeMsg:
	default:
		close(client.send)
		delete(h.clients, client)
	}
}

// unregisterClient handles client disconnection
func (h *Hub) unregisterClient(client *Client) {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	if _, ok := h.clients[client]; ok {
		delete(h.clients, client)
		close(client.send)
		h.logger.Info("WebSocket client disconnected",
			zap.String("user_id", client.userID),
			zap.String("username", client.username),
			zap.Int("total_clients", len(h.clients)))
	}
}

// broadcastMessage sends a message to all appropriate clients
func (h *Hub) broadcastMessage(message Message) {
	h.mutex.RLock()
	defer h.mutex.RUnlock()

	h.logger.Debug("Broadcasting message",
		zap.String("type", message.Type),
		zap.Int("client_count", len(h.clients)))

	for client := range h.clients {
		// Check if client is subscribed to this message type
		if client.isSubscribed(message.Type) {
			select {
			case client.send <- message:
			default:
				h.logger.Warn("Client send channel full, removing client",
					zap.String("user_id", client.userID))
				close(client.send)
				delete(h.clients, client)
			}
		}
	}
}

// NewClient creates a new WebSocket client
func NewClient(conn *websocket.Conn, hub *Hub, userID, username string, groups []string, clientIP, userAgent string) *Client {
	return &Client{
		conn:          conn,
		hub:           hub,
		send:          make(chan Message, 256),
		userID:        userID,
		username:      username,
		groups:        groups,
		subscriptions: make(map[string]bool),
		clientIP:      clientIP,
		userAgent:     userAgent,
		connectedAt:   time.Now(),
		done:          make(chan struct{}),
	}
}

// Subscribe adds a subscription for message types
func (c *Client) Subscribe(messageTypes ...string) {
	for _, msgType := range messageTypes {
		c.subscriptions[msgType] = true
	}
}

// Unsubscribe removes subscription for message types
func (c *Client) Unsubscribe(messageTypes ...string) {
	for _, msgType := range messageTypes {
		delete(c.subscriptions, msgType)
	}
}

// isSubscribed checks if client is subscribed to a message type
func (c *Client) isSubscribed(messageType string) bool {
	// If no subscriptions, receive all messages
	if len(c.subscriptions) == 0 {
		return true
	}
	return c.subscriptions[messageType]
}

// readPump handles reading messages from the WebSocket connection
func (c *Client) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()

	// Set read deadline and pong handler
	c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				c.hub.logger.Error("WebSocket read error",
					zap.String("user_id", c.userID),
					zap.Error(err))
			}
			break
		}

		// Handle incoming message (subscription management, etc.)
		c.handleIncomingMessage(message)
	}
}

// writePump handles writing messages to the WebSocket connection
func (c *Client) writePump() {
	ticker := time.NewTicker(54 * time.Second)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			// Send the message
			if err := c.conn.WriteJSON(message); err != nil {
				c.hub.logger.Error("WebSocket write error",
					zap.String("user_id", c.userID),
					zap.Error(err))
				return
			}

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}

		case <-c.done:
			return
		}
	}
}

// handleIncomingMessage processes messages received from the client
func (c *Client) handleIncomingMessage(data []byte) {
	var msg struct {
		Type string                 `json:"type"`
		Data map[string]interface{} `json:"data"`
	}

	if err := json.Unmarshal(data, &msg); err != nil {
		c.hub.logger.Warn("Invalid message format",
			zap.String("user_id", c.userID),
			zap.Error(err))
		return
	}

	switch msg.Type {
	case "subscribe":
		if topics, ok := msg.Data["topics"].([]interface{}); ok {
			for _, topic := range topics {
				if topicStr, ok := topic.(string); ok {
					c.Subscribe(topicStr)
				}
			}
		}

	case "unsubscribe":
		if topics, ok := msg.Data["topics"].([]interface{}); ok {
			for _, topic := range topics {
				if topicStr, ok := topic.(string); ok {
					c.Unsubscribe(topicStr)
				}
			}
		}

	case "ping":
		// Respond with pong
		pongMsg := Message{
			Type:      "pong",
			Data:      map[string]interface{}{"timestamp": time.Now()},
			Timestamp: time.Now(),
		}
		select {
		case c.send <- pongMsg:
		default:
		}

	default:
		c.hub.logger.Debug("Unknown message type",
			zap.String("user_id", c.userID),
			zap.String("type", msg.Type))
	}
}

// Start begins the client's read and write pumps
func (c *Client) Start() {
	// Register client with hub
	c.hub.register <- c

	// Start pumps in separate goroutines
	go c.writePump()
	go c.readPump()
}

// Stop gracefully stops the client
func (c *Client) Stop() {
	close(c.done)
}
