package core

import (
	"time"

	"github.com/zeebo/blake3"
	"github.com/torcnet/torcnet/pkg/crypto"
)

// Transaction represents a transaction in the TORC NET blockchain
type Transaction struct {
	Hash      []byte    `json:"hash"`
	From      string    `json:"from"`
	To        string    `json:"to"`
	Amount    uint64    `json:"amount"`
	Fee       uint64    `json:"fee"`
	Nonce     uint64    `json:"nonce"`
	Data      []byte    `json:"data"`
	Timestamp time.Time `json:"timestamp"`
	Signature []byte    `json:"signature"`
}

// NewTransaction creates a new transaction
func NewTransaction(from, to string, amount, fee, nonce uint64, data []byte) *Transaction {
	tx := &Transaction{
		From:      from,
		To:        to,
		Amount:    amount,
		Fee:       fee,
		Nonce:     nonce,
		Data:      data,
		Timestamp: time.Now(),
	}
	
	tx.Hash = tx.CalculateHash()
	return tx
}

// CalculateHash calculates the BLAKE3 hash of the transaction
func (tx *Transaction) CalculateHash() []byte {
	hasher := blake3.New()
	
	// Add all fields to the hash
	hasher.Write([]byte(tx.From))
	hasher.Write([]byte(tx.To))
	hasher.Write(uint64ToBytes(tx.Amount))
	hasher.Write(uint64ToBytes(tx.Fee))
	hasher.Write(uint64ToBytes(tx.Nonce))
	hasher.Write(tx.Data)
	hasher.Write([]byte(tx.Timestamp.String()))
	
	return hasher.Sum(nil)
}

// Sign signs the transaction with the sender's private key
func (tx *Transaction) Sign(privateKey []byte) error {
	// Create a signature using the private key
	signature, err := crypto.SignECDSA(privateKey, tx.Hash)
	if err != nil {
		return err
	}
	
	tx.Signature = signature
	return nil
}

// VerifySignature verifies the transaction signature
func (tx *Transaction) VerifySignature() bool {
	// Extract public key from the From address
	publicKey, err := crypto.PublicKeyFromAddress(tx.From)
	if err != nil {
		return false
	}
	
	// Verify the signature
	return crypto.VerifyECDSA(publicKey, tx.Hash, tx.Signature)
}

// Helper function to convert uint64 to bytes
func uint64ToBytes(n uint64) []byte {
	b := make([]byte, 8)
	for i := uint64(0); i < 8; i++ {
		b[i] = byte(n >> (i * 8))
	}
	return b
}