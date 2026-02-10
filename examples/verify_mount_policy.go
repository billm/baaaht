package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/billm/baaaht/orchestrator/pkg/policy"
)

func main() {
	// Get the absolute path to the example file
	wd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting working directory: %v\n", err)
		os.Exit(1)
	}

	policyPath := filepath.Join(wd, "examples", "mount_policy.yaml")

	// Load the policy
	pol, err := policy.LoadFromFile(policyPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading policy: %v\n", err)
		os.Exit(1)
	}

	// Verify the policy loaded correctly
	fmt.Printf("✓ Policy loaded successfully\n")
	fmt.Printf("  ID: %s\n", pol.ID)
	fmt.Printf("  Name: %s\n", pol.Name)
	fmt.Printf("  Mode: %s\n", pol.Mode)
	fmt.Printf("  Mount allowlist entries: %d\n", len(pol.Mounts.MountAllowlist))

	// Categorize entries
	userEntries := 0
	groupEntries := 0
	defaultEntries := 0
	deniedEntries := 0

	for _, entry := range pol.Mounts.MountAllowlist {
		if entry.Mode == policy.MountAccessModeDenied {
			deniedEntries++
		}
		if entry.User != "" {
			userEntries++
		} else if entry.Group != "" {
			groupEntries++
		} else {
			defaultEntries++
		}
	}

	fmt.Printf("\n✓ Entry breakdown:\n")
	fmt.Printf("  - Per-user entries: %d\n", userEntries)
	fmt.Printf("  - Per-group entries: %d\n", groupEntries)
	fmt.Printf("  - Default entries: %d\n", defaultEntries)
	fmt.Printf("  - Explicit denials: %d\n", deniedEntries)

	// Display a few entries for verification
	fmt.Printf("\n✓ Sample mount allowlist entries:\n")
	maxShow := 5
	if len(pol.Mounts.MountAllowlist) < maxShow {
		maxShow = len(pol.Mounts.MountAllowlist)
	}

	for i := 0; i < maxShow; i++ {
		entry := pol.Mounts.MountAllowlist[i]
		scope := "all"
		if entry.User != "" {
			scope = "user:" + entry.User
		} else if entry.Group != "" {
			scope = "group:" + entry.Group
		}
		fmt.Printf("  %d. [%s] %s -> %s\n", i+1, scope, entry.Path, entry.Mode)
	}

	if len(pol.Mounts.MountAllowlist) > 5 {
		fmt.Printf("  ... and %d more entries\n", len(pol.Mounts.MountAllowlist)-5)
	}

	fmt.Printf("\n✓ Policy validation passed\n")
	fmt.Printf("\nThe mount_policy.yaml file is ready to use!\n")
}
