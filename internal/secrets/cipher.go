package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
)

type Cipher struct {
	aead cipher.AEAD
}

func New(encodedKey string) (*Cipher, error) {
	if encodedKey == "" {
		return nil, errors.New("ARGUS_ENCRYPTION_KEY is required")
	}
	key, err := base64.StdEncoding.DecodeString(encodedKey)
	if err != nil {
		return nil, errors.New("ARGUS_ENCRYPTION_KEY must be base64 encoded")
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("ARGUS_ENCRYPTION_KEY must decode to 32 bytes, got %d", len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Cipher{aead: aead}, nil
}

func (cipher *Cipher) Encrypt(plaintext []byte, context []byte) (encrypted, nonce []byte, err error) {
	nonce = make([]byte, cipher.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, nil, err
	}
	encrypted = cipher.aead.Seal(nil, nonce, plaintext, context)
	return encrypted, nonce, nil
}

func (cipher *Cipher) Decrypt(encrypted, nonce, context []byte) ([]byte, error) {
	plaintext, err := cipher.aead.Open(nil, nonce, encrypted, context)
	if err != nil {
		return nil, errors.New("decrypt secret: authentication failed")
	}
	return plaintext, nil
}
