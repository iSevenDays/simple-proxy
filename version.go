package main

import (
	"fmt"
	"os/exec"
	"strings"
	"time"
)

var (
	// Set at build time via go build -ldflags
	Version   = "dev"
	GitCommit = "unknown"
	BuildTime = "unknown"
)

// GetVersionInfo returns formatted version information
func GetVersionInfo() string {
	return fmt.Sprintf("Simple Proxy v%s (commit: %s, built: %s)", Version, GitCommit, BuildTime)
}

// GetGitCommit gets the current git commit hash at runtime
func GetGitCommit() string {
	if GitCommit != "unknown" {
		return GitCommit // Use build-time value if available
	}
	
	// Fallback to runtime git command
	cmd := exec.Command("git", "rev-parse", "--short", "HEAD")
	output, err := cmd.Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(output))
}

// GetBuildInfo returns detailed build information
func GetBuildInfo() string {
	commit := GetGitCommit()
	buildTime := BuildTime
	if buildTime == "unknown" {
		buildTime = time.Now().Format("2006-01-02 15:04:05")
	}
	
	return fmt.Sprintf("Simple Proxy v%s\nCommit: %s\nBuild Time: %s\nHarmony Fix: v2.0 (Issue #8 resolved)", 
		Version, commit, buildTime)
}