package storageconsul

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
)

func (s *Storage) encrypt(bytes []byte) ([]byte, error) {
	// No key? No encrypt
	if len(s.AESKey) == 0 {
		return bytes, nil
	}

	c, err := aes.NewCipher(s.AESKey)
	if err != nil {
		return nil, fmt.Errorf("unable to create AES cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(c)
	if err != nil {
		return nil, fmt.Errorf("unable to create GCM cipher: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	_, err = io.ReadFull(rand.Reader, nonce)
	if err != nil {
		return nil, fmt.Errorf("unable to generate nonce: %w", err)
	}

	return gcm.Seal(nonce, nonce, bytes, nil), nil
}

func (s *Storage) EncryptStorageData(data *StorageData) ([]byte, error) {
	// JSON marshal, then encrypt if key is there
	bytes, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("unable to marshal: %w", err)
	}

	// Prefix with simple prefix and then encrypt
	bytes = append([]byte(s.ValuePrefix), bytes...)
	return s.encrypt(bytes)
}

func (s *Storage) decrypt(bytes []byte) ([]byte, error) {
	// No key? No decrypt
	if len(s.AESKey) == 0 {
		return bytes, nil
	}
	if len(bytes) < aes.BlockSize {
		return nil, fmt.Errorf("invalid contents")
	}

	block, err := aes.NewCipher(s.AESKey)
	if err != nil {
		return nil, fmt.Errorf("unable to create AES cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("unable to create GCM cipher: %w", err)
	}

	out, err := gcm.Open(nil, bytes[:gcm.NonceSize()], bytes[gcm.NonceSize():], nil)
	if err != nil {
		return nil, fmt.Errorf("decryption failure: %w", err)
	}

	return out, nil
}

func (s *Storage) DecryptStorageData(bytes []byte) (*StorageData, error) {
	// We have to decrypt if there is an AES key and then JSON unmarshal
	bytes, err := s.decrypt(bytes)
	if err != nil {
		return nil, fmt.Errorf("unable to decrypt data: %w", err)
	}

	// Simple sanity check of the beginning of the byte array just to check
	if len(bytes) < len(s.ValuePrefix) || string(bytes[:len(s.ValuePrefix)]) != s.ValuePrefix {
		return nil, fmt.Errorf("invalid data format")
	}

	// Now just json unmarshal
	data := &StorageData{}
	if err := json.Unmarshal(bytes[len(s.ValuePrefix):], data); err != nil {
		return nil, fmt.Errorf("unable to unmarshal result: %w", err)
	}
	return data, nil
}
