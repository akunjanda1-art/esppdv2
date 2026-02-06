package cryptox

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
)

var ErrCiphertextTooShort = errors.New("ciphertext too short")

type AESGCM struct {
	gcm cipher.AEAD
}

func NewAESGCM(key []byte) (*AESGCM, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("AES-256-GCM requires 32-byte key, got %d", len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &AESGCM{gcm: gcm}, nil
}

func NewAESGCMFromBase64(keyB64 string) (*AESGCM, error) {
	key, err := base64.StdEncoding.DecodeString(keyB64)
	if err != nil {
		return nil, fmt.Errorf("decode base64 key: %w", err)
	}
	return NewAESGCM(key)
}

// Encrypt returns nonce||ciphertext.
func (a *AESGCM) Encrypt(plaintext []byte, aad []byte) ([]byte, error) {
	nonce := make([]byte, a.gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("generate nonce: %w", err)
	}
	sealed := a.gcm.Seal(nil, nonce, plaintext, aad)
	out := make([]byte, 0, len(nonce)+len(sealed))
	out = append(out, nonce...)
	out = append(out, sealed...)
	return out, nil
}

func (a *AESGCM) Decrypt(ciphertext []byte, aad []byte) ([]byte, error) {
	ns := a.gcm.NonceSize()
	if len(ciphertext) < ns {
		return nil, ErrCiphertextTooShort
	}
	nonce := ciphertext[:ns]
	sealed := ciphertext[ns:]
	pt, err := a.gcm.Open(nil, nonce, sealed, aad)
	if err != nil {
		return nil, err
	}
	return pt, nil
}

func (a *AESGCM) EncryptString(plaintext string, aad []byte) ([]byte, error) {
	return a.Encrypt([]byte(plaintext), aad)
}

func (a *AESGCM) DecryptString(ciphertext []byte, aad []byte) (string, error) {
	pt, err := a.Decrypt(ciphertext, aad)
	if err != nil {
		return "", err
	}
	return string(pt), nil
}
