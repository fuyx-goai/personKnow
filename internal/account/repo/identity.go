package repo

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

type IdentityCodec struct {
	aead    cipher.AEAD
	hashKey []byte
}

func NewIdentityCodec(secret []byte) (*IdentityCodec, error) {
	if len(secret) < 32 {
		return nil, fmt.Errorf("身份加密密钥至少需要 32 字节")
	}
	cipherKey := sha256.Sum256(append(append([]byte{}, secret...), []byte(":cipher")...))
	hashKey := sha256.Sum256(append(append([]byte{}, secret...), []byte(":hash")...))
	block, err := aes.NewCipher(cipherKey[:])
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &IdentityCodec{aead: aead, hashKey: hashKey[:]}, nil
}

func (codec *IdentityCodec) Encode(value string) ([]byte, string, error) {
	nonce := make([]byte, codec.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, "", err
	}
	ciphertext := codec.aead.Seal(nil, nonce, []byte(value), nil)
	payload := append(nonce, ciphertext...)
	mac := hmac.New(sha256.New, codec.hashKey)
	_, _ = mac.Write([]byte(value))
	return payload, hex.EncodeToString(mac.Sum(nil)), nil
}

func (codec *IdentityCodec) Decode(payload []byte) (string, error) {
	nonceSize := codec.aead.NonceSize()
	if len(payload) <= nonceSize {
		return "", fmt.Errorf("身份密文无效")
	}
	plaintext, err := codec.aead.Open(nil, payload[:nonceSize], payload[nonceSize:], nil)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}
