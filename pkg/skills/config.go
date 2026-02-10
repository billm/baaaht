package skills

import (
	"os"
	"path/filepath"
	"time"

	"github.com/billm/baaaht/orchestrator/pkg/types"
)

// DefaultSkillConfig returns the default skill configuration
func DefaultSkillConfig() types.SkillConfig {
	storagePath := "/var/lib/baaaht/skills" // fallback
	if configDir, err := os.UserConfigDir(); err == nil {
		storagePath = filepath.Join(configDir, "baaaht", "skills")
	}

	return types.SkillConfig{
		Enabled:           true,
		StoragePath:       storagePath,
		MaxSkillsPerOwner: 100,
		AutoLoad:          true,
		LoadConfig: types.SkillLoadConfig{
			Enabled:       true,
			SkillPaths:    []string{storagePath},
			Recursive:     true,
			WatchChanges:  false,
			MaxLoadErrors: 10,
		},
		GitHubConfig: types.SkillGitHubConfig{
			Enabled:        false,
			APIEndpoint:    "https://api.github.com",
			MaxRepoSkills:  50,
			AutoUpdate:     false,
			UpdateInterval: 24 * time.Hour,
		},
		Retention: types.SkillRetention{
			Enabled:          false,
			MaxAge:           90 * 24 * time.Hour, // 90 days
			UnusedMaxAge:     180 * 24 * time.Hour, // 180 days
			ErrorMaxAge:      30 * 24 * time.Hour,  // 30 days
			MinLoadCount:     0,
			PreserveVerified: true,
		},
	}
}
