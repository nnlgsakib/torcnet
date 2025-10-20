package core

import (
	"time"

	"github.com/zeebo/blake3"
	"github.com/torcnet/torcnet/pkg/crypto"
)

// NanoBlock represents the smaller, independent blocks that run on individual nodes
type NanoBlock struct {
	Height      uint64            `json:"height"`
	PrevHash    []byte            `json:"prev_hash"`
	Timestamp   time.Time         `json:"timestamp"`
	Transactions []Transaction    `json:"transactions"`
	NodeID      string            `json:"node_id"`
	StateRoot   []byte            `json:"state_root"`
	Signature   []byte            `json:"signature"`
	Hash        []byte            `json:"hash"`
}

// MegaBlock represents the higher-level block that tracks multiple nano blocks
type MegaBlock struct {
	Height       uint64            `json:"height"`
	PrevHash     []byte            `json:"prev_hash"`
	Timestamp    time.Time         `json:"timestamp"`
	NanoBlockIDs [][]byte          `json:"nano_block_ids"`
	StateRoot    []byte            `json:"state_root"`
	ValidatorSet []string          `json:"validator_set"`
	Signatures   map[string][]byte `json:"signatures"`
	Hash         []byte            `json:"hash"`
}

// CalculateHash calculates the BLAKE3 hash of a NanoBlock
func (nb *NanoBlock) CalculateHash() []byte {
	hasher := blake3.New()
	
	// Add all fields to the hash
	hasher.Write(nb.PrevHash)
	hasher.Write([]byte(nb.Timestamp.String()))
	for _, tx := range nb.Transactions {
		hasher.Write(tx.Hash)
	}
	hasher.Write([]byte(nb.NodeID))
	hasher.Write(nb.StateRoot)
	
	return hasher.Sum(nil)
}

// CalculateHash calculates the BLAKE3 hash of a MegaBlock
func (mb *MegaBlock) CalculateHash() []byte {
	hasher := blake3.New()
	
	// Add all fields to the hash
	hasher.Write(mb.PrevHash)
	hasher.Write([]byte(mb.Timestamp.String()))
	for _, nanoID := range mb.NanoBlockIDs {
		hasher.Write(nanoID)
	}
	hasher.Write(mb.StateRoot)
	for _, validator := range mb.ValidatorSet {
		hasher.Write([]byte(validator))
	}
	
	return hasher.Sum(nil)
}

// Sign signs a NanoBlock with the node's private key
func (nb *NanoBlock) Sign(privateKey []byte) error {
	signature, err := crypto.SignBLS(privateKey, nb.Hash)
	if err != nil {
		return err
	}
	
	nb.Signature = signature
	return nil
}

// VerifySignature verifies the signature of a NanoBlock
func (nb *NanoBlock) VerifySignature() bool {
	publicKey, err := crypto.PublicKeyFromNodeID(nb.NodeID)
	if err != nil {
		return false
	}
	
	return crypto.VerifyBLS(publicKey, nb.Hash, nb.Signature)
}

// AddNanoBlock adds a NanoBlock reference to a MegaBlock
func (mb *MegaBlock) AddNanoBlock(nanoBlock *NanoBlock) {
	mb.NanoBlockIDs = append(mb.NanoBlockIDs, nanoBlock.Hash)
}

// Sign signs a MegaBlock with a validator's private key
func (mb *MegaBlock) Sign(validatorID string, privateKey []byte) error {
	if mb.Signatures == nil {
		mb.Signatures = make(map[string][]byte)
	}
	
	signature, err := crypto.SignBLS(privateKey, mb.Hash)
	if err != nil {
		return err
	}
	
	mb.Signatures[validatorID] = signature
	return nil
}

// VerifySignatures verifies all signatures of a MegaBlock
func (mb *MegaBlock) VerifySignatures() bool {
	if len(mb.Signatures) == 0 {
		return false
	}
	
	// Verify each validator's signature
	for validatorID, signature := range mb.Signatures {
		publicKey, err := crypto.PublicKeyFromValidatorID(validatorID)
		if err != nil {
			return false
		}
		
		if !crypto.VerifyBLS(publicKey, mb.Hash, signature) {
			return false
		}
	}
	
	return true
}