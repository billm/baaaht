package credentials

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"github.com/billm/baaaht/orchestrator/internal/logger"
	"github.com/billm/baaaht/orchestrator/pkg/types"
)

// InjectionMethod defines how credentials are injected into containers
type InjectionMethod string

const (
	// InjectionMethodEnvVar injects credentials as environment variables
	InjectionMethodEnvVar InjectionMethod = "env_var"
	// InjectionMethodFile injects credentials as files in a mounted directory
	InjectionMethodFile InjectionMethod = "file"
	// InjectionMethodBoth injects credentials using both methods
	InjectionMethodBoth InjectionMethod = "both"
)

// InjectionConfig defines how a credential should be injected
type InjectionConfig struct {
	CredentialID   string                 `json:"credential_id"`
	CredentialName string                 `json:"credential_name"` // Alternative to ID
	Method         InjectionMethod        `json:"method"`
	EnvVarName     string                 `json:"env_var_name"`     // For env injection
	FilePath       string                 `json:"file_path"`        // For file injection
	FilePermissions string                `json:"file_permissions"` // e.g., "0600"
	MountPath      string                 `json:"mount_path"`       // Mount point for credential files
	Format         string                 `json:"format"`           // e.g., "raw", "json", "env"
	Template       string                 `json:"template"`         // Template for complex formats
	AdditionalEnv  map[string]string      `json:"additional_env"`  // Additional env vars to set
}

// InjectedCredential represents a credential that has been prepared for injection
type InjectedCredential struct {
	EnvVars   map[string]string `json:"env_vars"`
	Files     map[string]string `json:"files"`       // path -> content
	Mounts    []MountInfo       `json:"mounts"`      // mount information
	Metadata  map[string]string `json:"metadata"`    // additional metadata
}

// MountInfo represents a mount point for credential files
type MountInfo struct {
	Source      string `json:"source"`
	Destination string `json:"destination"`
	ReadOnly    bool   `json:"read_only"`
}

// Injector handles secure credential injection into containers
type Injector struct {
	store  *Store
	logger *logger.Logger
	mu     sync.RWMutex
	closed bool
}

// New creates a new credential injector
func New(store *Store, log *logger.Logger) (*Injector, error) {
	if log == nil {
		var err error
		log, err = logger.NewDefault()
		if err != nil {
			return nil, types.WrapError(types.ErrCodeInternal, "failed to create default logger", err)
		}
	}

	if store == nil {
		return nil, types.NewError(types.ErrCodeInvalidArgument, "credential store is required")
	}

	return &Injector{
		store:  store,
		logger: log.With("component", "credential_injector"),
		closed: false,
	}, nil
}

// NewDefault creates a new credential injector with the global store
func NewDefault(log *logger.Logger) (*Injector, error) {
	return New(Global(), log)
}

// Prepare prepares credentials for injection without actually injecting them
// This returns the environment variables and file contents needed
func (inj *Injector) Prepare(ctx context.Context, configs []InjectionConfig) (*InjectedCredential, error) {
	inj.mu.RLock()
	defer inj.mu.RUnlock()

	if inj.closed {
		return nil, types.NewError(types.ErrCodeUnavailable, "credential injector is closed")
	}

	if inj.store.IsClosed() {
		return nil, types.NewError(types.ErrCodeUnavailable, "credential store is closed")
	}

	result := &InjectedCredential{
		EnvVars:  make(map[string]string),
		Files:    make(map[string]string),
		Mounts:   make([]MountInfo, 0),
		Metadata: make(map[string]string),
	}

	for _, cfg := range configs {
		// Get the credential
		var cred *Credential
		var err error

		if cfg.CredentialID != "" {
			cred, err = inj.store.Get(ctx, cfg.CredentialID)
		} else if cfg.CredentialName != "" {
			cred, err = inj.store.GetByName(ctx, cfg.CredentialName)
		} else {
			return nil, types.NewError(types.ErrCodeInvalidArgument, "either credential_id or credential_name is required")
		}

		if err != nil {
			return nil, types.WrapError(types.ErrCodeNotFound, fmt.Sprintf("failed to get credential: %s or %s", cfg.CredentialID, cfg.CredentialName), err)
		}

		// Check if credential is expired
		if cred.IsExpired() {
			return nil, types.NewError(types.ErrCodePermission, fmt.Sprintf("credential %s has expired", cred.Name))
		}

		// Prepare based on injection method
		switch cfg.Method {
		case InjectionMethodEnvVar, InjectionMethodBoth:
			if err := inj.prepareEnvVar(cred, cfg, result); err != nil {
				return nil, err
			}
			fallthrough
		case InjectionMethodFile:
			if cfg.Method == InjectionMethodFile || cfg.Method == InjectionMethodBoth {
				if err := inj.prepareFile(cred, cfg, result); err != nil {
					return nil, err
				}
			}
		default:
			return nil, types.NewError(types.ErrCodeInvalidArgument, fmt.Sprintf("unknown injection method: %s", cfg.Method))
		}

		inj.logger.Debug("Credential prepared for injection",
			"credential_id", cred.ID,
			"credential_name", cred.Name,
			"method", cfg.Method)
	}

	return result, nil
}

// prepareEnvVar prepares a credential for environment variable injection
func (inj *Injector) prepareEnvVar(cred *Credential, cfg InjectionConfig, result *InjectedCredential) error {
	// Determine env var name
	envName := cfg.EnvVarName
	if envName == "" {
		// Default: convert to uppercase and add prefix
		envName = fmt.Sprintf("CRED_%s_%s", strings.ToUpper(cred.Type), strings.ToUpper(cred.Name))
		// Sanitize the name
		envName = strings.Map(func(r rune) rune {
			if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
				return r
			}
			return '_'
		}, envName)
	}

	result.EnvVars[envName] = cred.Value

	// Add additional environment variables
	for k, v := range cfg.AdditionalEnv {
		result.EnvVars[k] = v
	}

	// Add metadata
	result.Metadata[fmt.Sprintf("cred_%s_id", cred.Name)] = cred.ID
	result.Metadata[fmt.Sprintf("cred_%s_type", cred.Name)] = cred.Type

	return nil
}

// prepareFile prepares a credential for file injection
func (inj *Injector) prepareFile(cred *Credential, cfg InjectionConfig, result *InjectedCredential) error {
	// Determine file path
	filePath := cfg.FilePath
	if filePath == "" {
		// Default: use credential name as filename
		filePath = fmt.Sprintf("/%s", cred.Name)
	}

	// Determine mount path
	mountPath := cfg.MountPath
	if mountPath == "" {
		mountPath = "/credentials"
	}

	// Full path for the file
	fullPath, err := filepathWithBase(mountPath, filePath)
	if err != nil {
		return types.WrapError(types.ErrCodeInvalidArgument, "invalid file path", err)
	}

	// Format the content based on the format setting
	content := cred.Value
	switch cfg.Format {
	case "json":
		// Use proper JSON marshaling to ensure correct escaping
		jsonData := map[string]string{
			"value": cred.Value,
			"type":  cred.Type,
			"name":  cred.Name,
		}
		jsonBytes, err := json.Marshal(jsonData)
		if err != nil {
			return types.WrapError(types.ErrCodeInternal, "failed to marshal credential to JSON", err)
		}
		content = string(jsonBytes)
	case "env":
		content = fmt.Sprintf("%s=%s\n", strings.ToUpper(cred.Name), cred.Value)
	case "raw", "":
		// Use the raw value
	default:
		if cfg.Template != "" {
			// Use template (simple variable substitution)
			content = strings.ReplaceAll(cfg.Template, "{{value}}", cred.Value)
			content = strings.ReplaceAll(content, "{{type}}", cred.Type)
			content = strings.ReplaceAll(content, "{{name}}", cred.Name)
			for k, v := range cred.Metadata {
				content = strings.ReplaceAll(content, fmt.Sprintf("{{metadata.%s}}", k), v)
			}
		}
	}

	result.Files[fullPath] = content

	// Add mount info if not already present
	mountExists := false
	for _, m := range result.Mounts {
		if m.Destination == mountPath {
			mountExists = true
			break
		}
	}

	if !mountExists {
		result.Mounts = append(result.Mounts, MountInfo{
			Source:      fmt.Sprintf("/tmp/credentials/%s", cred.ID),
			Destination: mountPath,
			ReadOnly:    true,
		})
	}

	return nil
}

// filepathWithBase joins base and path safely, preventing path traversal
func filepathWithBase(base, path string) (string, error) {
	// Remove leading slash from path
	if strings.HasPrefix(path, "/") {
		path = path[1:]
	}
	
	// Use filepath.Join for proper path handling
	fullPath := filepath.Join(base, path)
	
	// Clean the path to remove any .. or . components
	fullPath = filepath.Clean(fullPath)
	
	// Ensure the final path is still within the base directory
	// by checking if it starts with the cleaned base path
	cleanBase := filepath.Clean(base)
	if !strings.HasPrefix(fullPath, cleanBase) {
		// Path traversal attempt detected - return error
		return "", fmt.Errorf("path traversal attempt detected: %s escapes base directory %s", path, base)
	}
	
	return fullPath, nil
}

// ValidateConfig validates an injection configuration
func (inj *Injector) ValidateConfig(ctx context.Context, cfg InjectionConfig) error {
	if cfg.CredentialID == "" && cfg.CredentialName == "" {
		return types.NewError(types.ErrCodeInvalidArgument, "either credential_id or credential_name is required")
	}

	// Verify the credential exists
	var cred *Credential
	var err error

	if cfg.CredentialID != "" {
		cred, err = inj.store.Get(ctx, cfg.CredentialID)
	} else {
		cred, err = inj.store.GetByName(ctx, cfg.CredentialName)
	}

	if err != nil {
		return types.WrapError(types.ErrCodeNotFound, "credential not found", err)
	}

	// Check expiration
	if cred.IsExpired() {
		return types.NewError(types.ErrCodePermission, "credential has expired")
	}

	// Validate injection method
	switch cfg.Method {
	case InjectionMethodEnvVar, InjectionMethodFile, InjectionMethodBoth:
		// Valid methods
	default:
		return types.NewError(types.ErrCodeInvalidArgument, fmt.Sprintf("unknown injection method: %s", cfg.Method))
	}

	// Validate file-specific settings
	if cfg.Method == InjectionMethodFile || cfg.Method == InjectionMethodBoth {
		if cfg.FilePath == "" && cfg.Template == "" {
			// Use default file path, which is fine
		}
	}

	return nil
}

// GetInjectionEnvVars returns only the environment variables for injection
func (inj *Injector) GetInjectionEnvVars(ctx context.Context, configs []InjectionConfig) (map[string]string, error) {
	injected, err := inj.Prepare(ctx, configs)
	if err != nil {
		return nil, err
	}
	return injected.EnvVars, nil
}

// GetInjectionFiles returns only the file contents for injection
func (inj *Injector) GetInjectionFiles(ctx context.Context, configs []InjectionConfig) (map[string]string, error) {
	injected, err := inj.Prepare(ctx, configs)
	if err != nil {
		return nil, err
	}
	return injected.Files, nil
}

// GetInjectionMounts returns the mount points required for file injection
func (inj *Injector) GetInjectionMounts(ctx context.Context, configs []InjectionConfig) ([]MountInfo, error) {
	injected, err := inj.Prepare(ctx, configs)
	if err != nil {
		return nil, err
	}
	return injected.Mounts, nil
}

// VerifyAccess verifies that a credential can be accessed (used for authorization checks)
func (inj *Injector) VerifyAccess(ctx context.Context, credentialID string, sessionID string) error {
	// Get the credential to ensure it exists and is not expired
	cred, err := inj.store.Get(ctx, credentialID)
	if err != nil {
		return err
	}

	// Check expiration
	if cred.IsExpired() {
		return types.NewError(types.ErrCodePermission, "credential has expired")
	}

	// Log the access for audit purposes
	inj.logger.Info("Credential access verified",
		"credential_id", credentialID,
		"credential_name", cred.Name,
		"session_id", sessionID)

	return nil
}

// ListInjectableCredentials lists all credentials that can be injected
func (inj *Injector) ListInjectableCredentials(ctx context.Context) ([]*Credential, error) {
	allCreds, err := inj.store.List(ctx)
	if err != nil {
		return nil, err
	}

	// Filter out expired credentials
	result := make([]*Credential, 0)
	for _, cred := range allCreds {
		if !cred.IsExpired() {
			result = append(result, cred)
		}
	}

	return result, nil
}

// Close closes the injector
func (inj *Injector) Close() error {
	inj.mu.Lock()
	defer inj.mu.Unlock()

	if inj.closed {
		return nil
	}

	inj.closed = true
	inj.logger.Info("Credential injector closed")
	return nil
}

// IsClosed returns true if the injector is closed
func (inj *Injector) IsClosed() bool {
	inj.mu.RLock()
	defer inj.mu.RUnlock()
	return inj.closed
}

// Global injector instance
var (
	globalInjector *Injector
	injectorGlobalOnce     sync.Once
)

// InitGlobalInjector initializes the global credential injector
func InitGlobalInjector(log *logger.Logger) error {
	var initErr error
	injectorGlobalOnce.Do(func() {
		injector, err := NewDefault(log)
		if err != nil {
			initErr = err
			return
		}
		globalInjector = injector
	})
	return initErr
}

// GlobalInjector returns the global credential injector instance
func GlobalInjector() *Injector {
	if globalInjector == nil {
		// Initialize with default settings if not already initialized
		injector, err := NewDefault(nil)
		if err != nil {
			// Return a closed injector on error
			return &Injector{closed: true}
		}
		globalInjector = injector
	}
	return globalInjector
}

// SetGlobalInjector sets the global credential injector instance
func SetGlobalInjector(inj *Injector) {
	globalInjector = inj
	injectorGlobalOnce = sync.Once{}
}
