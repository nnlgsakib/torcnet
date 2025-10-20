package rpc

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/big"
	"strconv"
	"strings"

	"github.com/torcnet/torcnet/pkg/core"
	"github.com/torcnet/torcnet/pkg/vm"
)

// EVMRPCService provides EVM-compatible RPC methods
type EVMRPCService struct {
	blockchain *core.Blockchain
}

// NewEVMRPCService creates a new EVM RPC service
func NewEVMRPCService(blockchain *core.Blockchain) *EVMRPCService {
	return &EVMRPCService{
		blockchain: blockchain,
	}
}

// CallRequest represents an eth_call request
type CallRequest struct {
	From     string `json:"from,omitempty"`
	To       string `json:"to"`
	Gas      string `json:"gas,omitempty"`
	GasPrice string `json:"gasPrice,omitempty"`
	Value    string `json:"value,omitempty"`
	Data     string `json:"data,omitempty"`
}

// TransactionRequest represents a transaction request
type TransactionRequest struct {
	From     string `json:"from"`
	To       string `json:"to,omitempty"`
	Gas      string `json:"gas,omitempty"`
	GasPrice string `json:"gasPrice,omitempty"`
	Value    string `json:"value,omitempty"`
	Data     string `json:"data,omitempty"`
	Nonce    string `json:"nonce,omitempty"`
}

// BlockParameter represents a block parameter (latest, earliest, pending, or block number)
type BlockParameter string

const (
	BlockLatest   BlockParameter = "latest"
	BlockEarliest BlockParameter = "earliest"
	BlockPending  BlockParameter = "pending"
)

// EthChainId returns the chain ID
func (s *EVMRPCService) EthChainId() (string, error) {
	chainID := s.blockchain.GetChainID()
	return fmt.Sprintf("0x%x", chainID), nil
}

// EthBlockNumber returns the current block number
func (s *EVMRPCService) EthBlockNumber() (string, error) {
	height := s.blockchain.GetHeight()
	return fmt.Sprintf("0x%x", height), nil
}

// EthGetBalance returns the balance of an account
func (s *EVMRPCService) EthGetBalance(address string, blockParam BlockParameter) (string, error) {
	balance := s.blockchain.GetBalance(address)
	return fmt.Sprintf("0x%x", balance), nil
}

// EthGetTransactionCount returns the nonce of an account
func (s *EVMRPCService) EthGetTransactionCount(address string, blockParam BlockParameter) (string, error) {
	nonce := s.blockchain.GetNonce(address)
	return fmt.Sprintf("0x%x", nonce), nil
}

// EthGetCode returns the code at a given address
func (s *EVMRPCService) EthGetCode(address string, blockParam BlockParameter) (string, error) {
	code := s.blockchain.GetCode(address)
	if len(code) == 0 {
		return "0x", nil
	}
	return "0x" + hex.EncodeToString(code), nil
}

// EthCall executes a message call transaction
func (s *EVMRPCService) EthCall(callReq CallRequest, blockParam BlockParameter) (string, error) {
	// Convert string addresses to byte arrays
	fromBytes, _ := hex.DecodeString(strings.TrimPrefix(callReq.From, "0x"))
	toBytes, _ := hex.DecodeString(strings.TrimPrefix(callReq.To, "0x"))
	
	// Create EVM transaction from call request
	evmTx := &vm.EVMTransaction{
		From:     fromBytes,
		To:       toBytes,
		Gas:      21000, // Default gas limit
		GasPrice: big.NewInt(1),
		Value:    big.NewInt(0),
		Data:     []byte{},
	}

	// Parse gas if provided
	if callReq.Gas != "" {
		gas, err := strconv.ParseUint(strings.TrimPrefix(callReq.Gas, "0x"), 16, 64)
		if err == nil {
			evmTx.Gas = gas
		}
	}

	// Parse gas price if provided
	if callReq.GasPrice != "" {
		gasPrice, ok := new(big.Int).SetString(strings.TrimPrefix(callReq.GasPrice, "0x"), 16)
		if ok {
			evmTx.GasPrice = gasPrice
		}
	}

	// Parse value if provided
	if callReq.Value != "" {
		value, ok := new(big.Int).SetString(strings.TrimPrefix(callReq.Value, "0x"), 16)
		if ok {
			evmTx.Value = value
		}
	}

	// Parse data if provided
	if callReq.Data != "" {
		data, err := hex.DecodeString(strings.TrimPrefix(callReq.Data, "0x"))
		if err == nil {
			evmTx.Data = data
		}
	}

	// Execute call
	result, err := s.blockchain.CallContract(evmTx)
	if err != nil {
		return "", fmt.Errorf("call failed: %v", err)
	}

	return "0x" + hex.EncodeToString(result.ReturnData), nil
}

// EthSendTransaction sends a transaction
func (s *EVMRPCService) EthSendTransaction(txReq TransactionRequest) (string, error) {
	// Convert string addresses to byte arrays
	fromBytes, _ := hex.DecodeString(strings.TrimPrefix(txReq.From, "0x"))
	toBytes, _ := hex.DecodeString(strings.TrimPrefix(txReq.To, "0x"))
	
	// Create EVM transaction from request
	evmTx := &vm.EVMTransaction{
		From:     fromBytes,
		To:       toBytes,
		Gas:      21000,                  // Default gas limit
		GasPrice: big.NewInt(1000000000), // Default 1 Gwei
		Value:    big.NewInt(0),
		Data:     []byte{},
	}

	// Parse gas if provided
	if txReq.Gas != "" {
		gas, err := strconv.ParseUint(strings.TrimPrefix(txReq.Gas, "0x"), 16, 64)
		if err == nil {
			evmTx.Gas = gas
		}
	}

	// Parse gas price if provided
	if txReq.GasPrice != "" {
		gasPrice, ok := new(big.Int).SetString(strings.TrimPrefix(txReq.GasPrice, "0x"), 16)
		if ok {
			evmTx.GasPrice = gasPrice
		}
	}

	// Parse value if provided
	if txReq.Value != "" {
		value, ok := new(big.Int).SetString(strings.TrimPrefix(txReq.Value, "0x"), 16)
		if ok {
			evmTx.Value = value
		}
	}

	// Parse data if provided
	if txReq.Data != "" {
		data, err := hex.DecodeString(strings.TrimPrefix(txReq.Data, "0x"))
		if err == nil {
			evmTx.Data = data
		}
	}

	// Parse nonce if provided
	if txReq.Nonce != "" {
		nonce, err := strconv.ParseUint(strings.TrimPrefix(txReq.Nonce, "0x"), 16, 64)
		if err == nil {
			evmTx.Nonce = nonce
		}
	} else {
		evmTx.Nonce = s.blockchain.GetNonce(hex.EncodeToString(fromBytes))
	}

	// Calculate hash
	evmTx.Hash = evmTx.CalculateHash()

	// Process transaction
	receipt, err := s.blockchain.ProcessEVMTransaction(evmTx)
	if err != nil {
		return "", fmt.Errorf("transaction failed: %v", err)
	}

	return "0x" + hex.EncodeToString(receipt.TransactionHash), nil
}

// EthEstimateGas estimates gas for a transaction
func (s *EVMRPCService) EthEstimateGas(callReq CallRequest) (string, error) {
	// Convert string addresses to byte arrays
	fromBytes, _ := hex.DecodeString(strings.TrimPrefix(callReq.From, "0x"))
	toBytes, _ := hex.DecodeString(strings.TrimPrefix(callReq.To, "0x"))
	
	// Create EVM transaction from call request
	evmTx := &vm.EVMTransaction{
		From:     fromBytes,
		To:       toBytes,
		Gas:      10000000, // High gas limit for estimation
		GasPrice: big.NewInt(1),
		Value:    big.NewInt(0),
		Data:     []byte{},
	}

	// Parse value if provided
	if callReq.Value != "" {
		value, ok := new(big.Int).SetString(strings.TrimPrefix(callReq.Value, "0x"), 16)
		if ok {
			evmTx.Value = value
		}
	}

	// Parse data if provided
	if callReq.Data != "" {
		data, err := hex.DecodeString(strings.TrimPrefix(callReq.Data, "0x"))
		if err == nil {
			evmTx.Data = data
		}
	}

	// Estimate gas
	gasEstimate, err := s.blockchain.EstimateGas(evmTx)
	if err != nil {
		return "", fmt.Errorf("gas estimation failed: %v", err)
	}

	return fmt.Sprintf("0x%x", gasEstimate), nil
}

// EthGetTransactionReceipt returns the receipt of a transaction
func (s *EVMRPCService) EthGetTransactionReceipt(txHash string) (map[string]interface{}, error) {
	receipt, err := s.blockchain.GetTransactionReceipt(txHash)
	if err != nil {
		return nil, fmt.Errorf("receipt not found: %v", err)
	}

	return map[string]interface{}{
		"transactionHash":   "0x" + hex.EncodeToString(receipt.TransactionHash),
		"transactionIndex":  fmt.Sprintf("0x%x", receipt.TransactionIndex),
		"blockHash":         "0x" + hex.EncodeToString(receipt.BlockHash),
		"blockNumber":       fmt.Sprintf("0x%x", receipt.BlockNumber),
		"from":              receipt.From,
		"to":                receipt.To,
		"cumulativeGasUsed": fmt.Sprintf("0x%x", receipt.GasUsed),
		"gasUsed":           fmt.Sprintf("0x%x", receipt.GasUsed),
		"contractAddress":   receipt.ContractAddress,
		"logs":              receipt.Logs,
		"status":            fmt.Sprintf("0x%x", receipt.Status),
	}, nil
}

// EthGetStorageAt returns the storage value at a specific position
func (s *EVMRPCService) EthGetStorageAt(address string, position string, blockParam BlockParameter) (string, error) {
	value := s.blockchain.GetStorageAt(address, position)
	return "0x" + hex.EncodeToString(value), nil
}

// EthGasPrice returns the current gas price
func (s *EVMRPCService) EthGasPrice() (string, error) {
	// Return default gas price (1 Gwei)
	gasPrice := big.NewInt(1000000000)
	return fmt.Sprintf("0x%x", gasPrice), nil
}

// EthGetLogs returns logs matching the filter criteria
func (s *EVMRPCService) EthGetLogs(filter map[string]interface{}) ([]map[string]interface{}, error) {
	// TODO: Implement proper log filtering
	// For now, return empty array
	return []map[string]interface{}{}, nil
}

// NetVersion returns the network ID
func (s *EVMRPCService) NetVersion() (string, error) {
	chainID := s.blockchain.GetChainID()
	return chainID.String(), nil
}

// Web3ClientVersion returns the client version
func (s *EVMRPCService) Web3ClientVersion() (string, error) {
	return "TORC-NET/1.0.0", nil
}

// Web3Sha3 returns the Keccak-256 hash of the given data
func (s *EVMRPCService) Web3Sha3(data string) (string, error) {
	dataBytes, err := hexToBytes(data)
	if err != nil {
		return "", fmt.Errorf("invalid data: %v", err)
	}

	hash := sha256.Sum256(dataBytes)
	return "0x" + hex.EncodeToString(hash[:]), nil
}

// Helper functions

func hexToBytes(hexStr string) ([]byte, error) {
	if strings.HasPrefix(hexStr, "0x") {
		hexStr = hexStr[2:]
	}

	// Ensure even length
	if len(hexStr)%2 != 0 {
		hexStr = "0" + hexStr
	}

	return hex.DecodeString(hexStr)
}

func hexToBigInt(hexStr string) (*big.Int, error) {
	if strings.HasPrefix(hexStr, "0x") {
		hexStr = hexStr[2:]
	}

	result := new(big.Int)
	result.SetString(hexStr, 16)
	return result, nil
}

func hexToUint64(hexStr string) (uint64, error) {
	if strings.HasPrefix(hexStr, "0x") {
		hexStr = hexStr[2:]
	}

	return strconv.ParseUint(hexStr, 16, 64)
}

func bytesToHex(data []byte) string {
	return "0x" + hex.EncodeToString(data)
}

// RegisterEVMRPCMethods registers all EVM RPC methods with the RPC server
func RegisterEVMRPCMethods(server *RPCServer, service *EVMRPCService) {
	// Ethereum JSON-RPC methods
	server.RegisterMethod("eth_chainId", service.EthChainId)
	server.RegisterMethod("eth_blockNumber", service.EthBlockNumber)
	server.RegisterMethod("eth_getBalance", service.EthGetBalance)
	server.RegisterMethod("eth_getTransactionCount", service.EthGetTransactionCount)
	server.RegisterMethod("eth_getCode", service.EthGetCode)
	server.RegisterMethod("eth_call", service.EthCall)
	server.RegisterMethod("eth_sendTransaction", service.EthSendTransaction)
	server.RegisterMethod("eth_estimateGas", service.EthEstimateGas)
	server.RegisterMethod("eth_getTransactionReceipt", service.EthGetTransactionReceipt)
	server.RegisterMethod("eth_getStorageAt", service.EthGetStorageAt)
	server.RegisterMethod("eth_gasPrice", service.EthGasPrice)
	server.RegisterMethod("eth_getLogs", service.EthGetLogs)

	// Network methods
	server.RegisterMethod("net_version", service.NetVersion)

	// Web3 methods
	server.RegisterMethod("web3_clientVersion", service.Web3ClientVersion)
	server.RegisterMethod("web3_sha3", service.Web3Sha3)
}
