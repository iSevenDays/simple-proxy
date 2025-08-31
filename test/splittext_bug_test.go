package test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// splitTextForStreamingBuggy reproduces the buggy implementation to demonstrate the issue
func splitTextForStreamingBuggy(text string) []string {
	if text == "" {
		return []string{}
	}

	var chunks []string
	runes := []rune(text)
	chunkSize := 150 // Target ~150 characters per chunk for better streaming experience
	
	for i := 0; i < len(runes); i += chunkSize {
		end := i + chunkSize
		if end > len(runes) {
			end = len(runes)
		}

		// Try to break at word boundaries to avoid splitting words
		if end < len(runes) {
			// Look back for the last space or newline within a reasonable range
			for j := end; j > i+chunkSize/2 && j > i; j-- {
				if runes[j] == ' ' || runes[j] == '\n' || runes[j] == '\t' {
					end = j + 1 // Include the whitespace character
					break
				}
			}
		}

		chunk := string(runes[i:end])
		chunks = append(chunks, chunk)
		
		// BUGGY LOGIC: This causes character skipping
		i = end - 1 // -1 because loop will increment
	}

	return chunks
}

// splitTextForStreamingFixed shows the corrected implementation
func splitTextForStreamingFixed(text string) []string {
	if text == "" {
		return []string{}
	}

	var chunks []string
	runes := []rune(text)
	chunkSize := 150
	
	for i := 0; i < len(runes); {
		end := i + chunkSize
		if end > len(runes) {
			end = len(runes)
		}

		// Try to break at word boundaries to avoid splitting words
		if end < len(runes) {
			// Look back for the last space or newline within a reasonable range
			for j := end; j > i+chunkSize/2 && j > i; j-- {
				if runes[j] == ' ' || runes[j] == '\n' || runes[j] == '\t' {
					end = j + 1 // Include the whitespace character
					break
				}
			}
		}

		chunk := string(runes[i:end])
		chunks = append(chunks, chunk)
		
		// FIXED: Proper positioning for next iteration
		i = end
	}

	return chunks
}

// TestSplitTextBugReproduction demonstrates the character loss bug
func TestSplitTextBugReproduction(t *testing.T) {
	// Use the exact content that was truncated in the Loki logs
	content := `✅ PRD created: .claude/prds/installation-outcome-idea-b.md

The PRD defines a refactoring to use an InstallationOutcome record that captures the final result of an iOS app installation. This ensures metrics like app-installation.ios are reported exactly once per request, giving developers accurate visibility into installation success/failure rates, independent of internal fallback attempts.

Ready to create implementation epic? Run: /pm:prd-parse installation-outcome-idea-b`

	t.Logf("Original content length: %d", len(content))
	
	// Test buggy implementation
	buggyChunks := splitTextForStreamingBuggy(content)
	buggyReconstructed := strings.Join(buggyChunks, "")
	
	t.Logf("Buggy chunks count: %d", len(buggyChunks))
	t.Logf("Buggy reconstructed length: %d", len(buggyReconstructed))
	
	// Test fixed implementation  
	fixedChunks := splitTextForStreamingFixed(content)
	fixedReconstructed := strings.Join(fixedChunks, "")
	
	t.Logf("Fixed chunks count: %d", len(fixedChunks))
	t.Logf("Fixed reconstructed length: %d", len(fixedReconstructed))

	// Log chunks for analysis
	t.Logf("\n=== BUGGY CHUNKS ===")
	for i, chunk := range buggyChunks {
		t.Logf("Chunk %d (%d chars): %q", i, len(chunk), chunk)
	}
	
	t.Logf("\n=== FIXED CHUNKS ===")
	for i, chunk := range fixedChunks {
		t.Logf("Chunk %d (%d chars): %q", i, len(chunk), chunk)
	}
	
	// Verify the bug causes character loss
	if len(buggyReconstructed) < len(content) {
		t.Logf("🐛 BUG CONFIRMED: Buggy implementation loses %d characters", len(content) - len(buggyReconstructed))
		t.Logf("Missing content: %q", content[len(buggyReconstructed):])
		
		// Check if the command is lost
		if !strings.Contains(buggyReconstructed, "/pm:prd-parse installation-outcome-idea-b") {
			t.Logf("❌ CRITICAL: Command '/pm:prd-parse installation-outcome-idea-b' is lost in buggy implementation")
		}
	}
	
	// Verify the fix preserves all content
	assert.Equal(t, content, fixedReconstructed, "Fixed implementation should preserve all content")
	assert.Contains(t, fixedReconstructed, "/pm:prd-parse installation-outcome-idea-b", "Command should be preserved")
	assert.True(t, strings.HasSuffix(fixedReconstructed, "/pm:prd-parse installation-outcome-idea-b"), "Response should end with complete command")
	
	// Demonstrate the exact character loss
	if buggyReconstructed != content {
		t.Logf("\n=== CHARACTER LOSS ANALYSIS ===")
		t.Logf("Original:      %q", content)
		t.Logf("Buggy result:  %q", buggyReconstructed) 
		t.Logf("Expected loss: Characters around chunk boundaries with spaces")
	}
}

// TestChunkBoundaryEdgeCases tests edge cases that trigger the bug
func TestChunkBoundaryEdgeCases(t *testing.T) {
	testCases := []struct {
		name     string
		content  string
		expected string
	}{
		{
			name: "space_at_chunk_boundary",
			content: strings.Repeat("a", 149) + " command",
			expected: strings.Repeat("a", 149) + " command",
		},
		{
			name: "newline_at_chunk_boundary", 
			content: strings.Repeat("b", 149) + "\ncommand",
			expected: strings.Repeat("b", 149) + "\ncommand",
		},
		{
			name: "multiple_spaces_at_boundary",
			content: strings.Repeat("c", 148) + "  command",
			expected: strings.Repeat("c", 148) + "  command",
		},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			buggyResult := strings.Join(splitTextForStreamingBuggy(tc.content), "")
			fixedResult := strings.Join(splitTextForStreamingFixed(tc.content), "")
			
			t.Logf("Original: %q", tc.content)
			t.Logf("Buggy:    %q", buggyResult)
			t.Logf("Fixed:    %q", fixedResult)
			
			// Fixed version should preserve content
			assert.Equal(t, tc.expected, fixedResult, "Fixed implementation should preserve content")
			
			// Document if buggy version loses content
			if buggyResult != tc.content {
				t.Logf("🐛 Buggy implementation loses content in %s", tc.name)
			}
		})
	}
}