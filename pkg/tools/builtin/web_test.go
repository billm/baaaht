package builtin

import (
	"context"
	"net/url"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/billm/baaaht/orchestrator/pkg/types"
	"github.com/billm/baaaht/orchestrator/pkg/tools"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// Test NewWebTool
// =============================================================================

func TestNewWebTool(t *testing.T) {
	tests := []struct {
		name    string
		def     tools.ToolDefinition
		wantErr bool
		errCode string
	}{
		{
			name: "valid web tool",
			def: tools.ToolDefinition{
				Name:        "web_search",
				DisplayName: "Web Search",
				Type:        tools.ToolTypeWeb,
				Description: "Search the web",
				Enabled:     true,
			},
			wantErr: false,
		},
		{
			name: "valid web fetch tool",
			def: tools.ToolDefinition{
				Name:        "web_fetch",
				DisplayName: "Web Fetch",
				Type:        tools.ToolTypeWeb,
				Description: "Fetch a URL",
				Enabled:     true,
				SecurityPolicy: tools.ToolSecurityPolicy{
					AllowedHosts: []string{"example.com"},
					BlockedHosts: []string{"blocked.com"},
				},
			},
			wantErr: false,
		},
		{
			name: "empty name",
			def: tools.ToolDefinition{
				Name:    "",
				Enabled: true,
			},
			wantErr: true,
			errCode: types.ErrCodeInvalidArgument,
		},
		{
			name: "with custom timeout",
			def: tools.ToolDefinition{
				Name:    "web_fetch",
				Type:    tools.ToolTypeWeb,
				Enabled: true,
				Timeout: 60 * time.Second,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tool, err := NewWebTool(tt.def)

			if tt.wantErr {
				require.Error(t, err)
				assert.Nil(t, tool)
				if tt.errCode != "" {
					var customErr *types.Error
					require.ErrorAs(t, err, &customErr)
					assert.Equal(t, tt.errCode, customErr.Code)
				}
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, tool)
				assert.Equal(t, tt.def.Name, tool.Name())
				assert.Equal(t, tools.ToolTypeWeb, tool.Type())
			}
		})
	}
}

// =============================================================================
// Test Tool Interface Methods
// =============================================================================

func TestWebTool_InterfaceMethods(t *testing.T) {
	def := tools.ToolDefinition{
		Name:        "web_search",
		DisplayName: "Web Search",
		Type:        tools.ToolTypeWeb,
		Description: "Search the web",
		Enabled:     true,
	}

	tool, err := NewWebTool(def)
	require.NoError(t, err)
	require.NotNil(t, tool)

	// Test Name()
	assert.Equal(t, "web_search", tool.Name())

	// Test Type()
	assert.Equal(t, tools.ToolTypeWeb, tool.Type())

	// Test Description()
	assert.Equal(t, "Search the web", tool.Description())

	// Test Status()
	assert.Equal(t, types.StatusRunning, tool.Status())

	// Test IsAvailable()
	assert.True(t, tool.IsAvailable())

	// Test Enabled()
	assert.True(t, tool.Enabled())

	// Test SetEnabled()
	tool.SetEnabled(false)
	assert.False(t, tool.Enabled())

	tool.SetEnabled(true)
	assert.True(t, tool.Enabled())

	// Test Stats()
	stats := tool.Stats()
	assert.Equal(t, int64(0), stats.TotalExecutions)

	// Test LastUsed()
	assert.Nil(t, tool.LastUsed())

	// Test Definition()
	definition := tool.Definition()
	assert.Equal(t, "web_search", definition.Name)

	// Test Close()
	err = tool.Close()
	assert.NoError(t, err)
	assert.False(t, tool.IsAvailable())
	assert.Equal(t, types.StatusStopped, tool.Status())
}

// =============================================================================
// Test Parameter Validation
// =============================================================================

func TestWebTool_Validation(t *testing.T) {
	tests := []struct {
		name       string
		toolName   string
		parameters map[string]string
		wantErr    bool
		errCode    string
	}{
		{
			name:     "web_search with valid query",
			toolName: "web_search",
			parameters: map[string]string{
				"query": "golang tutorial",
			},
			wantErr: false,
		},
		{
			name:     "web_search with empty query",
			toolName: "web_search",
			parameters: map[string]string{
				"query": "",
			},
			wantErr: true,
			errCode: types.ErrCodeInvalidArgument,
		},
		{
			name:     "web_search with missing query",
			toolName: "web_search",
			parameters: map[string]string{},
			wantErr:    true,
			errCode:    types.ErrCodeInvalidArgument,
		},
		{
			name:     "web_fetch with valid URL",
			toolName: "web_fetch",
			parameters: map[string]string{
				"url": "https://example.com",
			},
			wantErr: false,
		},
		{
			name:     "web_fetch with empty URL",
			toolName: "web_fetch",
			parameters: map[string]string{
				"url": "",
			},
			wantErr: true,
			errCode: types.ErrCodeInvalidArgument,
		},
		{
			name:     "web_fetch with missing URL",
			toolName: "web_fetch",
			parameters: map[string]string{},
			wantErr:    true,
			errCode:    types.ErrCodeInvalidArgument,
		},
		{
			name:     "web_fetch with invalid URL",
			toolName: "web_fetch",
			parameters: map[string]string{
				"url": "://invalid-url",
			},
			wantErr: true,
			errCode: types.ErrCodeInvalidArgument,
		},
		{
			name:     "web_fetch with valid URL but missing protocol",
			toolName: "web_fetch",
			parameters: map[string]string{
				"url": "example.com",
			},
			wantErr: true,
			errCode: types.ErrCodeInvalidArgument,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			def := tools.ToolDefinition{
				Name:        tt.toolName,
				DisplayName: strings.ToTitle(tt.toolName),
				Type:        tools.ToolTypeWeb,
				Description: "Web tool",
				Enabled:     true,
			}

			tool, err := NewWebTool(def)
			require.NoError(t, err)

			err = tool.Validate(tt.parameters)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errCode != "" {
					var customErr *types.Error
					require.ErrorAs(t, err, &customErr)
					assert.Equal(t, tt.errCode, customErr.Code)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// =============================================================================
// Test Web Search
// =============================================================================

func TestWebTool_WebSearch(t *testing.T) {
	def := tools.ToolDefinition{
		Name:        "web_search",
		DisplayName: "Web Search",
		Type:        tools.ToolTypeWeb,
		Description: "Search the web",
		Enabled:     true,
	}

	tool, err := NewWebTool(def)
	require.NoError(t, err)

	tests := []struct {
		name       string
		query      string
		wantErr    bool
		wantOutput string
	}{
		{
			name:    "valid search query",
			query:   "golang tutorial",
			wantErr: false,
			wantOutput: "Web search performed for query: golang tutorial\n" +
				"Note: This is a placeholder implementation.\n" +
				"In the containerized version, this would use a search API.\n",
		},
		{
			name:    "search with special characters",
			query:   "test & query",
			wantErr: false,
			wantOutput: "Web search performed for query: test & query\n" +
				"Note: This is a placeholder implementation.\n" +
				"In the containerized version, this would use a search API.\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			params := map[string]string{
				"query": tt.query,
			}

			result, err := tool.Execute(ctx, params)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, "web_search", result.ToolName)
				assert.Equal(t, tools.ToolExecutionStatusCompleted, result.Status)
				assert.Equal(t, int32(0), result.ExitCode)
				assert.Equal(t, tt.wantOutput, result.OutputText)
			}
		})
	}
}

// =============================================================================
// Test Web Fetch
// =============================================================================

func TestWebTool_WebFetch(t *testing.T) {
	// Create a test server
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check user agent
		userAgent := r.Header.Get("User-Agent")
		assert.Contains(t, userAgent, "baaaht-web-tool")

		// Return test content
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Test response content"))
	}))
	defer testServer.Close()

	t.Run("successful fetch", func(t *testing.T) {
		def := tools.ToolDefinition{
			Name:        "web_fetch",
			DisplayName: "Web Fetch",
			Type:        tools.ToolTypeWeb,
			Description: "Fetch a URL",
			Enabled:     true,
		}

		tool, err := NewWebTool(def)
		require.NoError(t, err)

		ctx := context.Background()
		params := map[string]string{
			"url": testServer.URL,
		}

		result, err := tool.Execute(ctx, params)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, "web_fetch", result.ToolName)
		assert.Equal(t, tools.ToolExecutionStatusCompleted, result.Status)
		assert.Equal(t, int32(0), result.ExitCode)
		assert.Equal(t, "Test response content", result.OutputText)
	})

	t.Run("fetch with error status", func(t *testing.T) {
		// Server that returns 404
		errorServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte("Not found"))
		}))
		defer errorServer.Close()

		def := tools.ToolDefinition{
			Name:        "web_fetch",
			DisplayName: "Web Fetch",
			Type:        tools.ToolTypeWeb,
			Description: "Fetch a URL",
			Enabled:     true,
		}

		tool, err := NewWebTool(def)
		require.NoError(t, err)

		ctx := context.Background()
		params := map[string]string{
			"url": errorServer.URL,
		}

		result, err := tool.Execute(ctx, params)

		assert.Error(t, err)
		assert.Nil(t, result)
	})

	t.Run("fetch with blocked host", func(t *testing.T) {
		// Server with a blocked host
		blockedServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("Should not reach here"))
		}))
		defer blockedServer.Close()

		blockedParsedURL, err := url.Parse(blockedServer.URL)
		require.NoError(t, err)
		blockedHost := blockedParsedURL.Hostname()

		def := tools.ToolDefinition{
			Name:        "web_fetch",
			DisplayName: "Web Fetch",
			Type:        tools.ToolTypeWeb,
			Description: "Fetch a URL",
			Enabled:     true,
			SecurityPolicy: tools.ToolSecurityPolicy{
				BlockedHosts: []string{blockedHost},
			},
		}

		tool, err := NewWebTool(def)
		require.NoError(t, err)

		ctx := context.Background()
		params := map[string]string{
			"url": blockedServer.URL,
		}

		result, err := tool.Execute(ctx, params)

		assert.Error(t, err)
		assert.Nil(t, result)
	})

	t.Run("fetch with allowed host whitelist", func(t *testing.T) {
		// Two test servers
		allowedServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("Allowed content"))
		}))
		defer allowedServer.Close()

		allowedParsedURL, err := url.Parse(allowedServer.URL)
		require.NoError(t, err)
		allowedHost := allowedParsedURL.Hostname()

		t.Run("allowed host succeeds", func(t *testing.T) {
			def := tools.ToolDefinition{
				Name:        "web_fetch",
				DisplayName: "Web Fetch",
				Type:        tools.ToolTypeWeb,
				Description: "Fetch a URL",
				Enabled:     true,
				SecurityPolicy: tools.ToolSecurityPolicy{
					AllowedHosts: []string{allowedHost},
				},
			}

			tool, err := NewWebTool(def)
			require.NoError(t, err)

			ctx := context.Background()
			params := map[string]string{
				"url": allowedServer.URL,
			}

			result, err := tool.Execute(ctx, params)

			require.NoError(t, err)
			require.NotNil(t, result)
			assert.Equal(t, "Allowed content", result.OutputText)
		})

		t.Run("non-allowed host fails", func(t *testing.T) {
			def := tools.ToolDefinition{
				Name:        "web_fetch",
				DisplayName: "Web Fetch",
				Type:        tools.ToolTypeWeb,
				Description: "Fetch a URL",
				Enabled:     true,
				SecurityPolicy: tools.ToolSecurityPolicy{
					AllowedHosts: []string{allowedHost},
				},
			}

			tool, err := NewWebTool(def)
			require.NoError(t, err)

			ctx := context.Background()
			params := map[string]string{
				"url": "https://example.com",
			}

			result, err := tool.Execute(ctx, params)

			assert.Error(t, err)
			assert.Nil(t, result)
		})
	})

	t.Run("fetch with context cancellation", func(t *testing.T) {
		// Server that delays response
		slowServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(100 * time.Millisecond)
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("Slow response"))
		}))
		defer slowServer.Close()

		def := tools.ToolDefinition{
			Name:        "web_fetch",
			DisplayName: "Web Fetch",
			Type:        tools.ToolTypeWeb,
			Description: "Fetch a URL",
			Enabled:     true,
			Timeout:     10 * time.Millisecond, // Short timeout
		}

		tool, err := NewWebTool(def)
		require.NoError(t, err)

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		params := map[string]string{
			"url": slowServer.URL,
		}

		result, err := tool.Execute(ctx, params)

		// Should error due to context cancellation
		assert.Error(t, err)
		assert.Nil(t, result)
	})
}

// =============================================================================
// Test Stats Tracking
// =============================================================================

func TestWebTool_StatsTracking(t *testing.T) {
	def := tools.ToolDefinition{
		Name:        "web_search",
		DisplayName: "Web Search",
		Type:        tools.ToolTypeWeb,
		Description: "Search the web",
		Enabled:     true,
	}

	tool, err := NewWebTool(def)
	require.NoError(t, err)

	ctx := context.Background()

	// Execute successfully
	params := map[string]string{"query": "test"}
	_, err = tool.Execute(ctx, params)
	require.NoError(t, err)

	// Check stats
	stats := tool.Stats()
	assert.Equal(t, int64(1), stats.TotalExecutions)
	assert.Equal(t, int64(1), stats.SuccessfulExecutions)
	assert.Equal(t, int64(0), stats.FailedExecutions)
	assert.Greater(t, stats.TotalDuration, time.Duration(0))
	assert.NotNil(t, tool.LastUsed())

	// Execute with invalid params to trigger error
	invalidParams := map[string]string{}
	_, err = tool.Execute(ctx, invalidParams)
	assert.Error(t, err)

	// Check updated stats
	stats = tool.Stats()
	assert.Equal(t, int64(2), stats.TotalExecutions)
	assert.Equal(t, int64(1), stats.SuccessfulExecutions)
	assert.Equal(t, int64(1), stats.FailedExecutions)
}

// =============================================================================
// Test Closed Tool
// =============================================================================

func TestWebTool_ClosedTool(t *testing.T) {
	def := tools.ToolDefinition{
		Name:        "web_search",
		DisplayName: "Web Search",
		Type:        tools.ToolTypeWeb,
		Description: "Search the web",
		Enabled:     true,
	}

	tool, err := NewWebTool(def)
	require.NoError(t, err)

	// Close the tool
	err = tool.Close()
	require.NoError(t, err)

	// Try to execute after closing
	ctx := context.Background()
	params := map[string]string{"query": "test"}

	result, err := tool.Execute(ctx, params)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.False(t, tool.IsAvailable())
}

// =============================================================================
// Test Concurrent Execution
// =============================================================================

func TestWebTool_ConcurrentExecution(t *testing.T) {
	// Create a test server
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(10 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Response"))
	}))
	defer testServer.Close()

	def := tools.ToolDefinition{
		Name:        "web_fetch",
		DisplayName: "Web Fetch",
		Type:        tools.ToolTypeWeb,
		Description: "Fetch a URL",
		Enabled:     true,
	}

	tool, err := NewWebTool(def)
	require.NoError(t, err)

	ctx := context.Background()

	// Execute concurrently
	numGoroutines := 10
	done := make(chan bool, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			params := map[string]string{
				"url": testServer.URL,
			}
			_, _ = tool.Execute(ctx, params)
			done <- true
		}()
	}

	// Wait for all goroutines to complete
	for i := 0; i < numGoroutines; i++ {
		<-done
	}

	// Verify stats
	stats := tool.Stats()
	assert.Equal(t, int64(numGoroutines), stats.TotalExecutions)
	assert.Equal(t, int64(numGoroutines), stats.SuccessfulExecutions)
}

// =============================================================================
// Test Factory Function
// =============================================================================

func TestWebToolFactory(t *testing.T) {
	def := tools.ToolDefinition{
		Name:        "web_search",
		DisplayName: "Web Search",
		Type:        tools.ToolTypeWeb,
		Description: "Search the web",
		Enabled:     true,
	}

	tool, err := WebToolFactory(def)

	assert.NoError(t, err)
	assert.NotNil(t, tool)

	// Verify it implements the Tool interface
	var _ tools.Tool = tool
	assert.Equal(t, "web_search", tool.Name())
}

// =============================================================================
// Benchmarks
// =============================================================================

func BenchmarkWebTool_Execute(b *testing.B) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Response"))
	}))
	defer testServer.Close()

	def := tools.ToolDefinition{
		Name:        "web_fetch",
		DisplayName: "Web Fetch",
		Type:        tools.ToolTypeWeb,
		Description: "Fetch a URL",
		Enabled:     true,
	}

	tool, err := NewWebTool(def)
	require.NoError(b, err)

	ctx := context.Background()
	params := map[string]string{
		"url": testServer.URL,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = tool.Execute(ctx, params)
	}
}

func BenchmarkWebTool_Validation(b *testing.B) {
	def := tools.ToolDefinition{
		Name:        "web_fetch",
		DisplayName: "Web Fetch",
		Type:        tools.ToolTypeWeb,
		Description: "Fetch a URL",
		Enabled:     true,
	}

	tool, err := NewWebTool(def)
	require.NoError(b, err)

	params := map[string]string{
		"url": "https://example.com",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = tool.Validate(params)
	}
}
