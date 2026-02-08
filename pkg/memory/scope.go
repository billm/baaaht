package memory

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/billm/baaaht/orchestrator/internal/config"
	"github.com/billm/baaaht/orchestrator/pkg/types"
)

// ScopeInfo contains information about a memory scope
type ScopeInfo struct {
	Scope    types.MemoryScope `json:"scope"`
	BasePath string            `json:"base_path"`
}

// GetScopePath returns the base directory path for a given scope
func GetScopePath(cfg config.MemoryConfig, scope types.MemoryScope) (string, error) {
	if err := ValidateScope(scope); err != nil {
		return "", err
	}

	switch scope {
	case types.MemoryScopeUser:
		return cfg.UserMemoryPath, nil
	case types.MemoryScopeGroup:
		return cfg.GroupMemoryPath, nil
	default:
		return "", types.NewError(types.ErrCodeInvalidArgument, fmt.Sprintf("unknown memory scope: %s", scope))
	}
}

// ValidateScope validates that a scope value is valid
func ValidateScope(scope types.MemoryScope) error {
	switch scope {
	case types.MemoryScopeUser, types.MemoryScopeGroup:
		return nil
	default:
		return types.NewError(types.ErrCodeInvalidArgument, fmt.Sprintf("invalid memory scope: %s (must be 'user' or 'group')", scope))
	}
}

// GetOwnerPath returns the full directory path for a specific scope and owner
func GetOwnerPath(cfg config.MemoryConfig, scope types.MemoryScope, ownerID string) (string, error) {
	if ownerID == "" {
		return "", types.NewError(types.ErrCodeInvalidArgument, "owner ID cannot be empty")
	}

	if err := ValidateScope(scope); err != nil {
		return "", err
	}

	// Sanitize owner ID for filesystem safety
	sanitizedOwnerID := sanitizeOwnerID(ownerID)

	scopePath, err := GetScopePath(cfg, scope)
	if err != nil {
		return "", err
	}

	return filepath.Join(scopePath, sanitizedOwnerID), nil
}

// sanitizeOwnerID sanitizes an owner ID for safe filesystem usage
func sanitizeOwnerID(ownerID string) string {
	// Remove any path components that could lead to directory traversal
	ownerID = filepath.Base(ownerID)
	// Remove any non-alphanumeric characters (except dash, underscore, dot)
	ownerID = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			return r
		}
		return '-'
	}, ownerID)
	return ownerID
}

// CreateScopeDir creates the base directory for a scope if it doesn't exist
func CreateScopeDir(cfg config.MemoryConfig, scope types.MemoryScope) error {
	scopePath, err := GetScopePath(cfg, scope)
	if err != nil {
		return err
	}

	// Create directory with appropriate permissions
	if err := os.MkdirAll(scopePath, 0755); err != nil {
		return types.WrapError(types.ErrCodeInternal, fmt.Sprintf("failed to create %s scope directory", scope), err)
	}

	return nil
}

// CreateOwnerDir creates the directory for a specific scope and owner if it doesn't exist
func CreateOwnerDir(cfg config.MemoryConfig, scope types.MemoryScope, ownerID string) (string, error) {
	ownerPath, err := GetOwnerPath(cfg, scope, ownerID)
	if err != nil {
		return "", err
	}

	// Create directory with appropriate permissions
	if err := os.MkdirAll(ownerPath, 0755); err != nil {
		return "", types.WrapError(types.ErrCodeInternal, fmt.Sprintf("failed to create owner directory for %s/%s", scope, ownerID), err)
	}

	return ownerPath, nil
}

// ScopeExists checks if a scope directory exists
func ScopeExists(cfg config.MemoryConfig, scope types.MemoryScope) (bool, error) {
	scopePath, err := GetScopePath(cfg, scope)
	if err != nil {
		return false, err
	}

	info, err := os.Stat(scopePath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, types.WrapError(types.ErrCodeInternal, "failed to check scope directory", err)
	}

	return info.IsDir(), nil
}

// OwnerDirExists checks if an owner directory exists within a scope
func OwnerDirExists(cfg config.MemoryConfig, scope types.MemoryScope, ownerID string) (bool, error) {
	ownerPath, err := GetOwnerPath(cfg, scope, ownerID)
	if err != nil {
		return false, err
	}

	info, err := os.Stat(ownerPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, types.WrapError(types.ErrCodeInternal, "failed to check owner directory", err)
	}

	return info.IsDir(), nil
}

// ListOwners returns a list of all owner IDs that have memories in the given scope
func ListOwners(cfg config.MemoryConfig, scope types.MemoryScope) ([]string, error) {
	if err := ValidateScope(scope); err != nil {
		return nil, err
	}

	scopePath, err := GetScopePath(cfg, scope)
	if err != nil {
		return nil, err
	}

	// Check if scope directory exists
	exists, err := ScopeExists(cfg, scope)
	if err != nil {
		return nil, err
	}
	if !exists {
		return []string{}, nil
	}

	// Read directory entries
	entries, err := os.ReadDir(scopePath)
	if err != nil {
		return nil, types.WrapError(types.ErrCodeInternal, "failed to read scope directory", err)
	}

	owners := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			owners = append(owners, entry.Name())
		}
	}

	return owners, nil
}

// ListAllScopes returns information about all configured scopes
func ListAllScopes(cfg config.MemoryConfig) []ScopeInfo {
	return []ScopeInfo{
		{
			Scope:    types.MemoryScopeUser,
			BasePath: cfg.UserMemoryPath,
		},
		{
			Scope:    types.MemoryScopeGroup,
			BasePath: cfg.GroupMemoryPath,
		},
	}
}

// ValidateMemoryPath validates that a memory's file path is within the expected scope directory
// This prevents directory traversal attacks
func ValidateMemoryPath(cfg config.MemoryConfig, scope types.MemoryScope, ownerID, filePath string) error {
	// Get the expected owner directory
	expectedOwnerPath, err := GetOwnerPath(cfg, scope, ownerID)
	if err != nil {
		return err
	}

	// Make the expected path absolute for comparison
	expectedOwnerPath, err = filepath.Abs(expectedOwnerPath)
	if err != nil {
		return types.WrapError(types.ErrCodeInternal, "failed to resolve owner path", err)
	}

	// Make the file path absolute
	absFilePath, err := filepath.Abs(filePath)
	if err != nil {
		return types.WrapError(types.ErrCodeInternal, "failed to resolve file path", err)
	}

	// Check that the file path is within the expected owner directory
	relPath, err := filepath.Rel(expectedOwnerPath, absFilePath)
	if err != nil {
		return types.NewError(types.ErrCodePermission, "file path is not within expected scope directory")
	}

	// Check for path traversal (relPath starting with "..")
	if strings.HasPrefix(relPath, "..") {
		return types.NewError(types.ErrCodePermission, "file path is outside expected scope directory")
	}

	return nil
}

// GetScopeForPath determines which scope a file path belongs to
// Returns empty scope if the path doesn't match any scope
func GetScopeForPath(cfg config.MemoryConfig, filePath string) types.MemoryScope {
	userPath, err := filepath.Abs(cfg.UserMemoryPath)
	if err == nil {
		absFilePath, err := filepath.Abs(filePath)
		if err == nil {
			relPath, err := filepath.Rel(userPath, absFilePath)
			if err == nil && !strings.HasPrefix(relPath, "..") {
				return types.MemoryScopeUser
			}
		}
	}

	groupPath, err := filepath.Abs(cfg.GroupMemoryPath)
	if err == nil {
		absFilePath, err := filepath.Abs(filePath)
		if err == nil {
			relPath, err := filepath.Rel(groupPath, absFilePath)
			if err == nil && !strings.HasPrefix(relPath, "..") {
				return types.MemoryScopeGroup
			}
		}
	}

	return ""
}

// EnsureScopeDirectories ensures all scope directories exist
func EnsureScopeDirectories(cfg config.MemoryConfig) error {
	scopes := []types.MemoryScope{types.MemoryScopeUser, types.MemoryScopeGroup}

	for _, scope := range scopes {
		if err := CreateScopeDir(cfg, scope); err != nil {
			return types.WrapError(types.ErrCodeInternal, fmt.Sprintf("failed to create scope directory: %s", scope), err)
		}
	}

	return nil
}
