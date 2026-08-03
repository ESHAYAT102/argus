package secrets

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestEncryptRoundTrip(t *testing.T) {
	key := base64.StdEncoding.EncodeToString([]byte(strings.Repeat("k", 32)))
	cipher, err := New(key)
	if err != nil {
		t.Fatal(err)
	}
	context := []byte("environment-id:variable-name")
	encrypted, nonce, err := cipher.Encrypt([]byte("secret"), context)
	if err != nil {
		t.Fatal(err)
	}
	if string(encrypted) == "secret" {
		t.Fatal("ciphertext contains plaintext")
	}
	plaintext, err := cipher.Decrypt(encrypted, nonce, context)
	if err != nil {
		t.Fatal(err)
	}
	if string(plaintext) != "secret" {
		t.Fatalf("got %q", plaintext)
	}
	if _, err := cipher.Decrypt(encrypted, nonce, []byte("wrong-context")); err == nil {
		t.Fatal("expected context mismatch to fail")
	}
}
