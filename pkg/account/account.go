package account

import (
	"crypto/ecdsa"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/zeebo/blake3"
)

const (
	// AddressPrefix is the prefix for EVM-compatible addresses
	AddressPrefix = "0x"
	
	// AddressLength is the length of the address in bytes (same as Ethereum)
	AddressLength = 20
)

// Account represents a user account
type Account struct {
	PrivateKey *ecdsa.PrivateKey
	PublicKey  *ecdsa.PublicKey
	Address    string
}

// NewAccount creates a new account
func NewAccount() (*Account, error) {
	// Generate ECDSA key pair (same curve as Ethereum)
	privateKey, err := crypto.GenerateKey()
	if err != nil {
		return nil, err
	}

	// Create account from private key
	return NewAccountFromPrivateKey(privateKey)
}

// NewAccountFromPrivateKey creates a new account from a private key
func NewAccountFromPrivateKey(privateKey *ecdsa.PrivateKey) (*Account, error) {
	if privateKey == nil {
		return nil, errors.New("private key is nil")
	}

	publicKey := privateKey.Public().(*ecdsa.PublicKey)
	address := PublicKeyToAddress(publicKey)

	return &Account{
		PrivateKey: privateKey,
		PublicKey:  publicKey,
		Address:    address,
	}, nil
}

// PublicKeyToAddress converts a public key to an address
func PublicKeyToAddress(publicKey *ecdsa.PublicKey) string {
	// Convert public key to bytes (uncompressed format)
	pubBytes := crypto.FromECDSAPub(publicKey)
	
	// Use Ethereum's Keccak256 hash instead of BLAKE3 for EVM compatibility
	hash := crypto.Keccak256(pubBytes[1:]) // Skip the first byte (0x04 prefix)
	
	// Take the last 20 bytes (same as Ethereum)
	address := hash[len(hash)-AddressLength:]
	
	// Convert to hex and add 0x prefix
	return fmt.Sprintf("%s%s", AddressPrefix, hex.EncodeToString(address))
}

// AddressFromHex creates an address from a hex string
func AddressFromHex(hexStr string) (string, error) {
	// Remove 0x prefix if present
	if strings.HasPrefix(hexStr, "0x") {
		hexStr = hexStr[2:]
	}
	
	// Validate hex string length
	if len(hexStr) != AddressLength*2 {
		return "", fmt.Errorf("invalid address length: expected %d characters, got %d", AddressLength*2, len(hexStr))
	}
	
	// Validate hex format
	_, err := hex.DecodeString(hexStr)
	if err != nil {
		return "", fmt.Errorf("invalid hex format: %v", err)
	}
	
	// Return with 0x prefix
	return fmt.Sprintf("%s%s", AddressPrefix, hexStr), nil
}

// ImportFromPrivateKeyHex imports an account from a hex-encoded private key
func ImportFromPrivateKeyHex(hexKey string) (*Account, error) {
	// Decode hex
	privateKey, err := crypto.HexToECDSA(hexKey)
	if err != nil {
		return nil, err
	}
	
	return NewAccountFromPrivateKey(privateKey)
}

// ExportPrivateKeyHex exports the private key as a hex string
func (a *Account) ExportPrivateKeyHex() string {
	return hex.EncodeToString(crypto.FromECDSA(a.PrivateKey))
}

// Sign signs data with the account's private key
func (a *Account) Sign(data []byte) ([]byte, error) {
	// Hash the data using BLAKE3
	hasher := blake3.New()
	hasher.Write(data)
	hash := hasher.Sum(nil)
	
	// Sign the hash
	return crypto.Sign(hash, a.PrivateKey)
}

// VerifySignature verifies a signature
func VerifySignature(pubKey *ecdsa.PublicKey, data, signature []byte) bool {
	// Hash the data using BLAKE3
	hasher := blake3.New()
	hasher.Write(data)
	hash := hasher.Sum(nil)
	
	// Verify the signature
	return crypto.VerifySignature(crypto.FromECDSAPub(pubKey), hash, signature[:64])
}

// IsValidAddress checks if an address is valid
func IsValidAddress(address string) bool {
	// Check prefix
	if !strings.HasPrefix(address, AddressPrefix) {
		return false
	}
	
	// Remove prefix
	hexAddress := strings.TrimPrefix(address, AddressPrefix)
	
	// Check length
	if len(hexAddress) != AddressLength*2 {
		return false
	}
	
	// Check if it's valid hex
	_, err := hex.DecodeString(hexAddress)
	return err == nil
}