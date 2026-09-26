package converter

import (
	"bytes"
	"crypto/aes"
	"errors"
	"strconv"
	"testing"
)

var testKey = []byte("0123456789abcdef")

// payload returns deterministic bytes that do not repeat within the lengths
// these tests use.
func payload(size int) []byte {
	out := make([]byte, size)
	for i := range out {
		out[i] = byte(i*7 + 1)
	}
	return out
}

// encryptAES128ECB pads and encrypts. It lives in the test so that the inverse
// of the production decryption is implemented independently of it.
func encryptAES128ECB(t *testing.T, key, data []byte) []byte {
	t.Helper()
	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatalf("create cipher: %v", err)
	}
	blockSize := block.BlockSize()
	padding := blockSize - len(data)%blockSize
	padded := make([]byte, 0, len(data)+padding)
	padded = append(padded, data...)
	padded = append(padded, bytes.Repeat([]byte{byte(padding)}, padding)...)

	out := make([]byte, len(padded))
	for i := 0; i < len(padded); i += blockSize {
		block.Encrypt(out[i:i+blockSize], padded[i:i+blockSize])
	}
	return out
}

// encryptBlock encrypts one block without padding, so that tests can build
// ciphertext carrying deliberately invalid padding.
func encryptBlock(t *testing.T, key, block []byte) []byte {
	t.Helper()
	cipher, err := aes.NewCipher(key)
	if err != nil {
		t.Fatalf("create cipher: %v", err)
	}
	out := make([]byte, len(block))
	cipher.Encrypt(out, block)
	return out
}

func TestDecryptAES128ECBRoundTrip(t *testing.T) {
	for _, size := range []int{0, 1, 15, 16, 17, 31, 32, 33, 100, 4096} {
		t.Run(strconv.Itoa(size), func(t *testing.T) {
			plain := payload(size)
			decrypted, err := decryptAES128ECB(testKey, encryptAES128ECB(t, testKey, plain))
			if err != nil {
				t.Fatalf("decrypt: %v", err)
			}
			if !bytes.Equal(decrypted, plain) {
				t.Errorf("round trip returned %d bytes, want the %d that went in", len(decrypted), len(plain))
			}
		})
	}
}

func TestDecryptAES128ECBRejectsShortAndEmptyInput(t *testing.T) {
	for _, size := range []int{0, 15} {
		if _, err := decryptAES128ECB(testKey, payload(size)); err == nil {
			t.Errorf("decrypt succeeded on %d bytes, want an error", size)
		}
	}
}

func TestDecryptAES128ECBRejectsInvalidPadding(t *testing.T) {
	tests := []struct {
		name  string
		block []byte
	}{
		{"padding of zero", append(bytes.Repeat([]byte{0x11}, 15), 0x00)},
		{"padding larger than the block", append(bytes.Repeat([]byte{0x11}, 15), 0x11)},
		{"padding bytes that disagree", append(bytes.Repeat([]byte{0x11}, 11), 0x05, 0x11, 0x11, 0x11, 0x05)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := decryptAES128ECB(testKey, encryptBlock(t, testKey, tt.block))
			if !errors.Is(err, ErrInvalidPadding) {
				t.Fatalf("decrypt error = %v, want %v", err, ErrInvalidPadding)
			}
		})
	}
}

func TestBuildKeyBoxIsAPermutation(t *testing.T) {
	box, err := buildKeyBox([]byte("0123456789"))
	if err != nil {
		t.Fatalf("build key box: %v", err)
	}
	if len(box) != 256 {
		t.Fatalf("key box length = %d, want 256", len(box))
	}

	seen := make([]bool, 256)
	for _, value := range box {
		if seen[value] {
			t.Fatalf("key box repeats the value %d", value)
		}
		seen[value] = true
	}
}

func TestBuildKeyBoxIsDeterministic(t *testing.T) {
	first, err := buildKeyBox([]byte("0123456789"))
	if err != nil {
		t.Fatalf("build key box: %v", err)
	}
	second, err := buildKeyBox([]byte("0123456789"))
	if err != nil {
		t.Fatalf("build key box: %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Error("the same key produced two different key boxes")
	}

	other, err := buildKeyBox([]byte("fedcba9876543210"))
	if err != nil {
		t.Fatalf("build key box: %v", err)
	}
	if bytes.Equal(first, other) {
		t.Error("two different keys produced the same key box")
	}
}

func TestBuildKeyBoxRejectsEmptyKey(t *testing.T) {
	if _, err := buildKeyBox(nil); err == nil {
		t.Fatal("build key box succeeded on an empty key, want an error")
	}
}
