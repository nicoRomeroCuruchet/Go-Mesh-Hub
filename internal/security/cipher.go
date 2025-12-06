package security

import (
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"io"

	"golang.org/x/crypto/chacha20poly1305"
)

type Manager struct {
	aead cipher.AEAD
}

// New creates a new security manager with the hashed secret
func New(secret string) (*Manager, error) {
	key := sha256.Sum256([]byte(secret))
	aead, err := chacha20poly1305.New(key[:])
	if err != nil {
		return nil, err
	}
	return &Manager{aead: aead}, nil
}

// Encrypt generates a nonce, encrypts data, and prepends the nonce
func (m *Manager) Encrypt(plaintext []byte) ([]byte, error) {
	nonce := make([]byte, m.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return m.aead.Seal(nil, nonce, plaintext, nil), nil
}

// PackAndEncrypt combines nonce generation and appending into one step for transmission
func (m *Manager) PackAndEncrypt(plaintext []byte) ([]byte, error) {
	nonce := make([]byte, m.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	// Seal appends encrypted data to the nonce slice
	return m.aead.Seal(nonce, nonce, plaintext, nil), nil
}

// DecryptUnpack extracts the nonce and decrypts the payload
func (m *Manager) DecryptUnpack(packet []byte) ([]byte, error) {
	nonceSize := m.aead.NonceSize()
	if len(packet) < nonceSize {
		return nil, errors.New("packet too short")
	}

	nonce := packet[:nonceSize]
	ciphertext := packet[nonceSize:]

	return m.aead.Open(nil, nonce, ciphertext, nil)
}