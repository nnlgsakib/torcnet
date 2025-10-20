package account

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/torcnet/torcnet/pb"
	"github.com/torcnet/torcnet/pkg/keystore"
	"google.golang.org/protobuf/proto"
)

// Wallet manages multiple accounts
type Wallet struct {
	Accounts       map[string]*Account
	DefaultAccount string
	mu             sync.RWMutex
	walletPath     string
	keyStore       *keystore.KeyStore
}

// WalletData and AccountData are now defined in pb/account.proto
// Remove the old struct definitions as they're replaced by protobuf

// NewWallet creates a new wallet
func NewWallet(walletPath string) (*Wallet, error) {
	wallet := &Wallet{
		Accounts:   make(map[string]*Account),
		walletPath: walletPath,
		keyStore:   keystore.NewKeyStore(filepath.Join(filepath.Dir(walletPath), "keystore")),
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

	// Store encrypted private key
	keyPath, err := w.keyStore.StoreKey(account.PrivateKey, "") // Empty password for now
	if err != nil {
		return nil, fmt.Errorf("failed to store key: %v", err)
	}

	w.Accounts[account.Address] = account

	// Set as default if it's the first account
	if len(w.Accounts) == 1 {
		w.DefaultAccount = account.Address
	}

	// Save wallet with keyfile reference
	if err := w.saveWithKeyFile(account.Address, keyPath); err != nil {
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

	// Store encrypted private key
	keyPath, err := w.keyStore.StoreKey(account.PrivateKey, "") // Empty password for now
	if err != nil {
		return nil, fmt.Errorf("failed to store key: %v", err)
	}

	w.Accounts[account.Address] = account

	// Set as default if it's the first account
	if len(w.Accounts) == 1 {
		w.DefaultAccount = account.Address
	}

	// Save wallet with keyfile reference
	if err := w.saveWithKeyFile(account.Address, keyPath); err != nil {
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

// saveWithKeyFile saves wallet data with keyfile reference for a specific account
func (w *Wallet) saveWithKeyFile(address, keyPath string) error {
	// Load existing wallet data
	var data *pb.WalletData
	if protoData, err := ioutil.ReadFile(w.walletPath); err == nil {
		data = &pb.WalletData{}
		if err := proto.Unmarshal(protoData, data); err != nil {
			// If unmarshal fails, create new data
			data = &pb.WalletData{
				Accounts:       make([]*pb.AccountData, 0),
				DefaultAccount: w.DefaultAccount,
				CreatedAt:      time.Now().Unix(),
			}
		}
	} else {
		data = &pb.WalletData{
			Accounts:       make([]*pb.AccountData, 0),
			DefaultAccount: w.DefaultAccount,
			CreatedAt:      time.Now().Unix(),
		}
	}

	// Update or add account data
	found := false
	for _, accountData := range data.Accounts {
		if accountData.Address == address {
			accountData.KeyFile = keyPath
			found = true
			break
		}
	}

	if !found {
		accountData := &pb.AccountData{
			Address:   address,
			KeyFile:   keyPath,
			CreatedAt: time.Now().Unix(),
		}
		data.Accounts = append(data.Accounts, accountData)
	}

	data.DefaultAccount = w.DefaultAccount
	data.UpdatedAt = time.Now().Unix()

	// Marshal to protobuf
	protoData, err := proto.Marshal(data)
	if err != nil {
		return err
	}

	// Write to file
	return ioutil.WriteFile(w.walletPath, protoData, 0600)
}

// Save saves the wallet to disk (deprecated - use saveWithKeyFile for new accounts)
func (w *Wallet) Save() error {
	// Convert wallet to storage format
	data := &pb.WalletData{
		Accounts:       make([]*pb.AccountData, 0, len(w.Accounts)),
		DefaultAccount: w.DefaultAccount,
		UpdatedAt:      time.Now().Unix(),
	}

	for _, account := range w.Accounts {
		accountData := &pb.AccountData{
			Address:   account.Address,
			KeyFile:   "", // No keyfile for legacy accounts
			CreatedAt: time.Now().Unix(),
		}
		data.Accounts = append(data.Accounts, accountData)
	}

	// Marshal to protobuf
	protoData, err := proto.Marshal(data)
	if err != nil {
		return err
	}

	// Write to file
	return ioutil.WriteFile(w.walletPath, protoData, 0600)
}

// Load loads the wallet from disk
func (w *Wallet) Load() error {
	// Read file
	protoData, err := ioutil.ReadFile(w.walletPath)
	if err != nil {
		return err
	}

	// Unmarshal protobuf
	var data pb.WalletData
	if err := proto.Unmarshal(protoData, &data); err != nil {
		return err
	}

	// Clear existing accounts
	w.Accounts = make(map[string]*Account)

	// Load accounts
	for _, accountData := range data.Accounts {
		var account *Account

		if accountData.KeyFile != "" {
			// Load from encrypted keyfile
			privateKey, err := w.keyStore.LoadKey(accountData.KeyFile, "") // Empty password for now
			if err != nil {
				return fmt.Errorf("failed to load key from %s: %v", accountData.KeyFile, err)
			}

			account, err = NewAccountFromPrivateKey(privateKey)
			if err != nil {
				return err
			}
		} else {
			// Legacy: load from plain private key (if still present in old format)
			// This is for backward compatibility only
			return fmt.Errorf("legacy private key format no longer supported for address %s", accountData.Address)
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
