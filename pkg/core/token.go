package core

import (
	"errors"
	"math/big"
	"sync"
)

const (
	// TotalSupply is the total supply of TORC NET tokens (100 million)
	TotalSupply = 100000000

	// TokenDecimals is the number of decimal places for the token
	TokenDecimals = 18
)

// TokenState represents the token state
type TokenState struct {
	balances    map[string]*big.Int
	totalSupply *big.Int
	mu          sync.RWMutex
}

// NewTokenState creates a new token state
func NewTokenState() *TokenState {
	// Calculate total supply with decimals
	totalSupply := new(big.Int).Exp(big.NewInt(10), big.NewInt(TokenDecimals), nil)
	totalSupply = new(big.Int).Mul(totalSupply, big.NewInt(TotalSupply))

	return &TokenState{
		balances:    make(map[string]*big.Int),
		totalSupply: totalSupply,
	}
}

// InitGenesisAllocation initializes the genesis allocation of tokens
func (ts *TokenState) InitGenesisAllocation(genesisAddress string) error {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	// Allocate all tokens to the genesis address
	ts.balances[genesisAddress] = new(big.Int).Set(ts.totalSupply)

	return nil
}

// Transfer transfers tokens from one address to another
func (ts *TokenState) Transfer(from, to string, amount *big.Int) error {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	// Check if sender has enough balance
	senderBalance, ok := ts.balances[from]
	if !ok || senderBalance.Cmp(amount) < 0 {
		return errors.New("insufficient balance")
	}

	// Update sender balance
	ts.balances[from] = new(big.Int).Sub(senderBalance, amount)

	// Update receiver balance
	receiverBalance, ok := ts.balances[to]
	if !ok {
		receiverBalance = big.NewInt(0)
	}
	ts.balances[to] = new(big.Int).Add(receiverBalance, amount)

	return nil
}

// GetBalance returns the balance of an address
func (ts *TokenState) GetBalance(address string) *big.Int {
	ts.mu.RLock()
	defer ts.mu.RUnlock()

	balance, ok := ts.balances[address]
	if !ok {
		return big.NewInt(0)
	}
	return new(big.Int).Set(balance)
}

// GetTotalSupply returns the total supply of tokens
func (ts *TokenState) GetTotalSupply() *big.Int {
	return new(big.Int).Set(ts.totalSupply)
}

// FormatAmount formats an amount with the correct number of decimals
func FormatAmount(amount *big.Int) string {
	// Convert to a decimal string with the correct number of decimals
	// This is a simplified implementation
	return amount.String()
}