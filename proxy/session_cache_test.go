package proxy

import (
	"claude-proxy/types"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestSessionCacheBasicFunctionality tests the basic SessionCache operations
func TestSessionCacheBasicFunctionality(t *testing.T) {
	cache := &SessionCache{
		cache: make(map[string]*CacheEntry),
	}

	// Test 1: Get new ContextManager for session
	sessionID := "test-session-1"
	cm1 := cache.GetContextManager(sessionID)
	assert.NotNil(t, cm1, "Should create new ContextManager for new session")

	// Test 2: Same session should return same ContextManager
	cm2 := cache.GetContextManager(sessionID)
	assert.Same(t, cm1, cm2, "Same session should return same ContextManager instance")

	// Test 3: Different session should return different ContextManager
	cm3 := cache.GetContextManager("test-session-2")
	assert.NotSame(t, cm1, cm3, "Different sessions should have different ContextManager instances")
	assert.NotNil(t, cm3, "Should create ContextManager for new session")
}

// TestSessionCacheCleanup tests the session cleanup functionality
func TestSessionCacheCleanup(t *testing.T) {
	cache := &SessionCache{
		cache: make(map[string]*CacheEntry),
	}

	// Create some sessions
	cm1 := cache.GetContextManager("session-1")
	cm2 := cache.GetContextManager("session-2")
	cm3 := cache.GetContextManager("session-3")

	assert.NotNil(t, cm1)
	assert.NotNil(t, cm2)
	assert.NotNil(t, cm3)

	// Simulate old sessions by manipulating timestamps
	cache.mutex.Lock()
	cache.cache["session-1"].LastAccessed = time.Now().Add(-2 * time.Hour) // Old
	cache.cache["session-2"].LastAccessed = time.Now().Add(-30 * time.Minute) // Recent  
	cache.cache["session-3"].LastAccessed = time.Now().Add(-3 * time.Hour) // Old
	cache.mutex.Unlock()

	// Cleanup sessions older than 1 hour
	cache.CleanupExpiredSessions(1 * time.Hour)

	// Verify cleanup results
	cache.mutex.RLock()
	_, exists1 := cache.cache["session-1"]
	_, exists2 := cache.cache["session-2"]
	_, exists3 := cache.cache["session-3"]
	cache.mutex.RUnlock()

	assert.False(t, exists1, "Old session-1 should be cleaned up")
	assert.True(t, exists2, "Recent session-2 should remain")
	assert.False(t, exists3, "Old session-3 should be cleaned up")
}

// TestSessionCacheConcurrency tests thread safety of SessionCache
func TestSessionCacheConcurrency(t *testing.T) {
	cache := &SessionCache{
		cache: make(map[string]*CacheEntry),
	}

	// Test concurrent access
	done := make(chan bool, 10)
	
	for i := 0; i < 10; i++ {
		go func(id int) {
			sessionID := "concurrent-session"
			cm := cache.GetContextManager(sessionID)
			assert.NotNil(t, cm)
			done <- true
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify only one ContextManager was created for the session
	cache.mutex.RLock()
	assert.Len(t, cache.cache, 1, "Should have exactly one session")
	cache.mutex.RUnlock()
}

// TestGenerateSessionID tests the session ID generation logic
func TestGenerateSessionID(t *testing.T) {
	// Test with messages - same content within same second should generate same ID
	req1 := &types.AnthropicRequest{
		Model: "claude-3-haiku",
		Messages: []types.Message{
			{Role: "user", Content: "Hello world"},
		},
	}

	req2 := &types.AnthropicRequest{
		Model: "claude-3-haiku", 
		Messages: []types.Message{
			{Role: "user", Content: "Hello world"}, // Same content
		},
	}

	req3 := &types.AnthropicRequest{
		Model: "claude-3-haiku",
		Messages: []types.Message{
			{Role: "user", Content: "Different message"},
		},
	}

	sessionID1 := generateSessionID(req1)
	sessionID2 := generateSessionID(req2)
	sessionID3 := generateSessionID(req3)

	// Same content within same second generates same hash
	assert.Equal(t, sessionID1, sessionID2, "Same content within same second should generate same session ID")
	assert.NotEqual(t, sessionID1, sessionID3, "Different content should generate different session ID")
	assert.Len(t, sessionID1, 64, "Session ID should be 64 characters long (SHA256 hash)")

	// Test with no messages - should have unique timestamp suffix
	req4 := &types.AnthropicRequest{
		Model: "claude-3-haiku",
		Messages: []types.Message{},
	}
	sessionID4 := generateSessionID(req4)
	assert.True(t, strings.HasPrefix(sessionID4, "default-"), "Empty messages should use default prefix with timestamp")
	
	// Test that empty message requests get different IDs due to nanosecond timestamp
	req5 := &types.AnthropicRequest{
		Model: "claude-3-haiku",
		Messages: []types.Message{},
	}
	sessionID5 := generateSessionID(req5)
	assert.NotEqual(t, sessionID4, sessionID5, "Empty message requests should generate unique IDs with nanosecond precision")
}

// TestSessionCacheIntegration tests integration with actual request processing
func TestSessionCacheIntegration(t *testing.T) {
	// Test that would verify integration with TransformAnthropicToOpenAI
	// This test documents the expected integration behavior
	
	req := &types.AnthropicRequest{
		Model: "claude-3-haiku",
		Messages: []types.Message{
			{Role: "user", Content: "First message"},
			{Role: "assistant", Content: []types.Content{
				{Type: "thinking", Text: "I need to process this"},
				{Type: "tool_use", ID: "call_1", Name: "TestTool"},
			}},
			{Role: "user", Content: "Follow-up message"},
		},
	}

	// Expected behavior:
	// 1. Generate session ID from request
	sessionID := generateSessionID(req)
	assert.NotEmpty(t, sessionID, "Should generate valid session ID")
	
	// 2. Get ContextManager for session
	cm := globalSessionCache.GetContextManager(sessionID)
	assert.NotNil(t, cm, "Should get ContextManager for session")
	
	// 3. Update with conversation history
	for _, msg := range req.Messages {
		cm.UpdateHistory(msg)
	}
	
	// 4. Verify analysis preservation logic
	assert.True(t, cm.ShouldPreserveAnalysis(), "Should preserve analysis after tool call")
	
	// 5. Second request with same session should get same ContextManager
	cm2 := globalSessionCache.GetContextManager(sessionID)
	assert.Same(t, cm, cm2, "Same session should return same ContextManager")
	assert.True(t, cm2.ShouldPreserveAnalysis(), "Preservation state should persist")
}