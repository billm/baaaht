package memory

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/billm/baaaht/orchestrator/internal/config"
	"github.com/billm/baaaht/orchestrator/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateScope(t *testing.T) {
	tests := []struct {
		name    string
		scope   types.MemoryScope
		wantErr bool
	}{
		{
			name:    "valid user scope",
			scope:   types.MemoryScopeUser,
			wantErr: false,
		},
		{
			name:    "valid group scope",
			scope:   types.MemoryScopeGroup,
			wantErr: false,
		},
		{
			name:    "invalid scope",
			scope:   types.MemoryScope("invalid"),
			wantErr: true,
		},
		{
			name:    "empty scope",
			scope:   types.MemoryScope(""),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateScope(tt.scope)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestGetScopePath(t *testing.T) {
	tempDir := t.TempDir()
	cfg := config.MemoryConfig{
		StoragePath:     tempDir,
		UserMemoryPath:  filepath.Join(tempDir, "users"),
		GroupMemoryPath: filepath.Join(tempDir, "groups"),
	}

	t.Run("user scope path", func(t *testing.T) {
		path, err := GetScopePath(cfg, "user")
		require.NoError(t, err)
		assert.Equal(t, cfg.UserMemoryPath, path)
	})

	t.Run("group scope path", func(t *testing.T) {
		path, err := GetScopePath(cfg, "group")
		require.NoError(t, err)
		assert.Equal(t, cfg.GroupMemoryPath, path)
	})

	t.Run("invalid scope", func(t *testing.T) {
		_, err := GetScopePath(cfg, "invalid")
		assert.Error(t, err)
	})
}

func TestGetOwnerPath(t *testing.T) {
	tempDir := t.TempDir()
	cfg := config.MemoryConfig{
		StoragePath:     tempDir,
		UserMemoryPath:  filepath.Join(tempDir, "users"),
		GroupMemoryPath: filepath.Join(tempDir, "groups"),
	}

	t.Run("user owner path", func(t *testing.T) {
		path, err := GetOwnerPath(cfg, "user", "user123")
		require.NoError(t, err)
		assert.Equal(t, filepath.Join(cfg.UserMemoryPath, "user123"), path)
	})

	t.Run("group owner path", func(t *testing.T) {
		path, err := GetOwnerPath(cfg, "group", "group1")
		require.NoError(t, err)
		assert.Equal(t, filepath.Join(cfg.GroupMemoryPath, "group1"), path)
	})

	t.Run("empty owner ID", func(t *testing.T) {
		_, err := GetOwnerPath(cfg, "user", "")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "owner ID cannot be empty")
	})

	t.Run("sanitizes owner ID", func(t *testing.T) {
		// Owner IDs with special characters should be sanitized
		path, err := GetOwnerPath(cfg, "user", "user@123!test")
		require.NoError(t, err)
		// Should only contain safe characters
		assert.Contains(t, path, "user-123-test")
		assert.NotContains(t, path, "@")
		assert.NotContains(t, path, "!")
	})

	t.Run("prevents directory traversal", func(t *testing.T) {
		// Owner IDs with path traversal attempts should be sanitized
		path, err := GetOwnerPath(cfg, "user", "../../../etc")
		require.NoError(t, err)
		// Should be sanitized and not contain path traversal
		assert.NotContains(t, path, "..")
	})
}

func TestSanitizeOwnerID(t *testing.T) {
	tests := []struct {
		name     string
		ownerID  string
		expected string
	}{
		{
			name:     "alphanumeric",
			ownerID:  "user123",
			expected: "user123",
		},
		{
			name:     "with dashes and underscores",
			ownerID:  "user-123_test",
			expected: "user-123_test",
		},
		{
			name:     "with dots",
			ownerID:  "user.example.com",
			expected: "user.example.com",
		},
		{
			name:     "special chars become dashes",
			ownerID:  "user@123!test",
			expected: "user-123-test",
		},
		{
			name:     "directory traversal removed",
			ownerID:  "../../../etc",
			expected: "etc",
		},
		{
			name:     "mixed case preserved",
			ownerID:  "UserABC",
			expected: "UserABC",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeOwnerID(tt.ownerID)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCreateScopeDir(t *testing.T) {
	tempDir := t.TempDir()
	cfg := config.MemoryConfig{
		StoragePath:     tempDir,
		UserMemoryPath:  filepath.Join(tempDir, "users"),
		GroupMemoryPath: filepath.Join(tempDir, "groups"),
	}

	t.Run("create user scope directory", func(t *testing.T) {
		err := CreateScopeDir(cfg, "user")
		require.NoError(t, err)

		info, err := os.Stat(cfg.UserMemoryPath)
		require.NoError(t, err)
		assert.True(t, info.IsDir())
	})

	t.Run("create group scope directory", func(t *testing.T) {
		err := CreateScopeDir(cfg, "group")
		require.NoError(t, err)

		info, err := os.Stat(cfg.GroupMemoryPath)
		require.NoError(t, err)
		assert.True(t, info.IsDir())
	})

	t.Run("create existing directory succeeds", func(t *testing.T) {
		// Create directory first
		err := os.MkdirAll(cfg.UserMemoryPath, 0755)
		require.NoError(t, err)

		// Creating again should succeed
		err = CreateScopeDir(cfg, "user")
		assert.NoError(t, err)
	})
}

func TestCreateOwnerDir(t *testing.T) {
	tempDir := t.TempDir()
	cfg := config.MemoryConfig{
		StoragePath:     tempDir,
		UserMemoryPath:  filepath.Join(tempDir, "users"),
		GroupMemoryPath: filepath.Join(tempDir, "groups"),
	}

	t.Run("create user owner directory", func(t *testing.T) {
		path, err := CreateOwnerDir(cfg, "user", "user123")
		require.NoError(t, err)

		expectedPath := filepath.Join(cfg.UserMemoryPath, "user123")
		assert.Equal(t, expectedPath, path)

		info, err := os.Stat(expectedPath)
		require.NoError(t, err)
		assert.True(t, info.IsDir())
	})

	t.Run("create group owner directory", func(t *testing.T) {
		path, err := CreateOwnerDir(cfg, "group", "group1")
		require.NoError(t, err)

		expectedPath := filepath.Join(cfg.GroupMemoryPath, "group1")
		assert.Equal(t, expectedPath, path)

		info, err := os.Stat(expectedPath)
		require.NoError(t, err)
		assert.True(t, info.IsDir())
	})

	t.Run("create existing owner directory succeeds", func(t *testing.T) {
		// Create directory first
		ownerPath := filepath.Join(cfg.UserMemoryPath, "user456")
		err := os.MkdirAll(ownerPath, 0755)
		require.NoError(t, err)

		// Creating again should succeed
		path, err := CreateOwnerDir(cfg, "user", "user456")
		assert.NoError(t, err)
		assert.Equal(t, ownerPath, path)
	})

	t.Run("empty owner ID returns error", func(t *testing.T) {
		_, err := CreateOwnerDir(cfg, "user", "")
		assert.Error(t, err)
	})
}

func TestScopeExists(t *testing.T) {
	tempDir := t.TempDir()
	cfg := config.MemoryConfig{
		StoragePath:     tempDir,
		UserMemoryPath:  filepath.Join(tempDir, "users"),
		GroupMemoryPath: filepath.Join(tempDir, "groups"),
	}

	t.Run("existing scope", func(t *testing.T) {
		err := os.MkdirAll(cfg.UserMemoryPath, 0755)
		require.NoError(t, err)

		exists, err := ScopeExists(cfg, "user")
		require.NoError(t, err)
		assert.True(t, exists)
	})

	t.Run("non-existing scope", func(t *testing.T) {
		exists, err := ScopeExists(cfg, "group")
		require.NoError(t, err)
		assert.False(t, exists)
	})

	t.Run("invalid scope", func(t *testing.T) {
		_, err := ScopeExists(cfg, "invalid")
		assert.Error(t, err)
	})
}

func TestOwnerDirExists(t *testing.T) {
	tempDir := t.TempDir()
	cfg := config.MemoryConfig{
		StoragePath:     tempDir,
		UserMemoryPath:  filepath.Join(tempDir, "users"),
		GroupMemoryPath: filepath.Join(tempDir, "groups"),
	}

	t.Run("existing owner directory", func(t *testing.T) {
		ownerPath := filepath.Join(cfg.UserMemoryPath, "user123")
		err := os.MkdirAll(ownerPath, 0755)
		require.NoError(t, err)

		exists, err := OwnerDirExists(cfg, "user", "user123")
		require.NoError(t, err)
		assert.True(t, exists)
	})

	t.Run("non-existing owner directory", func(t *testing.T) {
		exists, err := OwnerDirExists(cfg, "user", "user999")
		require.NoError(t, err)
		assert.False(t, exists)
	})

	t.Run("scope directory doesn't exist", func(t *testing.T) {
		exists, err := OwnerDirExists(cfg, "user", "user999")
		require.NoError(t, err)
		assert.False(t, exists)
	})
}

func TestListOwners(t *testing.T) {
	tempDir := t.TempDir()
	cfg := config.MemoryConfig{
		StoragePath:     tempDir,
		UserMemoryPath:  filepath.Join(tempDir, "users"),
		GroupMemoryPath: filepath.Join(tempDir, "groups"),
	}

	t.Run("list owners with existing directories", func(t *testing.T) {
		// Create owner directories
		owners := []string{"user1", "user2", "user3"}
		for _, owner := range owners {
			ownerPath := filepath.Join(cfg.UserMemoryPath, owner)
			err := os.MkdirAll(ownerPath, 0755)
			require.NoError(t, err)
		}

		// Also create a file (not a directory) which should be ignored
		filePath := filepath.Join(cfg.UserMemoryPath, "notadir.txt")
		err := os.WriteFile(filePath, []byte("test"), 0644)
		require.NoError(t, err)

		result, err := ListOwners(cfg, "user")
		require.NoError(t, err)
		assert.ElementsMatch(t, owners, result)
	})

	t.Run("list owners with no directories", func(t *testing.T) {
		// Don't create any directories
		result, err := ListOwners(cfg, "group")
		require.NoError(t, err)
		assert.Empty(t, result)
	})

	t.Run("list owners when scope doesn't exist", func(t *testing.T) {
		// Create a new config with a different temp path that doesn't have any directories
		newTempDir := t.TempDir()
		newCfg := config.MemoryConfig{
			StoragePath:     newTempDir,
			UserMemoryPath:  filepath.Join(newTempDir, "users"),
			GroupMemoryPath: filepath.Join(newTempDir, "groups"),
		}

		result, err := ListOwners(newCfg, "user")
		require.NoError(t, err)
		assert.Empty(t, result)
	})

	t.Run("invalid scope", func(t *testing.T) {
		_, err := ListOwners(cfg, "invalid")
		assert.Error(t, err)
	})
}

func TestListAllScopes(t *testing.T) {
	tempDir := t.TempDir()
	cfg := config.MemoryConfig{
		StoragePath:     tempDir,
		UserMemoryPath:  filepath.Join(tempDir, "users"),
		GroupMemoryPath: filepath.Join(tempDir, "groups"),
	}

	scopes := ListAllScopes(cfg)
	require.Len(t, scopes, 2)

	assert.Equal(t, types.MemoryScope("user"), scopes[0].Scope)
	assert.Equal(t, cfg.UserMemoryPath, scopes[0].BasePath)

	assert.Equal(t, types.MemoryScope("group"), scopes[1].Scope)
	assert.Equal(t, cfg.GroupMemoryPath, scopes[1].BasePath)
}

func TestValidateMemoryPath(t *testing.T) {
	tempDir := t.TempDir()
	cfg := config.MemoryConfig{
		StoragePath:     tempDir,
		UserMemoryPath:  filepath.Join(tempDir, "users"),
		GroupMemoryPath: filepath.Join(tempDir, "groups"),
	}

	t.Run("valid user memory path", func(t *testing.T) {
		ownerPath := filepath.Join(cfg.UserMemoryPath, "user123")
		filePath := filepath.Join(ownerPath, "memory.md")

		err := ValidateMemoryPath(cfg, "user", "user123", filePath)
		assert.NoError(t, err)
	})

	t.Run("valid group memory path", func(t *testing.T) {
		ownerPath := filepath.Join(cfg.GroupMemoryPath, "group1")
		filePath := filepath.Join(ownerPath, "memory.md")

		err := ValidateMemoryPath(cfg, "group", "group1", filePath)
		assert.NoError(t, err)
	})

	t.Run("path outside owner directory", func(t *testing.T) {
		// Path is in a different owner's directory
		ownerPath := filepath.Join(cfg.UserMemoryPath, "user456")
		filePath := filepath.Join(ownerPath, "memory.md")

		err := ValidateMemoryPath(cfg, "user", "user123", filePath)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "outside expected scope directory")
	})

	t.Run("path traversal attempt", func(t *testing.T) {
		// Attempt to use path traversal
		ownerPath := filepath.Join(cfg.UserMemoryPath, "user123")
		filePath := filepath.Join(ownerPath, "..", "user456", "memory.md")

		err := ValidateMemoryPath(cfg, "user", "user123", filePath)
		assert.Error(t, err)
	})

	t.Run("path in different scope", func(t *testing.T) {
		// Path is in group scope but claiming to be user scope
		ownerPath := filepath.Join(cfg.GroupMemoryPath, "group1")
		filePath := filepath.Join(ownerPath, "memory.md")

		err := ValidateMemoryPath(cfg, "user", "group1", filePath)
		assert.Error(t, err)
	})

	t.Run("empty owner ID", func(t *testing.T) {
		filePath := filepath.Join(cfg.UserMemoryPath, "memory.md")
		err := ValidateMemoryPath(cfg, "user", "", filePath)
		assert.Error(t, err)
	})
}

func TestGetScopeForPath(t *testing.T) {
	tempDir := t.TempDir()
	cfg := config.MemoryConfig{
		StoragePath:     tempDir,
		UserMemoryPath:  filepath.Join(tempDir, "users"),
		GroupMemoryPath: filepath.Join(tempDir, "groups"),
	}

	t.Run("path in user scope", func(t *testing.T) {
		filePath := filepath.Join(cfg.UserMemoryPath, "user123", "memory.md")

		scope := GetScopeForPath(cfg, filePath)
		assert.Equal(t, types.MemoryScope("user"), scope)
	})

	t.Run("path in group scope", func(t *testing.T) {
		filePath := filepath.Join(cfg.GroupMemoryPath, "group1", "memory.md")

		scope := GetScopeForPath(cfg, filePath)
		assert.Equal(t, types.MemoryScope("group"), scope)
	})

	t.Run("path outside both scopes", func(t *testing.T) {
		filePath := filepath.Join(tempDir, "other", "file.txt")

		scope := GetScopeForPath(cfg, filePath)
		assert.Equal(t, types.MemoryScope(""), scope)
	})

	t.Run("user scope base directory", func(t *testing.T) {
		scope := GetScopeForPath(cfg, cfg.UserMemoryPath)
		assert.Equal(t, types.MemoryScope("user"), scope)
	})

	t.Run("group scope base directory", func(t *testing.T) {
		scope := GetScopeForPath(cfg, cfg.GroupMemoryPath)
		assert.Equal(t, types.MemoryScope("group"), scope)
	})
}

func TestEnsureScopeDirectories(t *testing.T) {
	tempDir := t.TempDir()
	cfg := config.MemoryConfig{
		StoragePath:     tempDir,
		UserMemoryPath:  filepath.Join(tempDir, "users"),
		GroupMemoryPath: filepath.Join(tempDir, "groups"),
	}

	t.Run("create all scope directories", func(t *testing.T) {
		err := EnsureScopeDirectories(cfg)
		require.NoError(t, err)

		// Check user scope directory
		info, err := os.Stat(cfg.UserMemoryPath)
		require.NoError(t, err)
		assert.True(t, info.IsDir())

		// Check group scope directory
		info, err = os.Stat(cfg.GroupMemoryPath)
		require.NoError(t, err)
		assert.True(t, info.IsDir())
	})

	t.Run("succeeds when directories exist", func(t *testing.T) {
		// Create directories first
		err := os.MkdirAll(cfg.UserMemoryPath, 0755)
		require.NoError(t, err)
		err = os.MkdirAll(cfg.GroupMemoryPath, 0755)
		require.NoError(t, err)

		// Should still succeed
		err = EnsureScopeDirectories(cfg)
		assert.NoError(t, err)
	})
}

// TestScopeIsolation verifies that user and group scopes are strictly isolated
func TestScopeIsolation(t *testing.T) {
	tempDir := t.TempDir()
	cfg := config.MemoryConfig{
		StoragePath:     tempDir,
		UserMemoryPath:  filepath.Join(tempDir, "users"),
		GroupMemoryPath: filepath.Join(tempDir, "groups"),
	}

	t.Run("user and group paths are different", func(t *testing.T) {
		userPath, err := GetScopePath(cfg, types.MemoryScopeUser)
		require.NoError(t, err)

		groupPath, err := GetScopePath(cfg, types.MemoryScopeGroup)
		require.NoError(t, err)

		assert.NotEqual(t, userPath, groupPath)
	})

	t.Run("owners in different scopes have separate paths", func(t *testing.T) {
		const sameID = "owner123"

		userPath, err := GetOwnerPath(cfg, types.MemoryScopeUser, sameID)
		require.NoError(t, err)

		groupPath, err := GetOwnerPath(cfg, types.MemoryScopeGroup, sameID)
		require.NoError(t, err)

		assert.NotEqual(t, userPath, groupPath)

		// Verify they're in different base directories
		assert.Contains(t, userPath, cfg.UserMemoryPath)
		assert.Contains(t, groupPath, cfg.GroupMemoryPath)
	})

	t.Run("cannot access user files from group scope", func(t *testing.T) {
		// Create a user file
		userOwnerPath := filepath.Join(cfg.UserMemoryPath, "user1")
		err := os.MkdirAll(userOwnerPath, 0755)
		require.NoError(t, err)

		userFile := filepath.Join(userOwnerPath, "memory.md")
		err = os.WriteFile(userFile, []byte("user memory"), 0644)
		require.NoError(t, err)

		// Try to validate it as a group file - should fail
		err = ValidateMemoryPath(cfg, types.MemoryScopeGroup, "user1", userFile)
		assert.Error(t, err)
	})

	t.Run("list owners returns scope-specific results", func(t *testing.T) {
		// Create user directories
		user1Path := filepath.Join(cfg.UserMemoryPath, "user1")
		user2Path := filepath.Join(cfg.UserMemoryPath, "user2")
		err := os.MkdirAll(user1Path, 0755)
		require.NoError(t, err)
		err = os.MkdirAll(user2Path, 0755)
		require.NoError(t, err)

		// Create group directories
		group1Path := filepath.Join(cfg.GroupMemoryPath, "group1")
		err = os.MkdirAll(group1Path, 0755)
		require.NoError(t, err)

		// List user owners
		userOwners, err := ListOwners(cfg, types.MemoryScopeUser)
		require.NoError(t, err)
		assert.ElementsMatch(t, []string{"user1", "user2"}, userOwners)

		// List group owners
		groupOwners, err := ListOwners(cfg, types.MemoryScopeGroup)
		require.NoError(t, err)
		assert.ElementsMatch(t, []string{"group1"}, groupOwners)
	})
}

// TestEdgeCases covers edge cases and error conditions
func TestEdgeCases(t *testing.T) {
	tempDir := t.TempDir()
	cfg := config.MemoryConfig{
		StoragePath:     tempDir,
		UserMemoryPath:  filepath.Join(tempDir, "users"),
		GroupMemoryPath: filepath.Join(tempDir, "groups"),
	}

	t.Run("owner ID with only special characters", func(t *testing.T) {
		path, err := GetOwnerPath(cfg, "user", "@@@")
		require.NoError(t, err)
		// Should be sanitized to all dashes
		assert.Contains(t, path, "---")
	})

	t.Run("owner ID with unicode characters", func(t *testing.T) {
		path, err := GetOwnerPath(cfg, "user", "用户123")
		require.NoError(t, err)
		// Unicode characters should be replaced with dashes
		assert.NotContains(t, path, "用户")
	})

	t.Run("very long owner ID", func(t *testing.T) {
		longID := string(make([]byte, 1000))
		for i := range longID {
			longID = longID[:i] + "a" + longID[i+1:]
		}

		path, err := GetOwnerPath(cfg, "user", longID)
		require.NoError(t, err)
		assert.Contains(t, path, "aaa") // Should contain the sanitized ID
	})

	t.Run("empty scopes list is correct", func(t *testing.T) {
		scopes := ListAllScopes(cfg)
		require.Len(t, scopes, 2)
		// Both scopes should be present even without directories
	})
}
