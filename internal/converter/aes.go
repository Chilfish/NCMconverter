package converter

import (
	"crypto/aes"
	"errors"
	"fmt"
)

// ErrInvalidPadding reports that decrypted data does not end with the PKCS#7
// padding that this format always appends.
var ErrInvalidPadding = errors.New("invalid PKCS#7 padding")

// decryptAES128ECB decrypts data with AES-128 in ECB mode and removes the
// trailing PKCS#7 padding.
func decryptAES128ECB(key, data []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create AES cipher: %w", err)
	}
	blockSize := block.BlockSize()
	if len(data) < blockSize {
		return nil, fmt.Errorf("decrypt AES-128-ECB: need at least %d bytes, got %d", blockSize, len(data))
	}

	padded := len(data) / blockSize * blockSize
	decrypted := make([]byte, padded)
	for i := 0; i < padded; i += blockSize {
		block.Decrypt(decrypted[i:i+blockSize], data[i:i+blockSize])
	}
	return unpadPKCS7(decrypted)
}

// unpadPKCS7 validates and removes the trailing PKCS#7 padding of a block
// aligned slice.
func unpadPKCS7(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, ErrInvalidPadding
	}
	padding := int(data[len(data)-1])
	if padding == 0 || padding > aes.BlockSize || padding > len(data) {
		return nil, fmt.Errorf("%w: %d trailing bytes claimed", ErrInvalidPadding, padding)
	}
	for _, b := range data[len(data)-padding:] {
		if int(b) != padding {
			return nil, fmt.Errorf("%w: byte %#x found in %d byte padding", ErrInvalidPadding, b, padding)
		}
	}
	return data[:len(data)-padding], nil
}

// buildKeyBox derives the substitution box that obfuscates the audio frames
// from the decrypted key material.
func buildKeyBox(key []byte) ([]byte, error) {
	if len(key) == 0 {
		return nil, errors.New("cannot derive a key box from empty key material")
	}
	box := make([]byte, 256)
	for i := range box {
		box[i] = byte(i)
	}
	var lastByte byte
	keyOffset := 0
	for i := range box {
		c := (box[i] + lastByte + key[keyOffset]) & 0xff
		keyOffset++
		if keyOffset >= len(key) {
			keyOffset = 0
		}
		box[i], box[c] = box[c], box[i]
		lastByte = c
	}
	return box, nil
}
