package credentials

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/billm/baaaht/orchestrator/internal/config"
	"github.com/billm/baaaht/orchestrator/internal/logger"
	"github.com/billm/baaaht/orchestrator/pkg/types"
)

// createTestStore creates a test credential store
func createTestStore(t *testing.T) *Store {
	t.Helper()

	log, err := logger.NewDefault()
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}

	// Create a temporary directory for test credentials
	tmpDir := t.TempDir()
	cfg := config.DefaultCredentialsConfig()
	cfg.StorePath = filepath.Join(tmpDir, "credentials.json")

	store, err := NewStore(cfg, log)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	return store
}

// TestNewStore tests creating a new credential store
func TestNewStore(t *testing.T) {
	store := createTestStore(t)
	defer store.Close()

	if store == nil {
		t.Fatal("store is nil")
	}

	if store.IsClosed() {
		t.Fatal("store should not be closed")
	}
}

// TestNewStoreNilLogger tests creating a store with nil logger
func TestNewStoreNilLogger(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := config.DefaultCredentialsConfig()
	cfg.StorePath = filepath.Join(tmpDir, "credentials.json")

	store, err := NewStore(cfg, nil)
	if err != nil {
		t.Fatalf("failed to create store with nil logger: %v", err)
	}
	defer store.Close()

	if store == nil {
		t.Fatal("store is nil")
	}
}

// TestNewDefaultStore tests creating a store with default config
func TestNewDefaultStore(t *testing.T) {
	store, err := NewDefault(nil)
	if err != nil {
		t.Fatalf("failed to create default store: %v", err)
	}
	defer store.Close()

	if store == nil {
		t.Fatal("store is nil")
	}
}

// TestStoreAndRetrieve tests storing and retrieving a credential
func TestStoreAndRetrieve(t *testing.T) {
	store := createTestStore(t)
	defer store.Close()

	ctx := context.Background()
	cred := &Credential{
		Name:  "test-api-key",
		Type:  "api_key",
		Value: "sk-test-1234567890",
		Metadata: map[string]string{
			"service": "openai",
			"owner":   "test-user",
		},
		Tags: []string{"production", "llm"},
	}

	// Store the credential
	err := store.Store(ctx, cred)
	if err != nil {
		t.Fatalf("failed to store credential: %v", err)
	}

	if cred.ID == "" {
		t.Fatal("credential ID should be set")
	}

	// Retrieve the credential
	retrieved, err := store.Get(ctx, cred.ID)
	if err != nil {
		t.Fatalf("failed to retrieve credential: %v", err)
	}

	if retrieved.Value != "sk-test-1234567890" {
		t.Errorf("credential value mismatch: got %s, want sk-test-1234567890", retrieved.Value)
	}

	if retrieved.Name != "test-api-key" {
		t.Errorf("credential name mismatch: got %s, want test-api-key", retrieved.Name)
	}

	if retrieved.Type != "api_key" {
		t.Errorf("credential type mismatch: got %s, want api_key", retrieved.Type)
	}

	if retrieved.Metadata["service"] != "openai" {
		t.Errorf("metadata service mismatch: got %s, want openai", retrieved.Metadata["service"])
	}
}

// TestStoreAndGetByName tests retrieving a credential by name
func TestStoreAndGetByName(t *testing.T) {
	store := createTestStore(t)
	defer store.Close()

	ctx := context.Background()
	cred := &Credential{
		Name:  "named-credential",
		Type:  "token",
		Value: "my-secret-token",
	}

	err := store.Store(ctx, cred)
	if err != nil {
		t.Fatalf("failed to store credential: %v", err)
	}

	// Retrieve by name
	retrieved, err := store.GetByName(ctx, "named-credential")
	if err != nil {
		t.Fatalf("failed to retrieve credential by name: %v", err)
	}

	if retrieved.Value != "my-secret-token" {
		t.Errorf("value mismatch: got %s, want my-secret-token", retrieved.Value)
	}
}

// TestStoreUpdate tests updating an existing credential
func TestStoreUpdate(t *testing.T) {
	store := createTestStore(t)
	defer store.Close()

	ctx := context.Background()
	cred := &Credential{
		Name:  "updatable-cred",
		Type:  "password",
		Value: "old-password",
	}

	err := store.Store(ctx, cred)
	if err != nil {
		t.Fatalf("failed to store credential: %v", err)
	}

	// Update the credential
	cred.Value = "new-password"
	cred.Metadata = map[string]string{"updated": "true"}
	err = store.Store(ctx, cred)
	if err != nil {
		t.Fatalf("failed to update credential: %v", err)
	}

	// Retrieve and verify
	retrieved, err := store.Get(ctx, cred.ID)
	if err != nil {
		t.Fatalf("failed to retrieve updated credential: %v", err)
	}

	if retrieved.Value != "new-password" {
		t.Errorf("value mismatch: got %s, want new-password", retrieved.Value)
	}

	if retrieved.Metadata["updated"] != "true" {
		t.Errorf("metadata not updated")
	}
}

// TestStoreWithoutName tests that storing without a name fails
func TestStoreWithoutName(t *testing.T) {
	store := createTestStore(t)
	defer store.Close()

	ctx := context.Background()
	cred := &Credential{
		Type:  "api_key",
		Value: "secret",
	}

	err := store.Store(ctx, cred)
	if err == nil {
		t.Fatal("expected error when storing credential without name")
	}

	if !types.IsErrCode(err, types.ErrCodeInvalidArgument) {
		t.Errorf("expected invalid argument error, got: %v", err)
	}
}

// TestGetNotFound tests retrieving a non-existent credential
func TestGetNotFound(t *testing.T) {
	store := createTestStore(t)
	defer store.Close()

	ctx := context.Background()
	_, err := store.Get(ctx, "non-existent-id")

	if err == nil {
		t.Fatal("expected error when getting non-existent credential")
	}

	if !types.IsErrCode(err, types.ErrCodeNotFound) {
		t.Errorf("expected not found error, got: %v", err)
	}
}

// TestGetByNameNotFound tests retrieving by non-existent name
func TestGetByNameNotFound(t *testing.T) {
	store := createTestStore(t)
	defer store.Close()

	ctx := context.Background()
	_, err := store.GetByName(ctx, "non-existent-name")

	if err == nil {
		t.Fatal("expected error when getting non-existent credential by name")
	}

	if !types.IsErrCode(err, types.ErrCodeNotFound) {
		t.Errorf("expected not found error, got: %v", err)
	}
}

// TestListCredentials tests listing all credentials
func TestListCredentials(t *testing.T) {
	store := createTestStore(t)
	defer store.Close()

	ctx := context.Background()

	// Store multiple credentials
	creds := []*Credential{
		{Name: "cred1", Type: "api_key", Value: "value1"},
		{Name: "cred2", Type: "token", Value: "value2"},
		{Name: "cred3", Type: "password", Value: "value3"},
	}

	for _, cred := range creds {
		if err := store.Store(ctx, cred); err != nil {
			t.Fatalf("failed to store credential: %v", err)
		}
	}

	// List credentials
	list, err := store.List(ctx)
	if err != nil {
		t.Fatalf("failed to list credentials: %v", err)
	}

	if len(list) != 3 {
		t.Errorf("expected 3 credentials, got %d", len(list))
	}

	// Verify values are redacted
	for _, cred := range list {
		if cred.Value != "***REDACTED***" {
			t.Errorf("listed credential value should be redacted, got: %s", cred.Value)
		}
	}
}

// TestDeleteCredential tests deleting a credential
func TestDeleteCredential(t *testing.T) {
	store := createTestStore(t)
	defer store.Close()

	ctx := context.Background()
	cred := &Credential{
		Name:  "deletable-cred",
		Type:  "api_key",
		Value: "to-be-deleted",
	}

	err := store.Store(ctx, cred)
	if err != nil {
		t.Fatalf("failed to store credential: %v", err)
	}

	// Delete the credential
	err = store.Delete(ctx, cred.ID)
	if err != nil {
		t.Fatalf("failed to delete credential: %v", err)
	}

	// Verify it's gone
	_, err = store.Get(ctx, cred.ID)
	if err == nil {
		t.Fatal("expected error when getting deleted credential")
	}

	if !types.IsErrCode(err, types.ErrCodeNotFound) {
		t.Errorf("expected not found error, got: %v", err)
	}
}

// TestDeleteNotFound tests deleting a non-existent credential
func TestDeleteNotFound(t *testing.T) {
	store := createTestStore(t)
	defer store.Close()

	ctx := context.Background()
	err := store.Delete(ctx, "non-existent-id")

	if err == nil {
		t.Fatal("expected error when deleting non-existent credential")
	}

	if !types.IsErrCode(err, types.ErrCodeNotFound) {
		t.Errorf("expected not found error, got: %v", err)
	}
}

// TestCredentialExpiration tests expired credentials
func TestCredentialExpiration(t *testing.T) {
	store := createTestStore(t)
	defer store.Close()

	ctx := context.Background()
	cred := &Credential{
		Name:      "expired-cred",
		Type:      "api_key",
		Value:     "expired-value",
		ExpiresAt: time.Now().Add(-1 * time.Hour), // Expired 1 hour ago
	}

	err := store.Store(ctx, cred)
	if err != nil {
		t.Fatalf("failed to store credential: %v", err)
	}

	// Verify credential is expired
	if !cred.IsExpired() {
		t.Error("credential should be expired")
	}

	// Try to retrieve the credential
	_, err = store.Get(ctx, cred.ID)
	if err == nil {
		t.Fatal("expected error when getting expired credential")
	}

	if !types.IsErrCode(err, types.ErrCodePermission) {
		t.Errorf("expected permission error, got: %v", err)
	}
}

// TestCredentialExpiring tests credentials that will expire soon
func TestCredentialExpiring(t *testing.T) {
	cred := &Credential{
		Name:      "expiring-cred",
		Type:      "api_key",
		Value:     "value",
		ExpiresAt: time.Now().Add(6 * time.Hour), // Expires in 6 hours
	}

	// Check if expiring within 7 days
	if !cred.IsExpiring(7 * 24 * time.Hour) {
		t.Error("credential should be expiring within 7 days")
	}

	// Check if not expiring within 1 hour
	if cred.IsExpiring(1 * time.Hour) {
		t.Error("credential should not be expiring within 1 hour")
	}
}

// TestCredentialAccessStats tests access statistics tracking
func TestCredentialAccessStats(t *testing.T) {
	store := createTestStore(t)
	defer store.Close()

	ctx := context.Background()
	cred := &Credential{
		Name:  "stats-cred",
		Type:  "api_key",
		Value: "value",
	}

	err := store.Store(ctx, cred)
	if err != nil {
		t.Fatalf("failed to store credential: %v", err)
	}

	// Access the credential multiple times
	for i := 0; i < 5; i++ {
		_, err := store.Get(ctx, cred.ID)
		if err != nil {
			t.Fatalf("failed to get credential: %v", err)
		}
		// Give time for the async update to run
		time.Sleep(10 * time.Millisecond)
	}

	// Get the credential directly from the store to check stats
	store.mu.RLock()
	storedCred := store.credentials[cred.ID]
	store.mu.RUnlock()

	if storedCred.AccessCount != 5 {
		t.Errorf("expected access count 5, got %d", storedCred.AccessCount)
	}

	if storedCred.LastUsedAt.IsZero() {
		t.Error("last used at should be set")
	}
}

// TestCleanupExpired tests cleaning up expired credentials
func TestCleanupExpired(t *testing.T) {
	store := createTestStore(t)
	defer store.Close()

	ctx := context.Background()

	// Store valid and expired credentials
	validCred := &Credential{
		Name:  "valid-cred",
		Type:  "api_key",
		Value: "valid-value",
		ExpiresAt: time.Now().Add(1 * time.Hour),
	}

	expiredCred := &Credential{
		Name:  "expired-cred",
		Type:  "api_key",
		Value: "expired-value",
		ExpiresAt: time.Now().Add(-1 * time.Hour),
	}

	store.Store(ctx, validCred)
	store.Store(ctx, expiredCred)

	// Cleanup expired credentials
	count, err := store.CleanupExpired(ctx)
	if err != nil {
		t.Fatalf("failed to cleanup expired credentials: %v", err)
	}

	if count != 1 {
		t.Errorf("expected to cleanup 1 credential, got %d", count)
	}

	// Verify valid credential still exists
	_, err = store.Get(ctx, validCred.ID)
	if err != nil {
		t.Errorf("valid credential should still exist: %v", err)
	}

	// Verify expired credential is gone
	_, err = store.Get(ctx, expiredCred.ID)
	if err == nil {
		t.Error("expired credential should be removed")
	}
}

// TestStoreStats tests getting store statistics
func TestStoreStats(t *testing.T) {
	store := createTestStore(t)
	defer store.Close()

	ctx := context.Background()

	// Store various credentials
	validCred := &Credential{
		Name:  "valid",
		Type:  "api_key",
		Value: "value",
	}

	expiredCred := &Credential{
		Name:      "expired",
		Type:      "api_key",
		Value:     "value",
		ExpiresAt: time.Now().Add(-1 * time.Hour),
	}

	expiringCred := &Credential{
		Name:      "expiring",
		Type:      "api_key",
		Value:     "value",
		ExpiresAt: time.Now().Add(6 * time.Hour),
	}

	store.Store(ctx, validCred)
	store.Store(ctx, expiredCred)
	store.Store(ctx, expiringCred)

	// Get stats
	stats, err := store.Stats(ctx)
	if err != nil {
		t.Fatalf("failed to get stats: %v", err)
	}

	if stats.TotalCredentials != 3 {
		t.Errorf("expected 3 total credentials, got %d", stats.TotalCredentials)
	}

	if stats.ExpiredCount != 1 {
		t.Errorf("expected 1 expired credential, got %d", stats.ExpiredCount)
	}

	if stats.ExpiringCount != 1 {
		t.Errorf("expected 1 expiring credential, got %d", stats.ExpiringCount)
	}

	if !stats.EncryptionEnabled {
		t.Error("encryption should be enabled")
	}
}

// TestEncryptDecrypt tests encryption and decryption
func TestEncryptDecrypt(t *testing.T) {
	store := createTestStore(t)
	defer store.Close()

	plaintext := "my-secret-value"

	// Encrypt
	ciphertext, err := store.encrypt(plaintext)
	if err != nil {
		t.Fatalf("failed to encrypt: %v", err)
	}

	if ciphertext == plaintext {
		t.Error("ciphertext should be different from plaintext")
	}

	// Decrypt
	decrypted, err := store.decrypt(ciphertext)
	if err != nil {
		t.Fatalf("failed to decrypt: %v", err)
	}

	if decrypted != plaintext {
		t.Errorf("decrypted value mismatch: got %s, want %s", decrypted, plaintext)
	}
}

// TestEncryptionDisabled tests that encryption can be disabled
func TestEncryptionDisabled(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := config.DefaultCredentialsConfig()
	cfg.EncryptionEnabled = false
	cfg.StorePath = filepath.Join(tmpDir, "credentials.json")

	log, _ := logger.NewDefault()
	store, err := NewStore(cfg, log)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	plaintext := "no-encryption"

	// Encrypt (should be no-op)
	ciphertext, err := store.encrypt(plaintext)
	if err != nil {
		t.Fatalf("failed to encrypt with encryption disabled: %v", err)
	}

	if ciphertext != plaintext {
		t.Error("with encryption disabled, ciphertext should equal plaintext")
	}

	// Decrypt (should be no-op)
	decrypted, err := store.decrypt(ciphertext)
	if err != nil {
		t.Fatalf("failed to decrypt with encryption disabled: %v", err)
	}

	if decrypted != plaintext {
		t.Error("with encryption disabled, decrypted should equal plaintext")
	}
}

// TestStorePersistence tests that credentials persist to disk
func TestStorePersistence(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := config.DefaultCredentialsConfig()
	cfg.StorePath = filepath.Join(tmpDir, "credentials.json")

	log, _ := logger.NewDefault()

	ctx := context.Background()
	cred := &Credential{
		Name:  "persistent-cred",
		Type:  "api_key",
		Value: "persistent-value",
	}

	// Create store and store a credential
	store1, err := NewStore(cfg, log)
	if err != nil {
		t.Fatalf("failed to create first store: %v", err)
	}

	err = store1.Store(ctx, cred)
	if err != nil {
		t.Fatalf("failed to store credential: %v", err)
	}

	err = store1.Close()
	if err != nil {
		t.Fatalf("failed to close first store: %v", err)
	}

	// Create a new store and verify the credential persists
	store2, err := NewStore(cfg, log)
	if err != nil {
		t.Fatalf("failed to create second store: %v", err)
	}
	defer store2.Close()

	retrieved, err := store2.Get(ctx, cred.ID)
	if err != nil {
		t.Fatalf("failed to retrieve persisted credential: %v", err)
	}

	if retrieved.Value != "persistent-value" {
		t.Errorf("persisted value mismatch: got %s, want persistent-value", retrieved.Value)
	}
}

// TestClose tests closing the store
func TestClose(t *testing.T) {
	store := createTestStore(t)

	ctx := context.Background()
	cred := &Credential{
		Name:  "close-test",
		Type:  "api_key",
		Value: "value",
	}

	err := store.Store(ctx, cred)
	if err != nil {
		t.Fatalf("failed to store credential: %v", err)
	}

	// Close the store
	err = store.Close()
	if err != nil {
		t.Fatalf("failed to close store: %v", err)
	}

	if !store.IsClosed() {
		t.Error("store should be closed")
	}

	// Operations should fail after closing
	_, err = store.Get(ctx, cred.ID)
	if err == nil {
		t.Error("expected error when getting credential from closed store")
	}

	err = store.Store(ctx, &Credential{Name: "test", Type: "test", Value: "test"})
	if err == nil {
		t.Error("expected error when storing credential to closed store")
	}
}

// TestGlobalStore tests the global store singleton
func TestGlobalStore(t *testing.T) {
	// Reset global store
	globalStore = nil
	storeGlobalOnce = sync.Once{}

	store1 := Global()
	store2 := Global()

	if store1 != store2 {
		t.Error("Global should return the same instance")
	}

	// Clean up
	if store1 != nil && !store1.IsClosed() {
		store1.Close()
	}
}

// TestInvalidCiphertext tests decrypting invalid ciphertext
func TestInvalidCiphertext(t *testing.T) {
	store := createTestStore(t)
	defer store.Close()

	// Try to decrypt invalid base64
	_, err := store.decrypt("not-valid-base64!!!")
	if err == nil {
		t.Error("expected error when decrypting invalid base64")
	}

	// Try to decrypt valid base64 but invalid ciphertext
	_, err = store.decrypt("aGVsbG8=") // "hello" in base64
	if err == nil {
		t.Error("expected error when decrypting invalid ciphertext")
	}
}

// TestStoreEmptyID tests storing a credential with an empty ID (should generate one)
func TestStoreEmptyID(t *testing.T) {
	store := createTestStore(t)
	defer store.Close()

	ctx := context.Background()
	cred := &Credential{
		ID:    "", // Empty ID
		Name:  "empty-id-test",
		Type:  "api_key",
		Value: "value",
	}

	err := store.Store(ctx, cred)
	if err != nil {
		t.Fatalf("failed to store credential with empty ID: %v", err)
	}

	if cred.ID == "" {
		t.Error("ID should be generated when empty")
	}
}

// BenchmarkStoreOperations benchmarks store operations
func BenchmarkStoreOperations(b *testing.B) {
	log, _ := logger.NewDefault()
	tmpDir := b.TempDir()
	cfg := config.DefaultCredentialsConfig()
	cfg.StorePath = filepath.Join(tmpDir, "credentials.json")

	store, err := NewStore(cfg, log)
	if err != nil {
		b.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cred := &Credential{
			Name:  fmt.Sprintf("bench-cred-%d", i),
			Type:  "api_key",
			Value: fmt.Sprintf("value-%d", i),
		}
		if err := store.Store(ctx, cred); err != nil {
			b.Fatalf("failed to store credential: %v", err)
		}
	}
}

// BenchmarkGetOperation benchmarks get operations
func BenchmarkGetOperation(b *testing.B) {
	log, _ := logger.NewDefault()
	tmpDir := b.TempDir()
	cfg := config.DefaultCredentialsConfig()
	cfg.StorePath = filepath.Join(tmpDir, "credentials.json")

	store, err := NewStore(cfg, log)
	if err != nil {
		b.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	cred := &Credential{
		Name:  "bench-cred",
		Type:  "api_key",
		Value: "value",
	}
	if err := store.Store(ctx, cred); err != nil {
		b.Fatalf("failed to store credential: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := store.Get(ctx, cred.ID)
		if err != nil {
			b.Fatalf("failed to get credential: %v", err)
		}
	}
}
