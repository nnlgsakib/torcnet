package keystore

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/torcnet/torcnet/pb"
	"golang.org/x/crypto/pbkdf2"
	"google.golang.org/protobuf/proto"
)

// KeyStore manages encrypted key files
type KeyStore struct {
	keyDir string
}

// EncryptedKey, CryptoKey, CipherParams, and KDFParams are now defined in pb/account.proto
// Remove the old struct definitions as they're replaced by protobuf

// NewKeyStore creates a new keystore
func NewKeyStore(keyDir string) *KeyStore {
	return &KeyStore{
		keyDir: keyDir,
	}
}

// StoreKey encrypts and stores a private key
func (ks *KeyStore) StoreKey(privateKey *ecdsa.PrivateKey, password string) (string, error) {
	// Create key directory if it doesn't exist
	if err := os.MkdirAll(ks.keyDir, 0700); err != nil {
		return "", err
	}

	// Generate address from private key
	address := crypto.PubkeyToAddress(privateKey.PublicKey).Hex()

	// Generate salt and IV
	salt := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return "", err
	}

	iv := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		return "", err
	}

	// Derive key using PBKDF2
	derivedKey := pbkdf2.Key([]byte(password), salt, 4096, 32, sha256.New)

	// Encrypt private key
	privateKeyBytes := crypto.FromECDSA(privateKey)
	block, err := aes.NewCipher(derivedKey[:16])
	if err != nil {
		return "", err
	}

	cipherText := make([]byte, len(privateKeyBytes))
	stream := cipher.NewCTR(block, iv)
	stream.XORKeyStream(cipherText, privateKeyBytes)

	// Calculate MAC
	mac := sha256.Sum256(append(derivedKey[16:32], cipherText...))

	// Create encrypted key structure
	encryptedKey := &pb.EncryptedKey{
		Address: address,
		Crypto: &pb.CryptoKey{
			Cipher:     "aes-128-ctr",
			CipherText: hex.EncodeToString(cipherText),
			CipherParams: &pb.CipherParams{
				Iv: hex.EncodeToString(iv),
			},
			Kdf: "pbkdf2",
			KdfParams: &pb.KDFParams{
				Dklen: 32,
				N:     4096,
				P:     1,
				R:     1,
				Salt:  hex.EncodeToString(salt),
			},
			Mac: hex.EncodeToString(mac[:]),
		},
		Id:        generateUUID(),
		Version:   3,
		Timestamp: time.Now().Unix(),
	}

	// Marshal to protobuf
	protoData, err := proto.Marshal(encryptedKey)
	if err != nil {
		return "", err
	}

	// Generate filename
	filename := fmt.Sprintf("UTC--%s--%s",
		time.Now().Format("2006-01-02T15-04-05.000000000Z"),
		strings.ToLower(address[2:]))

	keyPath := filepath.Join(ks.keyDir, filename)

	// Write to file
	if err := ioutil.WriteFile(keyPath, protoData, 0600); err != nil {
		return "", err
	}

	return keyPath, nil
}

// LoadKey decrypts and loads a private key
func (ks *KeyStore) LoadKey(keyPath, password string) (*ecdsa.PrivateKey, error) {
	// Read file
	protoData, err := ioutil.ReadFile(keyPath)
	if err != nil {
		return nil, err
	}

	// Unmarshal protobuf
	var encryptedKey pb.EncryptedKey
	if err := proto.Unmarshal(protoData, &encryptedKey); err != nil {
		return nil, err
	}

	// Decode parameters
	salt, err := hex.DecodeString(encryptedKey.Crypto.KdfParams.Salt)
	if err != nil {
		return nil, err
	}

	iv, err := hex.DecodeString(encryptedKey.Crypto.CipherParams.Iv)
	if err != nil {
		return nil, err
	}

	cipherText, err := hex.DecodeString(encryptedKey.Crypto.CipherText)
	if err != nil {
		return nil, err
	}

	mac, err := hex.DecodeString(encryptedKey.Crypto.Mac)
	if err != nil {
		return nil, err
	}

	// Derive key
	derivedKey := pbkdf2.Key([]byte(password), salt, int(encryptedKey.Crypto.KdfParams.N), 32, sha256.New)

	// Verify MAC
	expectedMAC := sha256.Sum256(append(derivedKey[16:32], cipherText...))
	if hex.EncodeToString(expectedMAC[:]) != hex.EncodeToString(mac) {
		return nil, errors.New("invalid password or corrupted key file")
	}

	// Decrypt private key
	block, err := aes.NewCipher(derivedKey[:16])
	if err != nil {
		return nil, err
	}

	privateKeyBytes := make([]byte, len(cipherText))
	stream := cipher.NewCTR(block, iv)
	stream.XORKeyStream(privateKeyBytes, cipherText)

	// Convert to ECDSA private key
	privateKey, err := crypto.ToECDSA(privateKeyBytes)
	if err != nil {
		return nil, err
	}

	return privateKey, nil
}

// ListKeys returns all key files in the keystore
func (ks *KeyStore) ListKeys() ([]string, error) {
	files, err := ioutil.ReadDir(ks.keyDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}

	var keyFiles []string
	for _, file := range files {
		if !file.IsDir() && strings.HasPrefix(file.Name(), "UTC--") {
			keyFiles = append(keyFiles, filepath.Join(ks.keyDir, file.Name()))
		}
	}

	return keyFiles, nil
}

// generateUUID generates a simple UUID for key identification
func generateUUID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
