package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
)

// Passwords of external mailboxes are stored encrypted (AES-GCM). The key is COLLECTOR_KEY
// (base64, 32 bytes) or, if not set, derived from the Dovecot master password.
func (s *Server) secretKey() []byte {
	if k, err := base64.StdEncoding.DecodeString(env("COLLECTOR_KEY", "")); err == nil && len(k) == 32 {
		return k
	}
	sum := sha256.Sum256([]byte("tmail-collector:" + s.cfg.MasterPass))
	return sum[:]
}

func (s *Server) seal(plain string) ([]byte, error) {
	block, err := aes.NewCipher(s.secretKey())
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, []byte(plain), nil), nil
}

func (s *Server) unseal(data []byte) (string, error) {
	block, err := aes.NewCipher(s.secretKey())
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(data) < gcm.NonceSize() {
		return "", errors.New("bad secret")
	}
	out, err := gcm.Open(nil, data[:gcm.NonceSize()], data[gcm.NonceSize():], nil)
	return string(out), err
}
