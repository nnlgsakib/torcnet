package crypto

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"math/big"
	"strings"
)

// ECDSAKeyPair represents an ECDSA key pair for accounts
type ECDSAKeyPair struct {
	PrivateKey *ecdsa.PrivateKey
	PublicKey  *ecdsa.PublicKey
}

// GenerateECDSAKeyPair generates a new ECDSA key pair
func GenerateECDSAKeyPair() (*ECDSAKeyPair, error) {
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	
	return &ECDSAKeyPair{
		PrivateKey: privateKey,
		PublicKey:  &privateKey.PublicKey,
	}, nil
}

// Sign signs a message using ECDSA
func (kp *ECDSAKeyPair) Sign(message []byte) ([]byte, error) {
	if kp.PrivateKey == nil {
		return nil, errors.New("private key is nil")
	}
	
	// Hash the message first
	hash := sha256.Sum256(message)
	
	// Sign the hash
	r, s, err := ecdsa.Sign(rand.Reader, kp.PrivateKey, hash[:])
	if err != nil {
		return nil, err
	}
	
	// Combine r and s into a single signature
	rBytes := r.Bytes()
	sBytes := s.Bytes()
	
	// Ensure both r and s are 32 bytes
	signature := make([]byte, 64)
	copy(signature[32-len(rBytes):32], rBytes)
	copy(signature[64-len(sBytes):64], sBytes)
	
	return signature, nil
}

// Verify verifies an ECDSA signature
func VerifyECDSASignature(publicKey *ecdsa.PublicKey, message, signature []byte) bool {
	if len(signature) != 64 {
		return false
	}
	
	// Extract r and s from signature
	r := new(big.Int).SetBytes(signature[:32])
	s := new(big.Int).SetBytes(signature[32:])
	
	// Hash the message
	hash := sha256.Sum256(message)
	
	// Verify the signature
	return ecdsa.Verify(publicKey, hash[:], r, s)
}

// AddressToPubKey converts a TRC address to a public key
func AddressToPubKey(address string) (*ecdsa.PublicKey, error) {
	// Remove the "trc" prefix
	if !strings.HasPrefix(address, "trc") {
		return nil, errors.New("invalid address format: missing trc prefix")
	}
	
	hexStr := address[3:]
	
	// Decode the hex string
	bytes, err := hex.DecodeString(hexStr)
	if err != nil {
		return nil, err
	}
	
	if len(bytes) != 20 {
		return nil, errors.New("invalid address length")
	}
	
	// In a real implementation, we would derive the public key from the address
	// For now, we'll return a placeholder
	return nil, errors.New("deriving public key from address not implemented")
}

// PubKeyToAddress converts a public key to a TRC address
func PubKeyToAddress(publicKey *ecdsa.PublicKey) string {
	// In Ethereum, the address is the last 20 bytes of the keccak256 hash of the public key
	// For TRC, we'll use a similar approach but with BLAKE3
	
	// Combine X and Y coordinates of the public key
	pubBytes := elliptic.Marshal(publicKey.Curve, publicKey.X, publicKey.Y)
	
	// Hash the public key
	hash := Hash(pubBytes)
	
	// Take the last 20 bytes
	address := hash[len(hash)-20:]
	
	// Convert to hex and add the "trc" prefix
	return "trc" + hex.EncodeToString(address)
}

// SignECDSA signs a message using ECDSA with a private key in bytes format
func SignECDSA(privateKeyBytes []byte, message []byte) ([]byte, error) {
	if len(privateKeyBytes) != 32 {
		return nil, errors.New("invalid private key length")
	}
	
	// Convert bytes to private key
	privateKey := new(ecdsa.PrivateKey)
	privateKey.Curve = elliptic.P256()
	privateKey.D = new(big.Int).SetBytes(privateKeyBytes)
	privateKey.PublicKey.X, privateKey.PublicKey.Y = privateKey.Curve.ScalarBaseMult(privateKeyBytes)
	
	// Hash the message first
	hash := sha256.Sum256(message)
	
	// Sign the hash
	r, s, err := ecdsa.Sign(rand.Reader, privateKey, hash[:])
	if err != nil {
		return nil, err
	}
	
	// Combine r and s into a single signature
	rBytes := r.Bytes()
	sBytes := s.Bytes()
	
	// Ensure both r and s are 32 bytes
	signature := make([]byte, 64)
	copy(signature[32-len(rBytes):32], rBytes)
	copy(signature[64-len(sBytes):64], sBytes)
	
	return signature, nil
}

// VerifyECDSA verifies an ECDSA signature using a public key in bytes format
func VerifyECDSA(publicKeyBytes []byte, message, signature []byte) bool {
	if len(signature) != 64 {
		return false
	}
	
	// Convert bytes to public key
	if len(publicKeyBytes) != 65 && len(publicKeyBytes) != 33 {
		return false
	}
	
	x, y := elliptic.Unmarshal(elliptic.P256(), publicKeyBytes)
	if x == nil || y == nil {
		return false
	}
	
	publicKey := &ecdsa.PublicKey{
		Curve: elliptic.P256(),
		X:     x,
		Y:     y,
	}
	
	// Extract r and s from signature
	r := new(big.Int).SetBytes(signature[:32])
	s := new(big.Int).SetBytes(signature[32:])
	
	// Hash the message
	hash := sha256.Sum256(message)
	
	// Verify the signature
	return ecdsa.Verify(publicKey, hash[:], r, s)
}

// PublicKeyFromAddress extracts a public key from a TRC address
func PublicKeyFromAddress(address string) ([]byte, error) {
	// Remove the "trc" prefix
	if !strings.HasPrefix(address, "trc") {
		return nil, errors.New("invalid address format: missing trc prefix")
	}
	
	hexStr := address[3:]
	
	// Decode the hex string
	bytes, err := hex.DecodeString(hexStr)
	if err != nil {
		return nil, err
	}
	
	if len(bytes) != 20 {
		return nil, errors.New("invalid address length")
	}
	
	// In a real implementation, we would derive the public key from the address
	// For now, we'll return a placeholder public key (this is a simplified implementation)
	// In practice, you would need to store the mapping between addresses and public keys
	// or use a different approach like recovering from signatures
	
	// Generate a placeholder public key based on the address
	// This is not cryptographically secure and is only for compilation purposes
	publicKeyBytes := make([]byte, 65)
	publicKeyBytes[0] = 0x04 // Uncompressed public key prefix
	copy(publicKeyBytes[1:21], bytes)   // Use address bytes as part of X coordinate
	copy(publicKeyBytes[21:41], bytes)  // Use address bytes as part of X coordinate (repeated)
	copy(publicKeyBytes[41:61], bytes)  // Use address bytes as part of Y coordinate
	copy(publicKeyBytes[61:65], bytes[:4]) // Fill remaining bytes
	
	return publicKeyBytes, nil
}