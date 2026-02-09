package policy

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/billm/baaaht/orchestrator/pkg/types"
	"gopkg.in/yaml.v3"
)

// envVarPattern matches ${VAR_NAME} and ${VAR_NAME:-default}
var envVarPattern = regexp.MustCompile(`\$\{([a-zA-Z_][a-zA-Z0-9_]*)(:-([^}]*))?\}`)

// interpolateEnvVars replaces environment variable placeholders with their values
// Supports ${VAR_NAME} and ${VAR_NAME:-default_value} syntax
func interpolateEnvVars(s string) string {
	return envVarPattern.ReplaceAllStringFunc(s, func(match string) string {
		// Extract the variable name and default value
		parts := envVarPattern.FindStringSubmatch(match)
		if len(parts) < 2 {
			return match // No match found, return original
		}

		varName := parts[1]
		defaultValue := ""
		if len(parts) >= 4 && parts[3] != "" {
			defaultValue = parts[3]
		}

		// Get the environment variable value
		if value := os.Getenv(varName); value != "" {
			return value
		}

		// Return default value if env var is not set
		return defaultValue
	})
}

// validateFilePath checks if the file path is valid and has the correct extension
func validateFilePath(path string) error {
	// Check if the path is empty
	if path == "" {
		return types.NewError(types.ErrCodeInvalidArgument, "policy file path cannot be empty")
	}

	// Check file extension
	ext := strings.ToLower(filepath.Ext(path))
	if ext != ".yaml" && ext != ".yml" {
		return types.NewError(types.ErrCodeInvalidArgument,
			"policy file must have .yaml or .yml extension, got: "+ext)
	}

	return nil
}

// validateYAMLContent validates the YAML content and provides detailed error messages
func validateYAMLContent(data []byte, path string) error {
	// Check for empty file
	if len(data) == 0 {
		return types.NewError(types.ErrCodeInvalid, "policy file is empty: "+path)
	}

	// Check for file that's only whitespace
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" {
		return types.NewError(types.ErrCodeInvalid, "policy file contains only whitespace: "+path)
	}

	// Parse YAML to validate syntax
	var node yaml.Node
	if err := yaml.Unmarshal(data, &node); err != nil {
		// Enhance YAML error with context
		return types.WrapError(types.ErrCodeInvalid, "invalid YAML syntax in "+path, err)
	}

	// Check if the document is empty (no actual content)
	if node.Kind == 0 && len(node.Content) == 0 {
		return types.NewError(types.ErrCodeInvalid, "policy file contains no valid YAML content: "+path)
	}

	return nil
}

// formatYAMLError formats a YAML error with file context
func formatYAMLError(err error, path string) error {
	if err == nil {
		return nil
	}

	// Check if it's a YAML parse error with line/column information
	if yamlErr, ok := err.(*yaml.TypeError); ok {
		return types.WrapError(types.ErrCodeInvalid, "YAML type error in "+path, yamlErr)
	}

	// For other errors, wrap with file context
	return types.WrapError(types.ErrCodeInvalid, "failed to parse YAML policy from "+path, err)
}

// LoadFromFile loads policy from a YAML file
func LoadFromFile(path string) (*Policy, error) {
	// Validate file path
	if err := validateFilePath(path); err != nil {
		return nil, err
	}

	// Read the file
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, types.WrapError(types.ErrCodeNotFound, "policy file not found: "+path, err)
		}
		return nil, types.WrapError(types.ErrCodeInvalidArgument, "failed to read policy file: "+path, err)
	}

	// Validate YAML content before parsing
	if err := validateYAMLContent(data, path); err != nil {
		return nil, err
	}

	// Parse YAML
	var pol Policy
	if err := yaml.Unmarshal(data, &pol); err != nil {
		return nil, formatYAMLError(err, path)
	}

	// Interpolate environment variables in all string fields
	interpolateEnvVarsInPolicy(&pol)

	// Apply defaults to any zero-valued fields that weren't specified in the YAML
	applyDefaults(&pol)

	// Validate the policy
	if err := pol.Validate(); err != nil {
		return nil, types.WrapError(types.ErrCodeInvalid, "policy validation failed for "+path, err)
	}

	return &pol, nil
}

// interpolateEnvVarsInPolicy interpolates environment variables in all string fields
func interpolateEnvVarsInPolicy(pol *Policy) {
	// Policy metadata
	pol.ID = interpolateEnvVars(pol.ID)
	pol.Name = interpolateEnvVars(pol.Name)
	pol.Description = interpolateEnvVars(pol.Description)

	// Labels
	if pol.Labels != nil {
		for k, v := range pol.Labels {
			pol.Labels[k] = interpolateEnvVars(v)
		}
	}

	// Note: We don't interpolate numeric fields like quotas
	// We only interpolate string fields in lists
	for i := range pol.Mounts.AllowedBindSources {
		pol.Mounts.AllowedBindSources[i] = interpolateEnvVars(pol.Mounts.AllowedBindSources[i])
	}
	for i := range pol.Mounts.DeniedBindSources {
		pol.Mounts.DeniedBindSources[i] = interpolateEnvVars(pol.Mounts.DeniedBindSources[i])
	}
	for i := range pol.Mounts.AllowedVolumes {
		pol.Mounts.AllowedVolumes[i] = interpolateEnvVars(pol.Mounts.AllowedVolumes[i])
	}
	for i := range pol.Mounts.DeniedVolumes {
		pol.Mounts.DeniedVolumes[i] = interpolateEnvVars(pol.Mounts.DeniedVolumes[i])
	}

	// Network policy
	for i := range pol.Network.AllowedNetworks {
		pol.Network.AllowedNetworks[i] = interpolateEnvVars(pol.Network.AllowedNetworks[i])
	}
	for i := range pol.Network.DeniedNetworks {
		pol.Network.DeniedNetworks[i] = interpolateEnvVars(pol.Network.DeniedNetworks[i])
	}

	// Image policy
	for i := range pol.Images.AllowedImages {
		pol.Images.AllowedImages[i] = interpolateEnvVars(pol.Images.AllowedImages[i])
	}
	for i := range pol.Images.DeniedImages {
		pol.Images.DeniedImages[i] = interpolateEnvVars(pol.Images.DeniedImages[i])
	}

	// Security policy - capabilities
	for i := range pol.Security.AllowedCapabilities {
		pol.Security.AllowedCapabilities[i] = interpolateEnvVars(pol.Security.AllowedCapabilities[i])
	}
	for i := range pol.Security.AddCapabilities {
		pol.Security.AddCapabilities[i] = interpolateEnvVars(pol.Security.AddCapabilities[i])
	}
	for i := range pol.Security.DropCapabilities {
		pol.Security.DropCapabilities[i] = interpolateEnvVars(pol.Security.DropCapabilities[i])
	}
}

// applyDefaults applies default values to any zero-valued fields
func applyDefaults(pol *Policy) {
	// Set default ID if empty
	if pol.ID == "" {
		pol.ID = "default"
	}

	// Set default name if empty
	if pol.Name == "" {
		pol.Name = "Policy"
	}

	// Set default enforcement mode if empty
	if pol.Mode == "" {
		pol.Mode = EnforcementModeStrict
	}
}

// LoadFromBytes loads policy from a YAML byte slice
func LoadFromBytes(data []byte) (*Policy, error) {
	// Validate YAML content before parsing
	if err := validateYAMLContent(data, "<bytes>"); err != nil {
		return nil, err
	}

	// Parse YAML
	var pol Policy
	if err := yaml.Unmarshal(data, &pol); err != nil {
		return nil, formatYAMLError(err, "<bytes>")
	}

	// Interpolate environment variables
	interpolateEnvVarsInPolicy(&pol)

	// Apply defaults
	applyDefaults(&pol)

	// Validate the policy
	if err := pol.Validate(); err != nil {
		return nil, types.WrapError(types.ErrCodeInvalid, "policy validation failed", err)
	}

	return &pol, nil
}
