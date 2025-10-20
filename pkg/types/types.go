package types

import (
	"time"
)

// Transaction represents a transaction in the blockchain
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

// NanoBlock represents a nano block in the blockchain
type NanoBlock struct {
	Height       uint64        `json:"height"`
	PrevHash     []byte        `json:"prev_hash"`
	Timestamp    time.Time     `json:"timestamp"`
	Transactions []Transaction `json:"transactions"`
	NodeID       string        `json:"node_id"`
	Signature    []byte        `json:"signature"`
	Hash         []byte        `json:"hash"`
	StateRoot    []byte        `json:"state_root"`
}

// MegaBlock represents a mega block containing multiple nano blocks
type MegaBlock struct {
	Height     uint64      `json:"height"`
	PrevHash   []byte      `json:"prev_hash"`
	Timestamp  time.Time   `json:"timestamp"`
	NanoBlocks []NanoBlock `json:"nano_blocks"`
	Signatures [][]byte    `json:"signatures"`
	Hash       []byte      `json:"hash"`
}