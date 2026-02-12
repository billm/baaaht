package builtin

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/billm/baaaht/orchestrator/internal/logger"
	"github.com/billm/baaaht/orchestrator/pkg/types"
	"github.com/billm/baaaht/orchestrator/pkg/tools"
)

// =============================================================================
// Web Tool Base
// =============================================================================

// WebTool implements the Tool interface for web operations
type WebTool struct {
	name         string
	displayName  string
	definition   tools.ToolDefinition
	logger       *logger.Logger
	mu           sync.RWMutex
	enabled      bool
	stats        tools.ToolUsageStats
	lastUsed     *time.Time
	closed       bool
	allowedHosts []string
	blockedHosts []string
	client       *http.Client
}

// NewWebTool creates a new web tool with the given definition
func NewWebTool(def tools.ToolDefinition) (*WebTool, error) {
	if def.Name == "" {
		return nil, types.NewError(types.ErrCodeInvalidArgument, "tool name cannot be empty")
	}

	log, err := logger.NewDefault()
	if err != nil {
		log = &logger.Logger{}
	}

	// Extract allowed/blocked hosts from security policy
	allowedHosts := def.SecurityPolicy.AllowedHosts
	if allowedHosts == nil {
		allowedHosts = []string{}
	}

	blockedHosts := def.SecurityPolicy.BlockedHosts
	if blockedHosts == nil {
		blockedHosts = []string{}
	}

	// Create HTTP client with timeout
	timeout := 30 * time.Second
	if def.Timeout > 0 {
		timeout = def.Timeout
	}

	return &WebTool{
		name:         def.Name,
		displayName:  def.DisplayName,
		definition:   def,
		logger:       log.With("component", fmt.Sprintf("web_tool_%s", def.Name)),
		enabled:      def.Enabled,
		allowedHosts: allowedHosts,
		blockedHosts: blockedHosts,
		client: &http.Client{
			Timeout: timeout,
		},
	}, nil
}

// Name returns the tool name
func (w *WebTool) Name() string {
	return w.name
}

// Type returns the tool type
func (w *WebTool) Type() tools.ToolType {
	return tools.ToolTypeWeb
}

// Description returns the tool description
func (w *WebTool) Description() string {
	return w.definition.Description
}

// Execute executes the web tool operation
func (w *WebTool) Execute(ctx context.Context, parameters map[string]string) (*tools.ToolExecutionResult, error) {
	startTime := time.Now()

	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed {
		return nil, types.NewError(types.ErrCodeUnavailable, "tool is closed")
	}

	// Update stats
	w.stats.TotalExecutions++
	now := time.Now()
	w.lastUsed = &now

	// Validate parameters first
	if err := w.Validate(parameters); err != nil {
		w.stats.FailedExecutions++
		return nil, err
	}

	// Execute the specific web operation
	var output string
	var exitCode int32
	var err error

	switch w.name {
	case "web_search":
		output, exitCode, err = w.webSearch(ctx, parameters)
	case "web_fetch":
		output, exitCode, err = w.webFetch(ctx, parameters)
	default:
		w.stats.FailedExecutions++
		return nil, types.NewError(types.ErrCodeInternal, fmt.Sprintf("unknown web tool: %s", w.name))
	}

	duration := time.Since(startTime)
	w.stats.TotalDuration += duration

	if err != nil {
		w.stats.FailedExecutions++
		// Return error for execution failures
		return nil, types.WrapError(types.ErrCodeInternal, fmt.Sprintf("%s execution failed", w.name), err)
	}

	w.stats.SuccessfulExecutions++
	w.stats.AverageDuration = time.Duration(int64(w.stats.TotalDuration) / w.stats.TotalExecutions)

	return &tools.ToolExecutionResult{
		ToolName:    w.name,
		Status:      tools.ToolExecutionStatusCompleted,
		ExitCode:    exitCode,
		OutputText:  output,
		OutputData:  []byte(output),
		Duration:    duration,
		CompletedAt: time.Now(),
	}, nil
}

// Validate validates the tool parameters
func (w *WebTool) Validate(parameters map[string]string) error {
	// Check required parameters
	for _, param := range w.definition.Parameters {
		if param.Required {
			if value, ok := parameters[param.Name]; !ok || value == "" {
				return types.NewError(types.ErrCodeInvalidArgument,
					fmt.Sprintf("missing required parameter: %s", param.Name))
			}
		}
	}

	// Perform tool-specific validation
	switch w.name {
	case "web_search":
		query := parameters["query"]
		if query == "" {
			return types.NewError(types.ErrCodeInvalidArgument, "search query cannot be empty")
		}
	case "web_fetch":
		urlStr := parameters["url"]
		if urlStr == "" {
			return types.NewError(types.ErrCodeInvalidArgument, "URL cannot be empty")
		}
		// Validate URL format
		if _, err := url.Parse(urlStr); err != nil {
			return types.NewError(types.ErrCodeInvalidArgument,
				fmt.Sprintf("invalid URL format: %w", err))
		}
	}

	return nil
}

// Definition returns the tool definition
func (w *WebTool) Definition() tools.ToolDefinition {
	// Update the definition with runtime info
	def := w.definition
	def.Enabled = w.Enabled()
	return def
}

// Status returns the current status
func (w *WebTool) Status() types.Status {
	w.mu.RLock()
	defer w.mu.RUnlock()

	if w.closed {
		return types.StatusStopped
	}
	return types.StatusRunning
}

// IsAvailable returns true if the tool is available
func (w *WebTool) IsAvailable() bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return !w.closed
}

// Enabled returns true if the tool is enabled
func (w *WebTool) Enabled() bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.enabled
}

// SetEnabled enables or disables the tool
func (w *WebTool) SetEnabled(enabled bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.enabled = enabled
}

// Stats returns usage statistics
func (w *WebTool) Stats() tools.ToolUsageStats {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.stats
}

// LastUsed returns the last used time
func (w *WebTool) LastUsed() *time.Time {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.lastUsed
}

// Close closes the tool
func (w *WebTool) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.closed = true
	return nil
}

// =============================================================================
// Web Operations
// =============================================================================

// webSearch performs a web search and returns results
func (w *WebTool) webSearch(ctx context.Context, parameters map[string]string) (string, int32, error) {
	query := parameters["query"]

	// In a real implementation, this would call a search API
	// For now, we return a placeholder response indicating the search query
	// The actual containerized implementation would use a search service

	result := fmt.Sprintf("Web search performed for query: %s\n", query)
	result += "Note: This is a placeholder implementation.\n"
	result += "In the containerized version, this would use a search API.\n"

	return result, 0, nil
}

// webFetch fetches a URL and returns its content
func (w *WebTool) webFetch(ctx context.Context, parameters map[string]string) (string, int32, error) {
	urlStr := parameters["url"]

	// Parse URL to extract hostname for validation
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return "", 1, fmt.Errorf("failed to parse URL: %w", err)
	}

	// Check against blocked hosts
	if err := w.checkBlockedHosts(parsedURL.Hostname()); err != nil {
		return "", 1, err
	}

	// Check against allowed hosts (if configured)
	if err := w.checkAllowedHosts(parsedURL.Hostname()); err != nil {
		return "", 1, err
	}

	// Create the request
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlStr, nil)
	if err != nil {
		return "", 1, fmt.Errorf("failed to create request: %w", err)
	}

	// Set user agent
	req.Header.Set("User-Agent", "baaaht-web-tool/1.0")

	// Execute the request
	resp, err := w.client.Do(req)
	if err != nil {
		return "", 1, fmt.Errorf("failed to fetch URL: %w", err)
	}
	defer resp.Body.Close()

	// Check status code
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", int32(resp.StatusCode), fmt.Errorf("HTTP request failed with status: %s", resp.Status)
	}

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", 1, fmt.Errorf("failed to read response body: %w", err)
	}

	return string(body), 0, nil
}

// =============================================================================
// Helper Functions
// =============================================================================

// checkBlockedHosts checks if a host is in the blocked list
func (w *WebTool) checkBlockedHosts(host string) error {
	hostLower := strings.ToLower(strings.TrimSpace(host))

	for _, blocked := range w.blockedHosts {
		blockedLower := strings.ToLower(strings.TrimSpace(blocked))
		// Check for exact match or suffix match (for subdomains)
		if hostLower == blockedLower ||
			strings.HasSuffix(hostLower, "."+blockedLower) {
			return types.NewError(types.ErrCodePermissionDenied,
				fmt.Sprintf("host '%s' is blocked by security policy", host))
		}
	}

	return nil
}

// checkAllowedHosts checks if a host is in the allowed list (if configured)
func (w *WebTool) checkAllowedHosts(host string) error {
	// If no allowed hosts configured, allow all
	if len(w.allowedHosts) == 0 {
		return nil
	}

	hostLower := strings.ToLower(strings.TrimSpace(host))

	for _, allowed := range w.allowedHosts {
		allowedLower := strings.ToLower(strings.TrimSpace(allowed))
		// Check for exact match or suffix match (for subdomains)
		if hostLower == allowedLower ||
			strings.HasSuffix(hostLower, "."+allowedLower) {
			return nil
		}
	}

	return types.NewError(types.ErrCodePermissionDenied,
		fmt.Sprintf("host '%s' is not in the allowed list", host))
}

// =============================================================================
// Factory Functions
// =============================================================================

// WebToolFactory creates a web tool from a definition
func WebToolFactory(def tools.ToolDefinition) (tools.Tool, error) {
	return NewWebTool(def)
}
