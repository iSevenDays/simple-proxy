package test

import (
	"strings" 
	"testing"
)

// simulateSplitTextForStreaming reproduces the current splitTextForStreaming logic
func simulateSplitTextForStreaming(text string) []string {
	if text == "" {
		return []string{}
	}

	var chunks []string
	runes := []rune(text)
	chunkSize := 150 // Same as production
	
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
		
		// Move to next position
		i = end
	}

	return chunks
}

// TestStreamingChunkAnalysis analyzes exactly how content gets chunked
func TestStreamingChunkAnalysis(t *testing.T) {
	// Test the exact content that's showing truncation behavior
	content := `⏺ ❌ No analysis found for task #refactor-installappviagoios-to-return-installationoutcome.Run: /pm:task-analyze refactor-installappviagoios-to-return-installationoutcome firstOr: /pm:task-start refactor-installappviagoios-to-return-installationoutcome --analyze to do both.`

	t.Logf("Analyzing content: %q", content)
	t.Logf("Content length: %d characters", len(content))

	chunks := simulateSplitTextForStreaming(content)
	
	t.Logf("Content split into %d chunks:", len(chunks))
	
	totalReconstructed := ""
	for i, chunk := range chunks {
		t.Logf("Chunk %d (%d chars): %q", i, len(chunk), chunk)
		totalReconstructed += chunk
	}
	
	t.Logf("Reconstructed length: %d", len(totalReconstructed))
	t.Logf("Reconstructed content: %q", totalReconstructed)
	
	// Check if reconstruction matches original
	if totalReconstructed != content {
		t.Logf("❌ MISMATCH DETECTED!")
		t.Logf("Original:     %q", content) 
		t.Logf("Reconstructed: %q", totalReconstructed)
		
		// Find where they differ
		minLen := len(content)
		if len(totalReconstructed) < minLen {
			minLen = len(totalReconstructed)
		}
		
		for i := 0; i < minLen; i++ {
			if content[i] != totalReconstructed[i] {
				t.Logf("First difference at position %d: original='%c' reconstructed='%c'", i, content[i], totalReconstructed[i])
				break
			}
		}
	} else {
		t.Logf("✅ Reconstruction matches original perfectly")
	}

	// Analyze where commands appear in chunks
	commands := []string{
		"/pm:task-analyze refactor-installappviagoios-to-return-installationoutcome",
		"/pm:task-start refactor-installappviagoios-to-return-installationoutcome",
	}

	for _, cmd := range commands {
		t.Logf("\nAnalyzing command: %q", cmd)
		
		// Check if command appears intact in any single chunk
		foundIntact := false
		for i, chunk := range chunks {
			if strings.Contains(chunk, cmd) {
				t.Logf("✅ Command found intact in chunk %d", i)
				foundIntact = true
				break
			}
		}
		
		if !foundIntact {
			t.Logf("❌ Command NOT found intact in any single chunk")
			
			// Look for fragments across chunks
			cmdRunes := []rune(cmd)
			t.Logf("Command length: %d characters", len(cmdRunes))
			
			// Find where command starts
			for i, chunk := range chunks {
				chunkStr := string(chunk)
				if idx := strings.Index(chunkStr, "/pm:"); idx >= 0 {
					t.Logf("Command starts in chunk %d at position %d", i, idx)
					
					// Check how much of command is in this chunk
					cmdStart := strings.Index(chunkStr, "/pm:")
					remainingInChunk := chunkStr[cmdStart:]
					t.Logf("Command fragment in chunk %d: %q", i, remainingInChunk)
					
					// Check if command continues in next chunk
					if i+1 < len(chunks) {
						nextChunk := chunks[i+1]
						t.Logf("Next chunk %d: %q", i+1, nextChunk)
						
						// See if combining gives us the full command
						combined := remainingInChunk + nextChunk
						if strings.Contains(combined, cmd) {
							t.Logf("✅ Full command found when combining chunks %d and %d", i, i+1)
						} else {
							t.Logf("❌ Full command NOT found even when combining adjacent chunks")
						}
					}
				}
			}
		}
	}
}

// TestSpecificTruncationScenario tests the exact UI behavior reported
func TestSpecificTruncationScenario(t *testing.T) {
	// This reproduces the exact behavior seen in Claude Code UI:
	// Original: "/pm:task-analyze refactor-installappviagoios-to-return-installationoutcome"
	// Displayed: "/pm:task-analyze refactor-installappviagoios-to-return-installation"
	// Next line: "outcome firstOr:"
	
	content := `Run: /pm:task-analyze refactor-installappviagoios-to-return-installationoutcome firstOr: /pm:task-start refactor-installappviagoios-to-return-installationoutcome --analyze`
	
	chunks := simulateSplitTextForStreaming(content)
	
	t.Logf("Testing specific truncation scenario:")
	t.Logf("Content: %q", content)
	t.Logf("Split into %d chunks:", len(chunks))
	
	for i, chunk := range chunks {
		t.Logf("Chunk %d: %q", i, chunk)
	}
	
	// Look for the specific pattern that would cause the UI display issue
	for i, chunk := range chunks {
		if strings.Contains(chunk, "refactor-installappviagoios-to-return-installation") && 
		   !strings.Contains(chunk, "installationoutcome") {
			t.Logf("❌ Found problematic split in chunk %d: command truncated", i)
			t.Logf("   Chunk content: %q", chunk)
			
			// Check next chunk for the missing part
			if i+1 < len(chunks) {
				nextChunk := chunks[i+1] 
				if strings.HasPrefix(nextChunk, "outcome") {
					t.Logf("❌ CONFIRMED: 'outcome' found at start of next chunk %d", i+1)
					t.Logf("   Next chunk: %q", nextChunk)
					t.Logf("   This would cause the UI display issue!")
				}
			}
		}
	}
	
	// Test reconstruction
	reconstructed := strings.Join(chunks, "")
	if reconstructed != content {
		t.Logf("❌ Reconstruction failed!")
	} else {
		t.Logf("✅ Reconstruction successful, so issue must be in streaming delivery")
	}
}

// TestCommandSplittingEdgeCases tests various edge cases that could cause splitting
func TestCommandSplittingEdgeCases(t *testing.T) {
	testCases := []struct{
		name string
		content string
		expectSplit bool
		description string
	}{
		{
			name: "command_at_exact_150_boundary",
			content: strings.Repeat("a", 150-len("/pm:task-analyze")) + "/pm:task-analyze refactor-installappviagoios-to-return-installationoutcome",
			expectSplit: true,
			description: "Command starting near chunk boundary should split",
		},
		{
			name: "long_command_no_spaces_in_lookback",
			content: strings.Repeat("b", 100) + "/pm:task-analyze-refactor-installappviagoios-to-return-installationoutcome-with-no-spaces-in-lookback-range", 
			expectSplit: true,
			description: "Long command without spaces in lookback range will split",
		},
		{
			name: "command_with_spaces_should_not_split",
			content: "Execute this command: /pm:task analyze refactor installappviagoios to return installationoutcome",
			expectSplit: false,
			description: "Command with spaces should break at space boundaries",
		},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			chunks := simulateSplitTextForStreaming(tc.content)
			reconstructed := strings.Join(chunks, "")
			
			// Basic reconstruction check
			if reconstructed != tc.content {
				t.Logf("❌ BASIC ISSUE: Reconstruction failed")
				t.Logf("Original:      %q", tc.content)
				t.Logf("Reconstructed: %q", reconstructed)
			}
			
			// Look for command splitting
			commandSplit := false
			for _, chunk := range chunks {
				// Check if chunk contains partial command patterns
				if strings.Contains(chunk, "/pm:") && 
				   (strings.HasSuffix(chunk, "installation") || 
				    strings.HasSuffix(chunk, "outcome") ||
				    strings.HasSuffix(chunk, "refactor")) {
					commandSplit = true
					t.Logf("Command appears to be split in chunk: %q", chunk)
				}
			}
			
			if commandSplit && !tc.expectSplit {
				t.Logf("❌ Unexpected command splitting in %s", tc.description)
			} else if !commandSplit && tc.expectSplit {
				t.Logf("✅ Command not split as expected in %s", tc.description)  
			} else {
				t.Logf("✅ Command splitting behavior matches expectation for %s", tc.description)
			}
		})
	}
}