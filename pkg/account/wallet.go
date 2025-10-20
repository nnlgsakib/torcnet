package account

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
)

// Wallet manages multiple accounts
type Wallet struct {
	Accounts     map[string]*Account
	DefaultAccount string
	mu          sync.RWMutex
	walletPath  string
}

// WalletData represents the data structure for wallet storage
type WalletData struct {
	Accounts     []AccountData `json:"accounts"`
	DefaultAccount string      `json:"default_account"`
	CreatedAt    int64         `json:"created_at"`
	UpdatedAt    int64         `json:"updated_at"`
}

// AccountData represents the data structure for account storage
type AccountData struct {
	Address    string `json:"address"`
	PrivateKey string `json:"private_key"`
	CreatedAt  int64  `json:"created_at"`
}

// NewWallet creates a new wallet
func NewWallet(walletPath string) (*Wallet, error) {
	wallet := &Wallet{
		Accounts:    make(map[string]*Account),
		walletPath:  walletPath,
	}

	// Create wallet directory if it doesn't exist
	if err := os.MkdirAll(filepath.Dir(walletPath), 0700); err != nil {
		return nil, err
	}

	// Load wallet if it exists
	if _, err := os.Stat(walletPath); err == nil {
		if err := wallet.Load(); err != nil {
			return nil, err
		}
	}

	return wallet, nil
}

// CreateAccount creates a new account and adds it to the wallet
func (w *Wallet) CreateAccount() (*Account, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	account, err := NewAccount()
	if err != nil {
		return nil, err
	}

	w.Accounts[account.Address] = account

	// Set as default if it's the first account
	if len(w.Accounts) == 1 {
		w.DefaultAccount = account.Address
	}

	// Save wallet
	if err := w.Save(); err != nil {
		return nil, err
	}

	return account, nil
}

// ImportAccount imports an account from a private key
func (w *Wallet) ImportAccount(privateKeyHex string) (*Account, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	account, err := ImportFromPrivateKeyHex(privateKeyHex)
	if err != nil {
		return nil, err
	}

	// Check if account already exists
	if _, exists := w.Accounts[account.Address]; exists {
		return nil, errors.New("account already exists in wallet")
	}

	w.Accounts[account.Address] = account

	// Set as default if it's the first account
	if len(w.Accounts) == 1 {
		w.DefaultAccount = account.Address
	}

	// Save wallet
	if err := w.Save(); err != nil {
		return nil, err
	}

	return account, nil
}

// GetAccount returns an account by address
func (w *Wallet) GetAccount(address string) (*Account, error) {
	w.mu.RLock()
	defer w.mu.RUnlock()

	account, exists := w.Accounts[address]
	if !exists {
		return nil, errors.New("account not found")
	}

	return account, nil
}

// GetDefaultAccount returns the default account
func (w *Wallet) GetDefaultAccount() (*Account, error) {
	w.mu.RLock()
	defer w.mu.RUnlock()

	if w.DefaultAccount == "" {
		return nil, errors.New("no default account set")
	}

	return w.GetAccount(w.DefaultAccount)
}

// SetDefaultAccount sets the default account
func (w *Wallet) SetDefaultAccount(address string) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if _, exists := w.Accounts[address]; !exists {
		return errors.New("account not found")
	}

	w.DefaultAccount = address

	// Save wallet
	return w.Save()
}

// ListAccounts returns a list of all accounts
func (w *Wallet) ListAccounts() []string {
	w.mu.RLock()
	defer w.mu.RUnlock()

	accounts := make([]string, 0, len(w.Accounts))
	for address := range w.Accounts {
		accounts = append(accounts, address)
	}

	return accounts
}

// RemoveAccount removes an account from the wallet
func (w *Wallet) RemoveAccount(address string) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if _, exists := w.Accounts[address]; !exists {
		return errors.New("account not found")
	}

	delete(w.Accounts, address)

	// Update default account if needed
	if w.DefaultAccount == address {
		if len(w.Accounts) > 0 {
			for addr := range w.Accounts {
				w.DefaultAccount = addr
				break
			}
		} else {
			w.DefaultAccount = ""
		}
	}

	// Save wallet
	return w.Save()
}

// Save saves the wallet to disk
func (w *Wallet) Save() error {
	// Convert wallet to storage format
	data := WalletData{
		Accounts:     make([]AccountData, 0, len(w.Accounts)),
		DefaultAccount: w.DefaultAccount,
		UpdatedAt:    time.Now().Unix(),
	}

	for _, account := range w.Accounts {
		accountData := AccountData{
			Address:    account.Address,
			PrivateKey: account.ExportPrivateKeyHex(),
			CreatedAt:  time.Now().Unix(),
		}
		data.Accounts = append(data.Accounts, accountData)
	}

	// Marshal to JSON
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}

	// Write to file
	return ioutil.WriteFile(w.walletPath, jsonData, 0600)
}

// Load loads the wallet from disk
func (w *Wallet) Load() error {
	// Read file
	jsonData, err := ioutil.ReadFile(w.walletPath)
	if err != nil {
		return err
	}

	// Unmarshal JSON
	var data WalletData
	if err := json.Unmarshal(jsonData, &data); err != nil {
		return err
	}

	// Clear existing accounts
	w.Accounts = make(map[string]*Account)

	// Load accounts
	for _, accountData := range data.Accounts {
		privateKey, err := crypto.HexToECDSA(accountData.PrivateKey)
		if err != nil {
			return fmt.Errorf("invalid private key for address %s: %v", accountData.Address, err)
		}

		account, err := NewAccountFromPrivateKey(privateKey)
		if err != nil {
			return err
		}

		// Verify address matches
		if account.Address != accountData.Address {
			return fmt.Errorf("address mismatch for account %s", accountData.Address)
		}

		w.Accounts[account.Address] = account
	}

	// Set default account
	w.DefaultAccount = data.DefaultAccount

	return nil
}