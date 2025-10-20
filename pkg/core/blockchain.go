package core

import (
	"errors"
	"fmt"
	"math/big"
	"sync"
	
	"github.com/torcnet/torcnet/pkg/db"
	"github.com/torcnet/torcnet/pkg/state"
)

// Blockchain represents the main blockchain structure
type Blockchain struct {
	NanoChain     []*NanoBlock
	MegaChain     []*MegaBlock
	CurrentNanoHeight uint64
	CurrentMegaHeight uint64
	PendingTxs    []Transaction
	mu            sync.RWMutex
	State       *state.State
	EVMIntegration *EVMIntegration
}

// NewBlockchain creates a new blockchain instance
func NewBlockchain(dataDir string, dbBackend string) *Blockchain {
	var database db.Database
	var err error
	
	// Initialize database based on dbBackend
	switch dbBackend {
	case "leveldb":
		database = db.NewLevelDB()
		err = database.Open(dataDir)
	case "pebble":
		database = db.NewPebbleDB()
		err = database.Open(dataDir)
	case "rocksdb":
		database = db.NewRocksDB()
		err = database.Open(dataDir)
	default:
		// Default to PebbleDB
		database = db.NewPebbleDB()
		err = database.Open(dataDir)
	}
	
	if err != nil {
		// Log error and fallback to in-memory mode
		fmt.Printf("Failed to initialize database: %v, falling back to in-memory mode\n", err)
		database = db.NewMemoryDB()
	}
	
	// Initialize state with the database
	stateInstance := state.NewState(database, 1000)
	
	// Initialize EVM integration with chain ID 1 (mainnet)
	chainID := big.NewInt(1)
	
	blockchain := &Blockchain{
		NanoChain:     make([]*NanoBlock, 0),
		MegaChain:     make([]*MegaBlock, 0),
		CurrentNanoHeight: 0,
		CurrentMegaHeight: 0,
		PendingTxs:    make([]Transaction, 0),
		State:       stateInstance,
	}
	
	// Initialize EVM integration
	blockchain.EVMIntegration = NewEVMIntegration(blockchain, chainID)
	
	return blockchain
}

// AddTransaction adds a transaction to the pending pool
func (bc *Blockchain) AddTransaction(tx Transaction) {
	bc.mu.Lock()
	defer bc.mu.Unlock()
	
	bc.PendingTxs = append(bc.PendingTxs, tx)
}

// CreateNanoBlock creates a new nano block with pending transactions
func (bc *Blockchain) CreateNanoBlock(nodeID string, privateKey []byte) (*NanoBlock, error) {
	bc.mu.Lock()
	defer bc.mu.Unlock()
	
	if len(bc.PendingTxs) == 0 {
		return nil, errors.New("no transactions to include in block")
	}
	
	// Apply transactions to state before creating block
	for _, tx := range bc.PendingTxs {
		if err := bc.ProcessTransaction(&tx); err != nil {
			return nil, fmt.Errorf("failed to process transaction %x: %v", tx.Hash, err)
		}
	}

	// Commit state changes
	if err := bc.State.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit state: %v", err)
	}

	// Get updated state root
	stateRoot := bc.State.RootHash()
	
	var prevHash []byte
	if bc.CurrentNanoHeight > 0 {
		prevHash = bc.NanoChain[bc.CurrentNanoHeight-1].Hash
	}
	
	block := &NanoBlock{
		Height:      bc.CurrentNanoHeight + 1,
		PrevHash:    prevHash,
		Timestamp:   getNow(),
		Transactions: bc.PendingTxs,
		NodeID:      nodeID,
		StateRoot:   stateRoot, // Add state root to block
	}
	
	// Calculate hash
	block.Hash = block.CalculateHash()
	
	// Sign the block
	if err := block.Sign(privateKey); err != nil {
		return nil, err
	}
	
	// Add to chain and update height
	bc.NanoChain = append(bc.NanoChain, block)
	bc.CurrentNanoHeight++
	
	// Clear pending transactions
	bc.PendingTxs = make([]Transaction, 0)
	
	return block, nil
}

// CreateMegaBlock creates a new mega block that references nano blocks
func (bc *Blockchain) CreateMegaBlock(validatorID string, privateKey []byte, nanoBlocks []*NanoBlock, stateRoot []byte) (*MegaBlock, error) {
	bc.mu.Lock()
	defer bc.mu.Unlock()
	
	if len(nanoBlocks) == 0 {
		return nil, errors.New("no nano blocks to include in mega block")
	}
	
	var prevHash []byte
	if bc.CurrentMegaHeight > 0 {
		prevHash = bc.MegaChain[bc.CurrentMegaHeight-1].Hash
	}
	
	block := &MegaBlock{
		Height:       bc.CurrentMegaHeight + 1,
		PrevHash:     prevHash,
		Timestamp:    getNow(),
		StateRoot:    stateRoot,
		ValidatorSet: []string{validatorID}, // In a real implementation, this would be the actual validator set
	}
	
	// Add nano block references
	for _, nanoBlock := range nanoBlocks {
		block.AddNanoBlock(nanoBlock)
	}
	
	// Calculate hash
	block.Hash = block.CalculateHash()
	
	// Sign the block
	if err := block.Sign(validatorID, privateKey); err != nil {
		return nil, err
	}
	
	// Add to chain and update height
	bc.MegaChain = append(bc.MegaChain, block)
	bc.CurrentMegaHeight++
	
	return block, nil
}

// ValidateNanoBlock validates a nano block
func (bc *Blockchain) ValidateNanoBlock(block *NanoBlock) bool {
	// Verify block hash
	calculatedHash := block.CalculateHash()
	if !bytesEqual(calculatedHash, block.Hash) {
		return false
	}
	
	// Verify signature
	if !block.VerifySignature() {
		return false
	}
	
	// Verify transactions
	for _, tx := range block.Transactions {
		if !tx.VerifySignature() {
			return false
		}
	}
	
	return true
}

// ValidateMegaBlock validates a mega block
func (bc *Blockchain) ValidateMegaBlock(block *MegaBlock) bool {
	// Verify block hash
	calculatedHash := block.CalculateHash()
	if !bytesEqual(calculatedHash, block.Hash) {
		return false
	}
	
	// Verify signatures
	if !block.VerifySignatures() {
		return false
	}
	
	return true
}

// Helper function to compare byte slices
func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// GetLatestBlock returns the latest nano block
func (bc *Blockchain) GetLatestBlock() *NanoBlock {
	bc.mu.RLock()
	defer bc.mu.RUnlock()
	
	if len(bc.NanoChain) == 0 {
		return nil
	}
	
	return bc.NanoChain[len(bc.NanoChain)-1]
}

// GetBlockByHash returns a block by its hash
func (bc *Blockchain) GetBlockByHash(hash string) *NanoBlock {
	bc.mu.RLock()
	defer bc.mu.RUnlock()
	
	for _, block := range bc.NanoChain {
		if fmt.Sprintf("%x", block.Hash) == hash {
			return block
		}
	}
	
	return nil
}

// ProcessTransaction processes a transaction and updates the state accordingly
func (bc *Blockchain) ProcessTransaction(tx *Transaction) error {
	// Verify transaction signature
	if !tx.VerifySignature() {
		return fmt.Errorf("invalid transaction signature")
	}
	
	// Get sender account
	sender, err := bc.State.GetAccount(tx.From)
	if err != nil {
		return fmt.Errorf("sender account not found: %s", tx.From)
	}
	
	// Check nonce
	if tx.Nonce != sender.Nonce {
		return fmt.Errorf("invalid nonce: expected %d, got %d", sender.Nonce, tx.Nonce)
	}
	
	// Check balance (amount + fee when applicable)
	if sender.Balance < tx.Amount+tx.Fee {
		return fmt.Errorf("insufficient balance: %d < %d", sender.Balance, tx.Amount+tx.Fee)
	}
	
	// Determine type by presence of data: empty -> transfer, non-empty -> contract
	if len(tx.Data) == 0 {
		// Simple transfer
		return bc.State.Transfer(tx.From, tx.To, tx.Amount)
	}
	
	// Smart contract or data-bearing transaction
	return bc.executeSmartContract(tx)
}

// executeSmartContract executes a smart contract transaction using EVM
func (bc *Blockchain) executeSmartContract(tx *Transaction) error {
	// Convert regular transaction to EVM transaction
	evmTx := bc.EVMIntegration.ConvertToEVMTransaction(tx)
	
	// Process EVM transaction
	blockHeight := bc.CurrentNanoHeight
	blockHash := make([]byte, 32) // TODO: Get actual current block hash
	
	receipt, err := bc.EVMIntegration.ProcessEVMTransaction(evmTx, blockHeight, blockHash, 0)
	if err != nil {
		return fmt.Errorf("EVM transaction failed: %v", err)
	}
	
	// Check if transaction was successful
	if receipt.Status == 0 {
		return fmt.Errorf("EVM transaction reverted")
	}
	
	return nil
}

// ApplyNanoBlock applies all transactions in a nano block to the state
func (bc *Blockchain) ApplyNanoBlock(block *NanoBlock) error {
	for _, tx := range block.Transactions {
		if err := bc.ProcessTransaction(&tx); err != nil {
			return fmt.Errorf("failed to process transaction %x: %v", tx.Hash, err)
		}
	}
	
	// Commit all changes to the database
	return bc.State.Commit()
}

// RevertNanoBlock reverts all transactions in a nano block from the state
// This is used when a block is invalidated or during chain reorganization
func (bc *Blockchain) RevertNanoBlock(block *NanoBlock) error {
	// Simplified placeholder implementation
	return fmt.Errorf("nano block reversion not implemented")
}

// GetTransactionByHash returns a transaction by its hash
func (bc *Blockchain) GetTransactionByHash(hash string) *Transaction {
	bc.mu.RLock()
	defer bc.mu.RUnlock()
	
	// Search in all blocks
	for _, block := range bc.NanoChain {
		for _, tx := range block.Transactions {
			if fmt.Sprintf("%x", tx.Hash) == hash {
				return &tx
			}
		}
	}
	
	// Search in pending transactions
	for _, tx := range bc.PendingTxs {
		if fmt.Sprintf("%x", tx.Hash) == hash {
			return &tx
		}
	}
	
	return nil
}