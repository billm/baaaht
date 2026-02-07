package credentials

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/billm/baaaht/orchestrator/internal/config"
	"github.com/billm/baaaht/orchestrator/internal/logger"
	"github.com/billm/baaaht/orchestrator/pkg/types"
)

// Credential represents a stored credential with metadata
type Credential struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Type        string            `json:"type"`  // e.g., "api_key", "token", "password"
	Value       string            `json:"value"` // encrypted value
	Metadata    map[string]string `json:"metadata"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
	ExpiresAt   time.Time         `json:"expires_at"`
	LastUsedAt  time.Time         `json:"last_used_at"`
	AccessCount int               `json:"access_count"`
	Tags        []string          `json:"tags"`
}

// IsExpired returns true if the credential has expired
func (c *Credential) IsExpired() bool {
	if c.ExpiresAt.IsZero() {
		return false
	}
	return time.Now().After(c.ExpiresAt)
}

// IsExpiring returns true if the credential will expire within the given duration
func (c *Credential) IsExpiring(within time.Duration) bool {
	if c.ExpiresAt.IsZero() {
		return false
	}
	return time.Now().Add(within).After(c.ExpiresAt)
}

// Store manages secure credential storage with encryption
type Store struct {
	credentials   map[string]*Credential
	mu            sync.RWMutex
	cfg           config.CredentialsConfig
	logger        *logger.Logger
	encryptionKey []byte
	gcm           cipher.AEAD
	closed        bool
}

// NewStore creates a new credential store with the specified configuration
func NewStore(cfg config.CredentialsConfig, log *logger.Logger) (*Store, error) {
	if log == nil {
		var err error
		log, err = logger.NewDefault()
		if err != nil {
			return nil, types.WrapError(types.ErrCodeInternal, "failed to create default logger", err)
		}
	}

	// Ensure the store directory exists
	if err := os.MkdirAll(filepath.Dir(cfg.StorePath), 0700); err != nil {
		return nil, types.WrapError(types.ErrCodeInternal, "failed to create credentials directory", err)
	}

	// Generate or load encryption key
	encryptionKey, err := getOrCreateEncryptionKey(cfg.StorePath)
	if err != nil {
		return nil, types.WrapError(types.ErrCodeInternal, "failed to initialize encryption key", err)
	}

	// Create AES-GCM cipher
	block, err := aes.NewCipher(encryptionKey)
	if err != nil {
		return nil, types.WrapError(types.ErrCodeInternal, "failed to create cipher", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, types.WrapError(types.ErrCodeInternal, "failed to create GCM", err)
	}

	store := &Store{
		credentials:   make(map[string]*Credential),
		cfg:           cfg,
		logger:        log.With("component", "credential_store"),
		encryptionKey: encryptionKey,
		gcm:           gcm,
		closed:        false,
	}

	// Load existing credentials from disk
	if err := store.loadFromDisk(); err != nil {
		store.logger.Warn("Failed to load existing credentials from disk", "error", err)
		// Continue anyway - the store will start fresh
	}

	store.logger.Info("Credential store initialized", "path", cfg.StorePath, "encryption_enabled", cfg.EncryptionEnabled)
	return store, nil
}

// NewDefaultStore creates a new credential store with default configuration
func NewDefaultStore(log *logger.Logger) (*Store, error) {
	cfg := config.DefaultCredentialsConfig()
	return NewStore(cfg, log)
}

// encrypt encrypts a plaintext value using AES-GCM
func (s *Store) encrypt(plaintext string) (string, error) {
	if !s.cfg.EncryptionEnabled {
		return plaintext, nil
	}

	// Create a random nonce
	nonce := make([]byte, s.gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", types.WrapError(types.ErrCodeInternal, "failed to generate nonce", err)
	}

	// Encrypt and authenticate
	ciphertext := s.gcm.Seal(nonce, nonce, []byte(plaintext), nil)

	// Encode as base64
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// decrypt decrypts a ciphertext value using AES-GCM
func (s *Store) decrypt(ciphertext string) (string, error) {
	if !s.cfg.EncryptionEnabled {
		return ciphertext, nil
	}

	// Decode from base64
	data, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", types.WrapError(types.ErrCodeInternal, "failed to decode credential", err)
	}

	// Extract nonce
	nonceSize := s.gcm.NonceSize()
	if len(data) < nonceSize {
		return "", types.NewError(types.ErrCodeInternal, "ciphertext too short")
	}

	nonce, ciphertextBytes := data[:nonceSize], data[nonceSize:]

	// Decrypt and verify
	plaintext, err := s.gcm.Open(nil, nonce, ciphertextBytes, nil)
	if err != nil {
		return "", types.WrapError(types.ErrCodePermission, "failed to decrypt credential", err)
	}

	return string(plaintext), nil
}

// Store stores a new credential or updates an existing one
func (s *Store) Store(ctx context.Context, cred *Credential) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return types.NewError(types.ErrCodeUnavailable, "credential store is closed")
	}

	if cred.ID == "" {
		cred.ID = types.GenerateID().String()
	}

	if cred.Name == "" {
		return types.NewError(types.ErrCodeInvalidArgument, "credential name is required")
	}

	// Check if credential already exists
	existing, exists := s.credentials[cred.ID]
	now := time.Now()

	if exists {
		// Update existing credential
		existing.Name = cred.Name
		existing.Type = cred.Type
		existing.Metadata = cred.Metadata
		existing.UpdatedAt = now
		existing.ExpiresAt = cred.ExpiresAt
		existing.Tags = cred.Tags

		// Encrypt and store the value
		encrypted, err := s.encrypt(cred.Value)
		if err != nil {
			return err
		}
		existing.Value = encrypted

		s.logger.Info("Credential updated", "id", cred.ID, "name", cred.Name)
	} else {
		// Create new credential
		cred.CreatedAt = now
		cred.UpdatedAt = now

		// Encrypt the value
		encrypted, err := s.encrypt(cred.Value)
		if err != nil {
			return err
		}
		cred.Value = encrypted

		s.credentials[cred.ID] = cred
		s.logger.Info("Credential stored", "id", cred.ID, "name", cred.Name, "type", cred.Type)
	}

	// Persist to disk
	if err := s.saveToDisk(); err != nil {
		s.logger.Error("Failed to persist credentials to disk", "error", err)
		return err
	}

	return nil
}

// Get retrieves a credential by ID
func (s *Store) Get(ctx context.Context, id string) (*Credential, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return nil, types.NewError(types.ErrCodeUnavailable, "credential store is closed")
	}

	cred, exists := s.credentials[id]
	if !exists {
		return nil, types.NewError(types.ErrCodeNotFound, fmt.Sprintf("credential not found: %s", id))
	}

	// Check if credential is expired
	if cred.IsExpired() {
		return nil, types.NewError(types.ErrCodePermission, "credential has expired")
	}

	// Decrypt the value
	decrypted, err := s.decrypt(cred.Value)
	if err != nil {
		return nil, err
	}

	// Return a copy with decrypted value
	result := &Credential{
		ID:          cred.ID,
		Name:        cred.Name,
		Type:        cred.Type,
		Value:       decrypted,
		Metadata:    cred.Metadata,
		CreatedAt:   cred.CreatedAt,
		UpdatedAt:   cred.UpdatedAt,
		ExpiresAt:   cred.ExpiresAt,
		LastUsedAt:  cred.LastUsedAt,
		AccessCount: cred.AccessCount,
		Tags:        cred.Tags,
	}

	// Update access statistics (in write lock)
	go s.updateAccessStats(id)

	return result, nil
}

// GetByName retrieves a credential by name
func (s *Store) GetByName(ctx context.Context, name string) (*Credential, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return nil, types.NewError(types.ErrCodeUnavailable, "credential store is closed")
	}

	for _, cred := range s.credentials {
		if cred.Name == name {
			// Check if credential is expired
			if cred.IsExpired() {
				return nil, types.NewError(types.ErrCodePermission, "credential has expired")
			}

			// Decrypt directly without nested Get call
			decrypted, err := s.decrypt(cred.Value)
			if err != nil {
				return nil, types.WrapError(types.ErrCodeInternal, "failed to decrypt credential", err)
			}

			// Update access statistics asynchronously
			go s.updateAccessStats(cred.ID)

			return &Credential{
				ID:          cred.ID,
				Name:        cred.Name,
				Type:        cred.Type,
				Value:       decrypted,
				Metadata:    cred.Metadata,
				CreatedAt:   cred.CreatedAt,
				UpdatedAt:   cred.UpdatedAt,
				ExpiresAt:   cred.ExpiresAt,
				LastUsedAt:  cred.LastUsedAt,
				AccessCount: cred.AccessCount,
				Tags:        cred.Tags,
			}, nil
		}
	}

	return nil, types.NewError(types.ErrCodeNotFound, fmt.Sprintf("credential not found: %s", name))
}

// updateAccessStats updates the last used time and access count
func (s *Store) updateAccessStats(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if cred, exists := s.credentials[id]; exists {
		cred.LastUsedAt = time.Now()
		cred.AccessCount++
	}
}

// List lists all credentials without their values
func (s *Store) List(ctx context.Context) ([]*Credential, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return nil, types.NewError(types.ErrCodeUnavailable, "credential store is closed")
	}

	result := make([]*Credential, 0, len(s.credentials))
	for _, cred := range s.credentials {
		// Return credential metadata without the value
		result = append(result, &Credential{
			ID:          cred.ID,
			Name:        cred.Name,
			Type:        cred.Type,
			Value:       "***REDACTED***",
			Metadata:    cred.Metadata,
			CreatedAt:   cred.CreatedAt,
			UpdatedAt:   cred.UpdatedAt,
			ExpiresAt:   cred.ExpiresAt,
			LastUsedAt:  cred.LastUsedAt,
			AccessCount: cred.AccessCount,
			Tags:        cred.Tags,
		})
	}

	return result, nil
}

// Delete removes a credential from the store
func (s *Store) Delete(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return types.NewError(types.ErrCodeUnavailable, "credential store is closed")
	}

	if _, exists := s.credentials[id]; !exists {
		return types.NewError(types.ErrCodeNotFound, fmt.Sprintf("credential not found: %s", id))
	}

	delete(s.credentials, id)
	s.logger.Info("Credential deleted", "id", id)

	// Persist to disk
	if err := s.saveToDisk(); err != nil {
		s.logger.Error("Failed to persist credentials to disk", "error", err)
		return err
	}

	return nil
}

// RotateKey rotates the encryption key and re-encrypts all credentials
func (s *Store) RotateKey(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return types.NewError(types.ErrCodeUnavailable, "credential store is closed")
	}

	s.logger.Info("Starting encryption key rotation")

	// Generate new encryption key
	newKey := make([]byte, 32) // AES-256
	if _, err := io.ReadFull(rand.Reader, newKey); err != nil {
		return types.WrapError(types.ErrCodeInternal, "failed to generate new encryption key", err)
	}

	// Create new cipher
	block, err := aes.NewCipher(newKey)
	if err != nil {
		return types.WrapError(types.ErrCodeInternal, "failed to create cipher", err)
	}

	newGCM, err := cipher.NewGCM(block)
	if err != nil {
		return types.WrapError(types.ErrCodeInternal, "failed to create GCM", err)
	}

	// Re-encrypt all credentials
	oldGCM := s.gcm
	s.gcm = newGCM

	for id, cred := range s.credentials {
		// Decrypt with old key
		decrypted, err := s.decryptWithGCM(oldGCM, cred.Value)
		if err != nil {
			s.gcm = oldGCM // Restore old cipher
			return types.WrapError(types.ErrCodeInternal, fmt.Sprintf("failed to decrypt credential %s", id), err)
		}

		// Encrypt with new key
		encrypted, err := s.encrypt(decrypted)
		if err != nil {
			s.gcm = oldGCM // Restore old cipher
			return types.WrapError(types.ErrCodeInternal, fmt.Sprintf("failed to encrypt credential %s", id), err)
		}

		cred.Value = encrypted
	}

	s.encryptionKey = newKey

	// Save new key to disk
	keyPath := filepath.Join(filepath.Dir(s.cfg.StorePath), ".cred_key")
	if err := os.WriteFile(keyPath, newKey, 0600); err != nil {
		return types.WrapError(types.ErrCodeInternal, "failed to save new encryption key", err)
	}

	// Persist re-encrypted credentials
	if err := s.saveToDisk(); err != nil {
		return err
	}

	s.logger.Info("Encryption key rotation completed")
	return nil
}

// decryptWithGCM decrypts using a specific GCM instance
func (s *Store) decryptWithGCM(gcm cipher.AEAD, ciphertext string) (string, error) {
	if !s.cfg.EncryptionEnabled {
		return ciphertext, nil
	}

	data, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", types.WrapError(types.ErrCodeInternal, "failed to decode credential", err)
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", types.NewError(types.ErrCodeInternal, "ciphertext too short")
	}

	nonce, ciphertextBytes := data[:nonceSize], data[nonceSize:]

	plaintext, err := gcm.Open(nil, nonce, ciphertextBytes, nil)
	if err != nil {
		return "", types.WrapError(types.ErrCodePermission, "failed to decrypt credential", err)
	}

	return string(plaintext), nil
}

// CleanupExpired removes all expired credentials from the store
func (s *Store) CleanupExpired(ctx context.Context) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return 0, types.NewError(types.ErrCodeUnavailable, "credential store is closed")
	}

	count := 0
	for id, cred := range s.credentials {
		if cred.IsExpired() {
			delete(s.credentials, id)
			count++
			s.logger.Info("Removed expired credential", "id", id, "name", cred.Name)
		}
	}

	if count > 0 {
		// Persist to disk
		if err := s.saveToDisk(); err != nil {
			return count, err
		}
	}

	return count, nil
}

// Stats returns statistics about the credential store
func (s *Store) Stats(ctx context.Context) (*StoreStats, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return nil, types.NewError(types.ErrCodeUnavailable, "credential store is closed")
	}

	expiredCount := 0
	expiringCount := 0
	totalAccessCount := 0

	for _, cred := range s.credentials {
		if cred.IsExpired() {
			expiredCount++
		} else if cred.IsExpiring(7 * 24 * time.Hour) {
			expiringCount++
		}
		totalAccessCount += cred.AccessCount
	}

	return &StoreStats{
		TotalCredentials:  len(s.credentials),
		ExpiredCount:      expiredCount,
		ExpiringCount:     expiringCount,
		TotalAccessCount:  totalAccessCount,
		EncryptionEnabled: s.cfg.EncryptionEnabled,
	}, nil
}

// StoreStats represents statistics about the credential store
type StoreStats struct {
	TotalCredentials  int       `json:"total_credentials"`
	ExpiredCount      int       `json:"expired_count"`
	ExpiringCount     int       `json:"expiring_count"`
	TotalAccessCount  int       `json:"total_access_count"`
	EncryptionEnabled bool      `json:"encryption_enabled"`
	LastKeyRotation   time.Time `json:"last_key_rotation"`
}

// Close closes the credential store and persists all data
func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil
	}

	// Final persist
	if err := s.saveToDisk(); err != nil {
		return err
	}

	s.closed = true
	s.logger.Info("Credential store closed")
	return nil
}

// IsClosed returns true if the store is closed
func (s *Store) IsClosed() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.closed
}

// saveToDisk saves all credentials to disk
func (s *Store) saveToDisk() error {
	data, err := json.MarshalIndent(s.credentials, "", "  ")
	if err != nil {
		return types.WrapError(types.ErrCodeInternal, "failed to marshal credentials", err)
	}

	// Write to temporary file first
	tmpPath := s.cfg.StorePath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0600); err != nil {
		return types.WrapError(types.ErrCodeInternal, "failed to write credentials file", err)
	}

	// Atomic rename
	if err := os.Rename(tmpPath, s.cfg.StorePath); err != nil {
		os.Remove(tmpPath)
		return types.WrapError(types.ErrCodeInternal, "failed to rename credentials file", err)
	}

	return nil
}

// loadFromDisk loads all credentials from disk
func (s *Store) loadFromDisk() error {
	data, err := os.ReadFile(s.cfg.StorePath)
	if err != nil {
		if os.IsNotExist(err) {
			// No existing credentials file, start fresh
			return nil
		}
		return types.WrapError(types.ErrCodeInternal, "failed to read credentials file", err)
	}

	if err := json.Unmarshal(data, &s.credentials); err != nil {
		return types.WrapError(types.ErrCodeInternal, "failed to unmarshal credentials", err)
	}

	s.logger.Info("Loaded credentials from disk", "count", len(s.credentials))
	return nil
}

// getOrCreateEncryptionKey gets an existing encryption key or creates a new one
func getOrCreateEncryptionKey(storePath string) ([]byte, error) {
	keyPath := filepath.Join(filepath.Dir(storePath), ".cred_key")

	// Try to load existing key
	if data, err := os.ReadFile(keyPath); err == nil {
		if len(data) == 32 { // AES-256
			return data, nil
		}
		return nil, types.NewError(types.ErrCodeInternal, "invalid encryption key length")
	}

	// Generate new key
	key := make([]byte, 32) // AES-256
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, types.WrapError(types.ErrCodeInternal, "failed to generate encryption key", err)
	}

	// Save new key
	if err := os.WriteFile(keyPath, key, 0600); err != nil {
		return nil, types.WrapError(types.ErrCodeInternal, "failed to save encryption key", err)
	}

	return key, nil
}

// Global store instance
var (
	globalStore     *Store
	storeGlobalOnce sync.Once
)

// InitGlobal initializes the global credential store
func InitGlobal(cfg config.CredentialsConfig, log *logger.Logger) error {
	var initErr error
	storeGlobalOnce.Do(func() {
		store, err := NewStore(cfg, log)
		if err != nil {
			initErr = err
			return
		}
		globalStore = store
	})
	return initErr
}

// Global returns the global credential store instance
func Global() *Store {
	if globalStore == nil {
		// Initialize with default settings if not already initialized
		store, err := NewDefaultStore(nil)
		if err != nil {
			// Return a closed store on error, but cache it as the singleton
			globalStore = &Store{closed: true}
			return globalStore
		}
		globalStore = store
	}
	return globalStore
}

// SetGlobal sets the global credential store instance
func SetGlobal(s *Store) {
	globalStore = s
	storeGlobalOnce = sync.Once{}
}
