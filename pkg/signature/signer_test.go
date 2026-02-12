// Copyright 2026 fanjia1024

package signature

import (
	"context"
	"testing"
)

// TestKeyGeneration 测试密钥生成
func TestKeyGeneration(t *testing.T) {
	store := NewMemoryKeyStore()

	err := store.GenerateKey(context.Background(), "test_key")
	if err != nil {
		t.Fatalf("generate key failed: %v", err)
	}

	keys, _ := store.ListKeys(context.Background())
	if len(keys) != 1 {
		t.Errorf("expected 1 key, got %d", len(keys))
	}
}

// TestSignAndVerify 测试签名和验证
func TestSignAndVerify(t *testing.T) {
	store := NewMemoryKeyStore()
	store.GenerateKey(context.Background(), "org_key")

	signer := NewSigner(store, "org_key")

	data := []byte("test evidence package")
	signature, err := signer.SignPackage(data)
	if err != nil {
		t.Fatalf("sign failed: %v", err)
	}

	pubKey, _ := store.GetVerifyKey(context.Background(), "org_key")
	if !VerifyPackage(data, signature, pubKey) {
		t.Error("verification should pass")
	}
}

// TestVerifyTamperedData 测试篡改后签名验证失败
func TestVerifyTamperedData(t *testing.T) {
	store := NewMemoryKeyStore()
	store.GenerateKey(context.Background(), "org_key")

	signer := NewSigner(store, "org_key")

	data := []byte("original data")
	signature, _ := signer.SignPackage(data)

	// 篡改数据
	tamperedData := []byte("tampered data")

	pubKey, _ := store.GetVerifyKey(context.Background(), "org_key")
	if VerifyPackage(tamperedData, signature, pubKey) {
		t.Error("verification should fail for tampered data")
	}
}
