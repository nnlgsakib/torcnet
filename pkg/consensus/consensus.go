package consensus

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/torcnet/torcnet/pkg/core"
	"github.com/torcnet/torcnet/pkg/crypto"
	"github.com/torcnet/torcnet/pkg/network"
)

// ConsensusEngine represents the consensus engine
type ConsensusEngine struct {
	blockchain      *core.Blockchain
	validators      []string
	validatorKeys   map[string]*crypto.BLSKeyPair
	vrfKey          *crypto.VRFKeyPair
	isValidator     bool
	validatorID     string
	mu              sync.RWMutex
	stopChan        chan struct{}
	parallelWorkers int
	p2pNode         *network.P2PNode
}

// NewConsensusEngine creates a new consensus engine
func NewConsensusEngine(blockchain *core.Blockchain, isValidator bool, validatorID string, parallelWorkers int, p2pNode *network.P2PNode) *ConsensusEngine {
	return &ConsensusEngine{
		blockchain:      blockchain,
		validators:      make([]string, 0),
		validatorKeys:   make(map[string]*crypto.BLSKeyPair),
		isValidator:     isValidator,
		validatorID:     validatorID,
		stopChan:        make(chan struct{}),
		parallelWorkers: parallelWorkers,
		p2pNode:         p2pNode,
	}
}

// Start starts the consensus engine
func (ce *ConsensusEngine) Start() error {
	// Generate VRF key for randomness
	vrfKey, err := crypto.GenerateVRFKeyPair()
	if err != nil {
		return err
	}
	ce.vrfKey = vrfKey

	// If this node is a validator, generate BLS key
	if ce.isValidator {
		blsKey, err := crypto.GenerateBLSKeyPair()
		if err != nil {
			return err
		}
		ce.validatorKeys[ce.validatorID] = blsKey
		ce.validators = append(ce.validators, ce.validatorID)
	}

	// Start consensus loop in a goroutine
	go ce.consensusLoop()

	return nil
}

// Stop stops the consensus engine
func (ce *ConsensusEngine) Stop() {
	close(ce.stopChan)
}

// consensusLoop runs the main consensus loop
func (ce *ConsensusEngine) consensusLoop() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// If this node is a validator, propose blocks
			if ce.isValidator {
				ce.proposeBlocks()
			}
		case <-ce.stopChan:
			return
		}
	}
}

// proposeBlocks proposes new blocks
func (ce *ConsensusEngine) proposeBlocks() {
	// Create nano blocks in parallel
	nanoBlocks := ce.createNanoBlocksParallel()
	if len(nanoBlocks) == 0 {
		return
	}

	// Calculate actual state root from trie
	stateRoot := ce.blockchain.State.RootHash()
	megaBlock, err := ce.blockchain.CreateMegaBlock(ce.validatorID, ce.validatorKeys[ce.validatorID].PrivateKey, nanoBlocks, stateRoot)
	if err != nil {
		return
	}

	// Broadcast the mega block to other nodes
	if ce.p2pNode != nil {
		// Serialize the block
		blockData, err := json.Marshal(megaBlock)
		if err != nil {
			fmt.Printf("Error serializing block: %v\n", err)
			return
		}
		
		// Broadcast to peers
		ce.p2pNode.Broadcast(network.BlockProtocol, blockData)
	}
}

// createNanoBlocksParallel creates nano blocks in parallel
func (ce *ConsensusEngine) createNanoBlocksParallel() []*core.NanoBlock {
	var wg sync.WaitGroup
	var mu sync.Mutex
	nanoBlocks := make([]*core.NanoBlock, 0)

	// Create a channel for transactions to be processed
	txChan := make(chan core.Transaction, 100)

	// Start worker goroutines
	for i := 0; i < ce.parallelWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			// Process transactions and create nano blocks
			for tx := range txChan {
				// Validate transaction
				if !tx.VerifySignature() {
					fmt.Printf("Invalid transaction signature: %x\n", tx.Hash)
					continue
				}

				// Create a nano block with the transaction
				block, err := ce.blockchain.CreateNanoBlock(ce.validatorID, ce.validatorKeys[ce.validatorID].PrivateKey)
				if err != nil {
					fmt.Printf("Error creating nano block: %v\n", err)
					continue
				}

				// Add to the list of nano blocks
				mu.Lock()
				nanoBlocks = append(nanoBlocks, block)
				mu.Unlock()
			}
		}(i)
	}

	// Get transactions from mempool
	pendingTxs := ce.blockchain.PendingTxs
	for _, tx := range pendingTxs {
		txChan <- tx
	}
	close(txChan)

	// Wait for all workers to finish
	wg.Wait()

	return nanoBlocks
}

// AddValidator adds a validator to the consensus engine
func (ce *ConsensusEngine) AddValidator(validatorID string, publicKey []byte) {
	ce.mu.Lock()
	defer ce.mu.Unlock()

	ce.validators = append(ce.validators, validatorID)
}

// RemoveValidator removes a validator from the consensus engine
func (ce *ConsensusEngine) RemoveValidator(validatorID string) {
	ce.mu.Lock()
	defer ce.mu.Unlock()

	for i, v := range ce.validators {
		if v == validatorID {
			ce.validators = append(ce.validators[:i], ce.validators[i+1:]...)
			delete(ce.validatorKeys, validatorID)
			break
		}
	}
}