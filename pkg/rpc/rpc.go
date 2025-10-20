package rpc

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/torcnet/torcnet/pkg/core"
	"github.com/torcnet/torcnet/pkg/network"
)

// RPCServer represents the RPC server
type RPCServer struct {
	listenAddr string
	blockchain *core.Blockchain
	p2pNode    *network.P2PNode
	router     *mux.Router
	methods    map[string]interface{}
}

// NewRPCServer creates a new RPC server
func NewRPCServer(listenAddr string, blockchain *core.Blockchain, p2pNode *network.P2PNode) *RPCServer {
	router := mux.NewRouter()
	
	server := &RPCServer{
		listenAddr: listenAddr,
		blockchain: blockchain,
		p2pNode:    p2pNode,
		router:     router,
		methods:    make(map[string]interface{}),
	}
	
	// Register routes
	server.registerRoutes()
	
	return server
}

// RegisterMethod registers a method for RPC calls
func (s *RPCServer) RegisterMethod(name string, handler interface{}) {
	s.methods[name] = handler
}

// Start starts the RPC server
func (s *RPCServer) Start() error {
	fmt.Printf("Starting RPC server on %s\n", s.listenAddr)
	
	server := &http.Server{
		Addr:         s.listenAddr,
		Handler:      s.router,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}
	
	// Start server in a goroutine
	go func() {
		if err := server.ListenAndServe(); err != nil {
			fmt.Printf("RPC server error: %v\n", err)
		}
	}()
	
	return nil
}

// registerRoutes registers all API routes
func (s *RPCServer) registerRoutes() {
	// Health check
	s.router.HandleFunc("/health", s.healthHandler).Methods("GET")
	
	// Blockchain routes
	s.router.HandleFunc("/blocks/latest", s.getLatestBlockHandler).Methods("GET")
	s.router.HandleFunc("/blocks/{hash}", s.getBlockByHashHandler).Methods("GET")
	s.router.HandleFunc("/transactions", s.submitTransactionHandler).Methods("POST")
	s.router.HandleFunc("/transactions/{hash}", s.getTransactionHandler).Methods("GET")
	
	// Account routes
	s.router.HandleFunc("/accounts/{address}", s.getAccountHandler).Methods("GET")
	s.router.HandleFunc("/accounts/{address}/balance", s.getBalanceHandler).Methods("GET")
}

// healthHandler handles health check requests
func (s *RPCServer) healthHandler(w http.ResponseWriter, r *http.Request) {
	response := map[string]interface{}{
		"status": "ok",
		"time":   time.Now().Unix(),
	}
	
	jsonResponse(w, response)
}

// getLatestBlockHandler handles requests for the latest block
func (s *RPCServer) getLatestBlockHandler(w http.ResponseWriter, r *http.Request) {
	// Get latest block
	block := s.blockchain.GetLatestBlock()
	
	if block == nil {
		errorResponse(w, "No blocks found", http.StatusNotFound)
		return
	}
	
	jsonResponse(w, block)
}

// getBlockByHashHandler handles requests for a specific block by hash
func (s *RPCServer) getBlockByHashHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	hash := vars["hash"]
	
	// Get block by hash
	block := s.blockchain.GetBlockByHash(hash)
	
	if block == nil {
		errorResponse(w, "Block not found", http.StatusNotFound)
		return
	}
	
	jsonResponse(w, block)
}

// submitTransactionHandler handles transaction submission
func (s *RPCServer) submitTransactionHandler(w http.ResponseWriter, r *http.Request) {
	var tx core.Transaction
	
	// Decode request body
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&tx); err != nil {
		errorResponse(w, "Invalid transaction format", http.StatusBadRequest)
		return
	}
	
	// Validate transaction
	if !tx.VerifySignature() {
		errorResponse(w, "Invalid transaction signature", http.StatusBadRequest)
		return
	}
	
	// Add to pending transactions
	s.blockchain.PendingTxs = append(s.blockchain.PendingTxs, tx)
	
	// Broadcast transaction to network
	txData, err := json.Marshal(tx)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error serializing transaction: %v", err), http.StatusInternalServerError)
		return
	}
	s.p2pNode.Broadcast(network.TxProtocol, txData)
	
	response := map[string]interface{}{
		"success": true,
		"hash":    fmt.Sprintf("%x", tx.Hash),
	}
	
	jsonResponse(w, response)
}

// getTransactionHandler handles requests for a specific transaction
func (s *RPCServer) getTransactionHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	hash := vars["hash"]
	
	// Get transaction by hash
	tx := s.blockchain.GetTransactionByHash(hash)
	
	if tx == nil {
		errorResponse(w, "Transaction not found", http.StatusNotFound)
		return
	}
	
	jsonResponse(w, tx)
}

// getAccountHandler handles requests for account information
func (s *RPCServer) getAccountHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	address := vars["address"]
	
	// Get account from state
	account, err := s.blockchain.State.GetAccount(address)
	
	if err != nil {
		errorResponse(w, "Account not found", http.StatusNotFound)
		return
	}
	
	jsonResponse(w, account)
}

// getBalanceHandler handles requests for account balance
func (s *RPCServer) getBalanceHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	address := vars["address"]
	
	// Get account from state
	account, err := s.blockchain.State.GetAccount(address)
	
	if err != nil {
		errorResponse(w, "Account not found", http.StatusNotFound)
		return
	}
	
	response := map[string]interface{}{
		"address": address,
		"balance": account.Balance,
	}
	
	jsonResponse(w, response)
}

// jsonResponse sends a JSON response
func jsonResponse(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	
	if err := json.NewEncoder(w).Encode(data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// errorResponse sends an error response
func errorResponse(w http.ResponseWriter, message string, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	
	response := map[string]interface{}{
		"error": message,
	}
	
	json.NewEncoder(w).Encode(response)
}