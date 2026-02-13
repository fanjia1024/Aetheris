// Copyright 2026 fanjia1024
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package signature

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"fmt"
)

// KeyStore 签名密钥存储接口（3.0-M4）
type KeyStore interface {
	// GetSigningKey 获取签名私钥
	GetSigningKey(ctx context.Context, keyID string) (ed25519.PrivateKey, error)

	// GetVerifyKey 获取验证公钥
	GetVerifyKey(ctx context.Context, keyID string) (ed25519.PublicKey, error)

	// GenerateKey 生成新密钥对
	GenerateKey(ctx context.Context, keyID string) error

	// ListKeys 列出所有密钥
	ListKeys(ctx context.Context) ([]string, error)
}

// Signer 签名器
type Signer struct {
	keyStore KeyStore
	keyID    string
}

// NewSigner 创建签名器
func NewSigner(keyStore KeyStore, keyID string) *Signer {
	return &Signer{
		keyStore: keyStore,
		keyID:    keyID,
	}
}

// SignPackage 签名证据包
// 返回格式: "ed25519:<keyID>:<base64_signature>"
func (s *Signer) SignPackage(packageData []byte) (string, error) {
	privKey, err := s.keyStore.GetSigningKey(context.Background(), s.keyID)
	if err != nil {
		return "", fmt.Errorf("failed to get signing key: %w", err)
	}

	// Ed25519 签名
	signature := ed25519.Sign(privKey, packageData)

	// 格式: ed25519:<keyID>:<signature_base64>
	sigStr := fmt.Sprintf("ed25519:%s:%s", s.keyID, base64.StdEncoding.EncodeToString(signature))
	return sigStr, nil
}

// VerifyPackage 验证证据包签名
func VerifyPackage(packageData []byte, signatureStr string, pubKey ed25519.PublicKey) bool {
	// 解析签名格式: ed25519:<keyID>:<signature_base64>
	parts := []byte(signatureStr)

	// 找到第一个冒号
	firstColon := -1
	for i, b := range parts {
		if b == ':' {
			firstColon = i
			break
		}
	}
	if firstColon == -1 {
		return false
	}

	algorithm := string(parts[:firstColon])
	if algorithm != "ed25519" {
		return false
	}

	// 找到第二个冒号
	secondColon := -1
	for i := firstColon + 1; i < len(parts); i++ {
		if parts[i] == ':' {
			secondColon = i
			break
		}
	}
	if secondColon == -1 {
		return false
	}

	// keyID := string(parts[firstColon+1 : secondColon])
	sigBase64 := string(parts[secondColon+1:])

	signature, err := base64.StdEncoding.DecodeString(sigBase64)
	if err != nil {
		return false
	}

	// 验证签名
	return ed25519.Verify(pubKey, packageData, signature)
}

// MemoryKeyStore 内存密钥存储（用于开发和测试）
type MemoryKeyStore struct {
	keys map[string]keyPair
}

type keyPair struct {
	privateKey ed25519.PrivateKey
	publicKey  ed25519.PublicKey
}

// NewMemoryKeyStore 创建内存密钥存储
func NewMemoryKeyStore() *MemoryKeyStore {
	return &MemoryKeyStore{
		keys: make(map[string]keyPair),
	}
}

// GenerateKey 生成新密钥对
func (m *MemoryKeyStore) GenerateKey(ctx context.Context, keyID string) error {
	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return err
	}

	m.keys[keyID] = keyPair{
		privateKey: privKey,
		publicKey:  pubKey,
	}

	return nil
}

// GetSigningKey 获取签名私钥
func (m *MemoryKeyStore) GetSigningKey(ctx context.Context, keyID string) (ed25519.PrivateKey, error) {
	kp, ok := m.keys[keyID]
	if !ok {
		return nil, fmt.Errorf("key not found: %s", keyID)
	}
	return kp.privateKey, nil
}

// GetVerifyKey 获取验证公钥
func (m *MemoryKeyStore) GetVerifyKey(ctx context.Context, keyID string) (ed25519.PublicKey, error) {
	kp, ok := m.keys[keyID]
	if !ok {
		return nil, fmt.Errorf("key not found: %s", keyID)
	}
	return kp.publicKey, nil
}

// ListKeys 列出所有密钥
func (m *MemoryKeyStore) ListKeys(ctx context.Context) ([]string, error) {
	keys := []string{}
	for keyID := range m.keys {
		keys = append(keys, keyID)
	}
	return keys, nil
}
