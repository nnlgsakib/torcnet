package core

import (
	"fmt"
	"math/big"
	"encoding/hex"

	"github.com/torcnet/torcnet/pkg/vm"
	"github.com/torcnet/torcnet/pkg/db"
)

// EVMIntegration handles EVM transaction processing within the blockchain
type EVMIntegration struct {
	evm       *vm.EVM
	stateDB   *vm.EVMStateDB
	processor *vm.TransactionProcessor
	chainID   *big.Int
}

// NewEVMIntegration creates a new EVM integration instance
func NewEVMIntegration(blockchain *Blockchain, chainID *big.Int) *EVMIntegration {
	// Create EVM state DB with the blockchain's database
	// We need to access the database from the blockchain's state
	// For now, we'll create a memory database as a temporary solution
	memDB := db.NewMemoryDB()
	stateDB := vm.NewEVMStateDB(memDB)

	// Create EVM instance with proper parameters
	evm := vm.NewEVM(stateDB, chainID, big.NewInt(1), big.NewInt(1000000000), uint64(1000000), []byte{}, big.NewInt(1))

	// Create transaction processor with correct parameter order
	processor := vm.NewTransactionProcessor(stateDB, evm, chainID, big.NewInt(1000000000))

	return &EVMIntegration{
		evm:       evm,
		stateDB:   stateDB,
		processor: processor,
		chainID:   chainID,
	}
}

// ProcessEVMTransaction processes an EVM transaction and returns a receipt
func (ei *EVMIntegration) ProcessEVMTransaction(tx *vm.EVMTransaction, blockNumber uint64, blockHash []byte, txIndex uint64) (*vm.TransactionReceipt, error) {
	return ei.processor.ProcessTransaction(tx, blockNumber, blockHash, uint32(txIndex))
}

// ConvertToEVMTransaction converts a regular transaction to an EVM transaction
func (ei *EVMIntegration) ConvertToEVMTransaction(tx *Transaction) *vm.EVMTransaction {
	// Convert string addresses to byte arrays
	fromBytes, _ := hex.DecodeString(tx.From)
	toBytes, _ := hex.DecodeString(tx.To)
	
	evmTx := &vm.EVMTransaction{
		Nonce:    tx.Nonce,
		GasPrice: big.NewInt(int64(tx.Fee)), // Use fee as gas price
		Gas:      21000,                     // Default gas limit for simple transfers
		To:       toBytes,
		Value:    big.NewInt(int64(tx.Amount)),
		Data:     tx.Data,
		V:        big.NewInt(1), // Default values for signature
		R:        big.NewInt(1),
		S:        big.NewInt(1),
	}

	// Set sender address
	evmTx.From = fromBytes

	// Calculate hash using the private method (we'll need to make it public or use a different approach)
	evmTx.Hash = evmTx.CalculateHash()

	// If transaction has data, increase gas limit for contract interaction
	if len(tx.Data) > 0 {
		evmTx.Gas = 100000 // Higher gas limit for contract calls
	}

	return evmTx
}

// EstimateGas estimates gas required for a transaction
func (ei *EVMIntegration) EstimateGas(tx *vm.EVMTransaction) (uint64, error) {
	return ei.processor.EstimateGas(tx)
}

// GetTransactionReceipt gets a transaction receipt by hash
func (ei *EVMIntegration) GetTransactionReceipt(txHash string) (*vm.TransactionReceipt, error) {
	// This would typically query a receipt database
	// For now, return nil as receipts are generated during processing
	return nil, fmt.Errorf("receipt not found for transaction %s", txHash)
}

// Call executes a contract call without creating a transaction
func (ei *EVMIntegration) Call(tx *vm.EVMTransaction, blockNumber uint64) (*vm.ExecutionResult, error) {
	// Create a snapshot to avoid state changes
	snapshot := ei.stateDB.Snapshot()
	defer ei.stateDB.RevertToSnapshot(snapshot)

	// Execute the call
	if len(tx.To) == 0 {
		return nil, fmt.Errorf("cannot call contract creation")
	}

	// Check if target is a contract
	code := ei.stateDB.GetCode(tx.To)
	if len(code) == 0 {
		// Simple value transfer
		return &vm.ExecutionResult{
			ReturnData: []byte{},
			GasUsed:    21000,
		}, nil
	}

	// Execute contract call using the EVM's Call method with proper signature
	result, gasUsed, err := ei.evm.Call(tx.From, tx.To, tx.Data, tx.Gas, tx.Value)
	if err != nil {
		return nil, err
	}

	return &vm.ExecutionResult{
		ReturnData: result,
		GasUsed:    gasUsed,
	}, nil
}

// GetBalance gets the balance of an address
func (ei *EVMIntegration) GetBalance(address string) *big.Int {
	addressBytes, _ := hex.DecodeString(address)
	return ei.stateDB.GetBalance(addressBytes)
}

// GetNonce gets the nonce of an address
func (ei *EVMIntegration) GetNonce(address string) uint64 {
	addressBytes, _ := hex.DecodeString(address)
	return ei.stateDB.GetNonce(addressBytes)
}

// GetCode gets the contract code at an address
func (ei *EVMIntegration) GetCode(address string) []byte {
	addressBytes, _ := hex.DecodeString(address)
	return ei.stateDB.GetCode(addressBytes)
}

// GetStorageAt gets storage value at a specific key for an address
func (ei *EVMIntegration) GetStorageAt(address string, key string) []byte {
	addressBytes, _ := hex.DecodeString(address)
	keyBytes, _ := hex.DecodeString(key)
	return ei.stateDB.GetState(addressBytes, keyBytes)
}

// GetStateDB returns the EVM state database
func (ei *EVMIntegration) GetStateDB() *vm.EVMStateDB {
	return ei.stateDB
}

// Commit commits all pending state changes
func (ei *EVMIntegration) Commit() error {
	return ei.stateDB.Commit()
}

// Snapshot creates a state snapshot
func (ei *EVMIntegration) Snapshot() int {
	return ei.stateDB.Snapshot()
}

// RevertToSnapshot reverts to a previous state snapshot
func (ei *EVMIntegration) RevertToSnapshot(id int) {
	ei.stateDB.RevertToSnapshot(id)
}
