package test

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// ChunkBuffer represents a buffer for streaming chunks
type ChunkBuffer struct {
	chunks     []StreamChunk
	timer      *time.Timer
	mu         sync.Mutex
	flushFunc  func([]StreamChunk)
	bufferTime time.Duration
}

// StreamChunk represents a streaming chunk with type information
type StreamChunk struct {
	Type    string      // "text", "tool_use", etc.
	Content string      // actual content
	Data    interface{} // additional data for tool_use chunks
}

// NewChunkBuffer creates a new chunk buffer with specified buffer time
func NewChunkBuffer(bufferTime time.Duration, flushFunc func([]StreamChunk)) *ChunkBuffer {
	return &ChunkBuffer{
		chunks:     make([]StreamChunk, 0),
		bufferTime: bufferTime,
		flushFunc:  flushFunc,
	}
}

// AddChunk adds a chunk to the buffer and manages timing
func (cb *ChunkBuffer) AddChunk(chunk StreamChunk) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.chunks = append(cb.chunks, chunk)

	// Reset timer
	if cb.timer != nil {
		cb.timer.Stop()
	}

	cb.timer = time.AfterFunc(cb.bufferTime, func() {
		cb.flush()
	})
}

// flush sends buffered chunks and clears the buffer
func (cb *ChunkBuffer) flush() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if len(cb.chunks) == 0 {
		return
	}

	// Merge consecutive text chunks
	mergedChunks := cb.mergeConsecutiveTextChunks()

	// Send merged chunks
	if cb.flushFunc != nil {
		cb.flushFunc(mergedChunks)
	}

	// Clear buffer
	cb.chunks = cb.chunks[:0]
	if cb.timer != nil {
		cb.timer.Stop()
		cb.timer = nil
	}
}

// ForceFlush immediately flushes the buffer (for cleanup)
func (cb *ChunkBuffer) ForceFlush() {
	if cb.timer != nil {
		cb.timer.Stop()
	}
	cb.flush()
}

// mergeConsecutiveTextChunks merges consecutive text chunks into single chunks
func (cb *ChunkBuffer) mergeConsecutiveTextChunks() []StreamChunk {
	if len(cb.chunks) == 0 {
		return []StreamChunk{}
	}

	merged := make([]StreamChunk, 0, len(cb.chunks))
	
	for i := 0; i < len(cb.chunks); i++ {
		current := cb.chunks[i]
		
		// Start a new merged chunk
		if current.Type == "text" {
			// Look ahead and merge consecutive text chunks
			mergedContent := current.Content
			
			for j := i + 1; j < len(cb.chunks) && cb.chunks[j].Type == "text"; j++ {
				mergedContent += cb.chunks[j].Content
				i = j // Skip merged chunks
			}
			
			merged = append(merged, StreamChunk{
				Type:    "text",
				Content: mergedContent,
			})
		} else {
			// Non-text chunks are not merged
			merged = append(merged, current)
		}
	}
	
	return merged
}

// TestChunkBufferBasicFunctionality tests basic buffer operations
func TestChunkBufferBasicFunctionality(t *testing.T) {
	var receivedChunks []StreamChunk
	flushFunc := func(chunks []StreamChunk) {
		receivedChunks = append(receivedChunks, chunks...)
	}

	buffer := NewChunkBuffer(100*time.Millisecond, flushFunc)

	// Add some text chunks
	buffer.AddChunk(StreamChunk{Type: "text", Content: "Hello "})
	buffer.AddChunk(StreamChunk{Type: "text", Content: "world!"})

	// Wait for buffer to flush
	time.Sleep(150 * time.Millisecond)

	assert.Len(t, receivedChunks, 1, "Should merge consecutive text chunks")
	assert.Equal(t, "Hello world!", receivedChunks[0].Content, "Should merge text content correctly")
	assert.Equal(t, "text", receivedChunks[0].Type, "Should preserve chunk type")
}

// TestChunkBufferMixedTypes tests handling of mixed chunk types
func TestChunkBufferMixedTypes(t *testing.T) {
	var receivedChunks []StreamChunk
	flushFunc := func(chunks []StreamChunk) {
		receivedChunks = append(receivedChunks, chunks...)
	}

	buffer := NewChunkBuffer(100*time.Millisecond, flushFunc)

	// Add mixed chunk types
	buffer.AddChunk(StreamChunk{Type: "text", Content: "Before tool: "})
	buffer.AddChunk(StreamChunk{Type: "tool_use", Content: "read_file", Data: map[string]string{"file": "test.txt"}})
	buffer.AddChunk(StreamChunk{Type: "text", Content: " after tool"})

	// Wait for buffer to flush
	time.Sleep(150 * time.Millisecond)

	assert.Len(t, receivedChunks, 3, "Should preserve different chunk types separately")
	assert.Equal(t, "Before tool: ", receivedChunks[0].Content)
	assert.Equal(t, "read_file", receivedChunks[1].Content)
	assert.Equal(t, " after tool", receivedChunks[2].Content)
}

// TestCommandPreservationWithBuffering tests that commands are preserved
func TestCommandPreservationWithBuffering(t *testing.T) {
	// Simulate the exact problematic scenario
	originalChunks := []StreamChunk{
		{Type: "text", Content: "⏺ ❌ No analysis found for task #refactor-installappviagoios-to-return-installationoutcome.Run: /pm:task-analyze "},
		{Type: "text", Content: "refactor-installappviagoios-to-return-installationoutcome firstOr: /pm:task-start refactor-installappviagoios-to-return-installationoutcome --analyze "},
		{Type: "text", Content: "to do both."},
	}

	var receivedChunks []StreamChunk
	flushFunc := func(chunks []StreamChunk) {
		receivedChunks = append(receivedChunks, chunks...)
	}

	buffer := NewChunkBuffer(500*time.Millisecond, flushFunc)

	// Add chunks as they would come from streaming
	for _, chunk := range originalChunks {
		buffer.AddChunk(chunk)
		// Simulate small delays between chunks
		time.Sleep(10 * time.Millisecond)
	}

	// Wait for buffer to flush
	time.Sleep(600 * time.Millisecond)

	// Should merge all text chunks into one
	assert.Len(t, receivedChunks, 1, "Should merge all consecutive text chunks")
	
	mergedContent := receivedChunks[0].Content
	t.Logf("Merged content: %q", mergedContent)

	// Verify commands are preserved
	commands := []string{
		"/pm:task-analyze refactor-installappviagoios-to-return-installationoutcome",
		"/pm:task-start refactor-installappviagoios-to-return-installationoutcome",
	}

	for _, cmd := range commands {
		assert.Contains(t, mergedContent, cmd, "Command should be preserved intact: %s", cmd)
	}

	// Verify complete content is preserved
	expectedContent := "⏺ ❌ No analysis found for task #refactor-installappviagoios-to-return-installationoutcome.Run: /pm:task-analyze refactor-installappviagoios-to-return-installationoutcome firstOr: /pm:task-start refactor-installappviagoios-to-return-installationoutcome --analyze to do both."
	assert.Equal(t, expectedContent, mergedContent, "Complete content should be preserved")
}

// TestBufferTimingBehavior tests timer reset behavior
func TestBufferTimingBehavior(t *testing.T) {
	var flushCount int
	var receivedChunks []StreamChunk
	
	flushFunc := func(chunks []StreamChunk) {
		flushCount++
		receivedChunks = append(receivedChunks, chunks...)
	}

	buffer := NewChunkBuffer(200*time.Millisecond, flushFunc)

	// Add chunks with delays less than buffer time
	buffer.AddChunk(StreamChunk{Type: "text", Content: "chunk1 "})
	time.Sleep(50 * time.Millisecond)
	
	buffer.AddChunk(StreamChunk{Type: "text", Content: "chunk2 "})
	time.Sleep(50 * time.Millisecond)
	
	buffer.AddChunk(StreamChunk{Type: "text", Content: "chunk3"})

	// Wait for buffer to flush
	time.Sleep(300 * time.Millisecond)

	assert.Equal(t, 1, flushCount, "Should flush only once after timer expires")
	assert.Len(t, receivedChunks, 1, "Should merge all chunks")
	assert.Equal(t, "chunk1 chunk2 chunk3", receivedChunks[0].Content, "Should merge content correctly")
}

// TestForceFlushCleanup tests force flush functionality
func TestForceFlushCleanup(t *testing.T) {
	var receivedChunks []StreamChunk
	flushFunc := func(chunks []StreamChunk) {
		receivedChunks = append(receivedChunks, chunks...)
	}

	buffer := NewChunkBuffer(1*time.Second, flushFunc) // Long buffer time

	// Add chunks
	buffer.AddChunk(StreamChunk{Type: "text", Content: "test content"})

	// Force flush before timer expires
	buffer.ForceFlush()

	assert.Len(t, receivedChunks, 1, "Should flush immediately on force flush")
	assert.Equal(t, "test content", receivedChunks[0].Content, "Should preserve content")
}

// BenchmarkChunkBufferPerformance benchmarks buffer performance
func BenchmarkChunkBufferPerformance(b *testing.B) {
	flushFunc := func(chunks []StreamChunk) {
		// No-op for benchmark
	}

	buffer := NewChunkBuffer(100*time.Millisecond, flushFunc)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buffer.AddChunk(StreamChunk{
			Type:    "text",
			Content: "benchmark chunk content",
		})
	}
	
	buffer.ForceFlush() // Clean up
}

// TestLargeContentHandling tests handling of large content
func TestLargeContentHandling(t *testing.T) {
	var receivedChunks []StreamChunk
	flushFunc := func(chunks []StreamChunk) {
		receivedChunks = append(receivedChunks, chunks...)
	}

	buffer := NewChunkBuffer(100*time.Millisecond, flushFunc)

	// Add large chunks
	largeContent1 := strings.Repeat("A", 1000)
	largeContent2 := strings.Repeat("B", 1000)

	buffer.AddChunk(StreamChunk{Type: "text", Content: largeContent1})
	buffer.AddChunk(StreamChunk{Type: "text", Content: largeContent2})

	// Wait for flush
	time.Sleep(150 * time.Millisecond)

	assert.Len(t, receivedChunks, 1, "Should merge large chunks")
	assert.Equal(t, 2000, len(receivedChunks[0].Content), "Should preserve all content")
	assert.Equal(t, largeContent1+largeContent2, receivedChunks[0].Content, "Should merge correctly")
}