package core

import (
	"encoding/hex"
	"fmt"
	"math/big"

	"github.com/torcnet/torcnet/pkg/vm"
)

// GetBalance returns the balance of an address using EVM integration
func (bc *Blockchain) GetBalance(address string) *big.Int {
	bc.mu.RLock()
	defer bc.mu.RUnlock()
	
	return bc.EVMIntegration.GetBalance(address)
}

// GetNonce returns the nonce of an address using EVM integration
func (bc *Blockchain) GetNonce(address string) uint64 {
	bc.mu.RLock()
	defer bc.mu.RUnlock()
	
	return bc.EVMIntegration.GetNonce(address)
}

// GetCode returns the contract code at an address
func (bc *Blockchain) GetCode(address string) []byte {
	bc.mu.RLock()
	defer bc.mu.RUnlock()
	
	return bc.EVMIntegration.GetCode(address)
}

// GetStorageAt returns storage value at a specific key for an address
func (bc *Blockchain) GetStorageAt(address string, key string) []byte {
	bc.mu.RLock()
	defer bc.mu.RUnlock()
	
	return bc.EVMIntegration.GetStorageAt(address, key)
}

// CallContract executes a contract call without creating a transaction
func (bc *Blockchain) CallContract(tx *vm.EVMTransaction) (*vm.ExecutionResult, error) {
	bc.mu.RLock()
	defer bc.mu.RUnlock()
	
	return bc.EVMIntegration.Call(tx, bc.CurrentNanoHeight)
}

// EstimateGas estimates gas required for a transaction
func (bc *Blockchain) EstimateGas(tx *vm.EVMTransaction) (uint64, error) {
	bc.mu.RLock()
	defer bc.mu.RUnlock()
	
	return bc.EVMIntegration.EstimateGas(tx)
}

// GetTransactionReceipt gets a transaction receipt by hash
func (bc *Blockchain) GetTransactionReceipt(txHash string) (*vm.TransactionReceipt, error) {
	bc.mu.RLock()
	defer bc.mu.RUnlock()
	
	return bc.EVMIntegration.GetTransactionReceipt(txHash)
}

// GetHeight returns the current blockchain height
func (bc *Blockchain) GetHeight() uint64 {
	bc.mu.RLock()
	defer bc.mu.RUnlock()
	
	return bc.CurrentNanoHeight
}

// GetChainID returns the chain ID
func (bc *Blockchain) GetChainID() *big.Int {
	return bc.EVMIntegration.chainID
}

// ProcessEVMTransaction processes an EVM transaction directly
func (bc *Blockchain) ProcessEVMTransaction(tx *vm.EVMTransaction) (*vm.TransactionReceipt, error) {
	bc.mu.Lock()
	defer bc.mu.Unlock()
	
	blockHeight := bc.CurrentNanoHeight
	blockHash := make([]byte, 32) // TODO: Get actual current block hash
	
	receipt, err := bc.EVMIntegration.ProcessEVMTransaction(tx, blockHeight, blockHash, 0)
	if err != nil {
		return nil, fmt.Errorf("EVM transaction failed: %v", err)
	}
	
	// Commit state changes
	if err := bc.EVMIntegration.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit EVM state: %v", err)
	}
	
	return receipt, nil
}

// AddEVMTransaction adds an EVM transaction to the pending pool
func (bc *Blockchain) AddEVMTransaction(evmTx *vm.EVMTransaction) error {
	bc.mu.Lock()
	defer bc.mu.Unlock()
	
	// Convert EVM transaction to regular transaction for storage
	tx := Transaction{
		From:      hex.EncodeToString(evmTx.From),
		To:        hex.EncodeToString(evmTx.To),
		Amount:    evmTx.Value.Uint64(),
		Fee:       new(big.Int).Mul(big.NewInt(int64(evmTx.Gas)), evmTx.GasPrice).Uint64(),
		Nonce:     evmTx.Nonce,
		Data:      evmTx.Data,
		Timestamp: getNow(),
	}
	
	// Calculate hash
	tx.Hash = tx.CalculateHash()
	
	// Add to pending transactions
	bc.PendingTxs = append(bc.PendingTxs, tx)
	
	return nil
}