package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const (
	// MasterKeyFile is the filename for the master encryption key.
	MasterKeyFile = "master.key"
	// MasterKeySize is the key size in bytes (AES-256).
	MasterKeySize = 32
)

// MasterKeyProvider manages the master encryption key used for encrypting secrets at rest.
type MasterKeyProvider struct {
	keyPath string
	key     []byte
}

// NewMasterKeyProvider creates a provider that loads or generates the master key
// from the given kandev config directory.
func NewMasterKeyProvider(kandevDir string) (*MasterKeyProvider, error) {
	keyPath := filepath.Join(kandevDir, MasterKeyFile)
	provider := &MasterKeyProvider{keyPath: keyPath}

	if err := provider.loadOrGenerate(); err != nil {
		return nil, fmt.Errorf("master key init: %w", err)
	}
	return provider, nil
}

func (p *MasterKeyProvider) loadOrGenerate() error {
	data, err := os.ReadFile(p.keyPath)
	if err == nil && len(data) == MasterKeySize {
		p.key = data
		return nil
	}

	key := make([]byte, MasterKeySize)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return fmt.Errorf("generate key: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(p.keyPath), 0700); err != nil {
		return fmt.Errorf("create key dir: %w", err)
	}

	if err := os.WriteFile(p.keyPath, key, 0600); err != nil {
		return fmt.Errorf("write key: %w", err)
	}

	p.key = key
	return nil
}

// Key returns the master key bytes.
func (p *MasterKeyProvider) Key() []byte {
	return p.key
}

// Encrypt encrypts plaintext using AES-256-GCM with a random nonce.
// Returns (ciphertext, nonce, error).
func Encrypt(plaintext, key []byte) ([]byte, []byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, nil, fmt.Errorf("create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, nil, fmt.Errorf("create GCM: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, nil, fmt.Errorf("generate nonce: %w", err)
	}

	ciphertext := gcm.Seal(nil, nonce, plaintext, nil)
	return ciphertext, nonce, nil
}

// Decrypt decrypts ciphertext using AES-256-GCM.
func Decrypt(ciphertext, nonce, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create GCM: %w", err)
	}

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("decrypt: %w", err)
	}

	return plaintext, nil
}
