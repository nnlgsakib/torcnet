package rpc

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/torcnet/torcnet/pkg/core"
)

// NewRPCError creates a new RPC error
func NewRPCError(code int, message string, data interface{}) *RPCError {
	return &RPCError{
		Code:    code,
		Message: message,
		Data:    data,
	}
}

// WebSocketServer handles WebSocket connections for real-time RPC
type WebSocketServer struct {
	upgrader    websocket.Upgrader
	connections map[string]*WSConnection
	methods     map[string]interface{}
	mutex       sync.RWMutex
	addr        string
	blockchain  *core.Blockchain

	// Event subscription management
	subscriptions map[string]*Subscription
	subMutex      sync.RWMutex
	nextSubID     uint64
}

// WSConnection represents a WebSocket connection
type WSConnection struct {
	conn          *websocket.Conn
	id            string
	send          chan []byte
	subscriptions map[string]*Subscription
	mutex         sync.RWMutex
	ctx           context.Context
	cancel        context.CancelFunc
}

// Subscription represents an event subscription
type Subscription struct {
	ID      string                 `json:"id"`
	Type    string                 `json:"type"`
	Params  map[string]interface{} `json:"params"`
	Conn    *WSConnection          `json:"-"`
	Active  bool                   `json:"active"`
	Created time.Time              `json:"created"`
}

// WSRequest represents a WebSocket RPC request
type WSRequest struct {
	ID     interface{}   `json:"id"`
	Method string        `json:"method"`
	Params []interface{} `json:"params"`
}

// RPCError represents an RPC error
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// WSResponse represents a WebSocket RPC response
type WSResponse struct {
	ID     interface{} `json:"id"`
	Result interface{} `json:"result,omitempty"`
	Error  *RPCError   `json:"error,omitempty"`
}

// WSNotification represents a WebSocket notification (subscription event)
type WSNotification struct {
	Method string      `json:"method"`
	Params interface{} `json:"params"`
}

// SubscriptionEvent represents an event sent to subscribers
type SubscriptionEvent struct {
	Subscription string      `json:"subscription"`
	Result       interface{} `json:"result"`
}

// NewWebSocketServer creates a new WebSocket RPC server
func NewWebSocketServer(addr string, blockchain *core.Blockchain) *WebSocketServer {
	return &WebSocketServer{
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true // Allow all origins in development
			},
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
		},
		addr:          addr,
		connections:   make(map[string]*WSConnection),
		methods:       make(map[string]interface{}),
		subscriptions: make(map[string]*Subscription),
		blockchain:    blockchain,
	}
}

// RegisterMethod registers a method for WebSocket RPC calls
func (ws *WebSocketServer) RegisterMethod(name string, handler interface{}) {
	ws.mutex.Lock()
	defer ws.mutex.Unlock()
	ws.methods[name] = handler
}

// HandleWebSocket handles WebSocket upgrade and connection
func (ws *WebSocketServer) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := ws.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade failed: %v", err)
		return
	}

	// Create connection context
	ctx, cancel := context.WithCancel(context.Background())

	wsConn := &WSConnection{
		conn:          conn,
		id:            generateConnectionID(),
		send:          make(chan []byte, 256),
		subscriptions: make(map[string]*Subscription),
		ctx:           ctx,
		cancel:        cancel,
	}

	// Register connection
	ws.mutex.Lock()
	ws.connections[wsConn.id] = wsConn
	ws.mutex.Unlock()

	// Start connection handlers
	go wsConn.writePump()
	go wsConn.readPump(ws)

	log.Printf("WebSocket connection established: %s", wsConn.id)
}

// readPump handles incoming WebSocket messages
func (conn *WSConnection) readPump(server *WebSocketServer) {
	defer func() {
		conn.cleanup(server)
		conn.conn.Close()
	}()

	conn.conn.SetReadLimit(512)
	conn.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	conn.conn.SetPongHandler(func(string) error {
		conn.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		select {
		case <-conn.ctx.Done():
			return
		default:
			_, message, err := conn.conn.ReadMessage()
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					log.Printf("WebSocket error: %v", err)
				}
				return
			}

			// Process RPC request
			go conn.handleMessage(server, message)
		}
	}
}

// writePump handles outgoing WebSocket messages
func (conn *WSConnection) writePump() {
	ticker := time.NewTicker(54 * time.Second)
	defer func() {
		ticker.Stop()
		conn.conn.Close()
	}()

	for {
		select {
		case <-conn.ctx.Done():
			return
		case message, ok := <-conn.send:
			conn.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				conn.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			if err := conn.conn.WriteMessage(websocket.TextMessage, message); err != nil {
				log.Printf("WebSocket write error: %v", err)
				return
			}

		case <-ticker.C:
			conn.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := conn.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// handleMessage processes incoming WebSocket messages
func (conn *WSConnection) handleMessage(server *WebSocketServer, message []byte) {
	var req WSRequest
	if err := json.Unmarshal(message, &req); err != nil {
		conn.sendError(nil, NewRPCError(-32700, "Parse error", nil))
		return
	}

	// Handle subscription methods
	switch req.Method {
	case "eth_subscribe":
		conn.handleSubscribe(server, &req)
		return
	case "eth_unsubscribe":
		conn.handleUnsubscribe(server, &req)
		return
	}

	// Handle regular RPC methods
	server.mutex.RLock()
	handler, exists := server.methods[req.Method]
	server.mutex.RUnlock()

	if !exists {
		conn.sendError(req.ID, NewRPCError(-32601, "Method not found", nil))
		return
	}

	// Execute method (simplified - in production, use reflection for proper parameter handling)
	result, err := conn.executeMethod(handler, req.Params)
	if err != nil {
		conn.sendError(req.ID, NewRPCError(-32603, "Internal error", err.Error()))
		return
	}

	conn.sendResult(req.ID, result)
}

// handleSubscribe handles subscription requests
func (conn *WSConnection) handleSubscribe(server *WebSocketServer, req *WSRequest) {
	if len(req.Params) == 0 {
		conn.sendError(req.ID, NewRPCError(-32602, "Invalid params", "subscription type required"))
		return
	}

	subType, ok := req.Params[0].(string)
	if !ok {
		conn.sendError(req.ID, NewRPCError(-32602, "Invalid params", "subscription type must be string"))
		return
	}

	// Generate subscription ID
	server.subMutex.Lock()
	server.nextSubID++
	subID := fmt.Sprintf("0x%x", server.nextSubID)
	server.subMutex.Unlock()

	// Parse subscription parameters
	var params map[string]interface{}
	if len(req.Params) > 1 {
		if p, ok := req.Params[1].(map[string]interface{}); ok {
			params = p
		}
	}

	// Create subscription
	sub := &Subscription{
		ID:      subID,
		Type:    subType,
		Params:  params,
		Conn:    conn,
		Active:  true,
		Created: time.Now(),
	}

	// Register subscription
	server.subMutex.Lock()
	server.subscriptions[subID] = sub
	server.subMutex.Unlock()

	conn.mutex.Lock()
	conn.subscriptions[subID] = sub
	conn.mutex.Unlock()

	// Send subscription ID back to client
	conn.sendResult(req.ID, subID)

	log.Printf("Created subscription %s for type %s", subID, subType)
}

// handleUnsubscribe handles unsubscription requests
func (conn *WSConnection) handleUnsubscribe(server *WebSocketServer, req *WSRequest) {
	if len(req.Params) == 0 {
		conn.sendError(req.ID, NewRPCError(-32602, "Invalid params", "subscription ID required"))
		return
	}

	subID, ok := req.Params[0].(string)
	if !ok {
		conn.sendError(req.ID, NewRPCError(-32602, "Invalid params", "subscription ID must be string"))
		return
	}

	// Remove subscription
	server.subMutex.Lock()
	if sub, exists := server.subscriptions[subID]; exists {
		if sub.Conn.id == conn.id {
			delete(server.subscriptions, subID)
			sub.Active = false
		}
	}
	server.subMutex.Unlock()

	conn.mutex.Lock()
	delete(conn.subscriptions, subID)
	conn.mutex.Unlock()

	conn.sendResult(req.ID, true)
	log.Printf("Removed subscription %s", subID)
}

// executeMethod executes a method handler (simplified implementation)
func (conn *WSConnection) executeMethod(handler interface{}, params []interface{}) (interface{}, error) {
	// This is a simplified implementation
	// In production, use reflection to properly call methods with correct parameters

	// For now, assume handler is a function that takes no parameters
	if fn, ok := handler.(func() (string, error)); ok {
		return fn()
	}

	// Handle methods with parameters (add more cases as needed)
	if fn, ok := handler.(func(string) (string, error)); ok && len(params) > 0 {
		if param, ok := params[0].(string); ok {
			return fn(param)
		}
	}

	return nil, fmt.Errorf("unsupported method signature")
}

// sendResult sends a successful result to the client
func (conn *WSConnection) sendResult(id interface{}, result interface{}) {
	response := WSResponse{
		ID:     id,
		Result: result,
	}
	conn.sendResponse(response)
}

// sendError sends an error response to the client
func (conn *WSConnection) sendError(id interface{}, err *RPCError) {
	response := WSResponse{
		ID:    id,
		Error: err,
	}
	conn.sendResponse(response)
}

// sendResponse sends a response to the client
func (conn *WSConnection) sendResponse(response WSResponse) {
	data, err := json.Marshal(response)
	if err != nil {
		log.Printf("Failed to marshal response: %v", err)
		return
	}

	select {
	case conn.send <- data:
	case <-conn.ctx.Done():
	default:
		// Channel is full, close connection
		conn.cancel()
	}
}

// SendNotification sends a notification to subscribers
func (server *WebSocketServer) SendNotification(subType string, event interface{}) {
	server.subMutex.RLock()
	defer server.subMutex.RUnlock()

	for _, sub := range server.subscriptions {
		if sub.Active && sub.Type == subType {
			notification := WSNotification{
				Method: "eth_subscription",
				Params: SubscriptionEvent{
					Subscription: sub.ID,
					Result:       event,
				},
			}

			data, err := json.Marshal(notification)
			if err != nil {
				log.Printf("Failed to marshal notification: %v", err)
				continue
			}

			select {
			case sub.Conn.send <- data:
			case <-sub.Conn.ctx.Done():
				// Connection is closed, mark subscription as inactive
				sub.Active = false
			default:
				// Channel is full, skip this notification
				log.Printf("Skipping notification for subscription %s (channel full)", sub.ID)
			}
		}
	}
}

// cleanup removes connection and its subscriptions
func (conn *WSConnection) cleanup(server *WebSocketServer) {
	// Remove connection
	server.mutex.Lock()
	delete(server.connections, conn.id)
	server.mutex.Unlock()

	// Remove all subscriptions for this connection
	server.subMutex.Lock()
	for subID, sub := range server.subscriptions {
		if sub.Conn.id == conn.id {
			delete(server.subscriptions, subID)
		}
	}
	server.subMutex.Unlock()

	// Cancel context
	conn.cancel()

	log.Printf("WebSocket connection closed: %s", conn.id)
}

// GetActiveConnections returns the number of active connections
func (server *WebSocketServer) GetActiveConnections() int {
	server.mutex.RLock()
	defer server.mutex.RUnlock()
	return len(server.connections)
}

// GetActiveSubscriptions returns the number of active subscriptions
func (server *WebSocketServer) GetActiveSubscriptions() int {
	server.subMutex.RLock()
	defer server.subMutex.RUnlock()

	count := 0
	for _, sub := range server.subscriptions {
		if sub.Active {
			count++
		}
	}
	return count
}

// BroadcastNewBlock broadcasts new block events to subscribers
func (server *WebSocketServer) BroadcastNewBlock(block interface{}) {
	server.SendNotification("newHeads", block)
}

// BroadcastNewTransaction broadcasts new transaction events to subscribers
func (server *WebSocketServer) BroadcastNewTransaction(tx interface{}) {
	server.SendNotification("newPendingTransactions", tx)
}

// BroadcastLogs broadcasts log events to subscribers
func (server *WebSocketServer) BroadcastLogs(logs interface{}) {
	server.SendNotification("logs", logs)
}

// Helper function to generate connection IDs
func generateConnectionID() string {
	return fmt.Sprintf("conn_%d", time.Now().UnixNano())
}

// Start starts the WebSocket server on the specified address
func (server *WebSocketServer) Start() error {
	http.HandleFunc("/ws", server.HandleWebSocket)

	log.Printf("WebSocket RPC server starting on %s", server.addr)
	return http.ListenAndServe(server.addr, nil)
}

// RegisterEVMWebSocketMethods registers EVM RPC methods for WebSocket access
func RegisterEVMWebSocketMethods(server *WebSocketServer, evmService *EVMRPCService) {
	// Register all EVM RPC methods
	server.RegisterMethod("eth_chainId", evmService.EthChainId)
	server.RegisterMethod("eth_blockNumber", evmService.EthBlockNumber)
	server.RegisterMethod("eth_getBalance", evmService.EthGetBalance)
	server.RegisterMethod("eth_getTransactionCount", evmService.EthGetTransactionCount)
	server.RegisterMethod("eth_getCode", evmService.EthGetCode)
	server.RegisterMethod("eth_call", evmService.EthCall)
	server.RegisterMethod("eth_sendTransaction", evmService.EthSendTransaction)
	server.RegisterMethod("eth_estimateGas", evmService.EthEstimateGas)
	server.RegisterMethod("eth_getTransactionReceipt", evmService.EthGetTransactionReceipt)
	server.RegisterMethod("eth_getStorageAt", evmService.EthGetStorageAt)
	server.RegisterMethod("eth_gasPrice", evmService.EthGasPrice)
	server.RegisterMethod("eth_getLogs", evmService.EthGetLogs)
	server.RegisterMethod("net_version", evmService.NetVersion)
	server.RegisterMethod("web3_clientVersion", evmService.Web3ClientVersion)
	server.RegisterMethod("web3_sha3", evmService.Web3Sha3)
}