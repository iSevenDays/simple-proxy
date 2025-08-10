# Test Architecture Documentation

## Overview

The Simple Proxy test suite uses **real LLM endpoints** from the environment configuration for testing LLM-based validation logic. This provides realistic testing of the actual system behavior.

## Test Optimizations

### Circuit Breaker Optimization

Tests use an optimized circuit breaker configuration for faster failover:

```go
func getTestCircuitBreakerConfig() config.CircuitBreakerConfig {
    return config.CircuitBreakerConfig{
        FailureThreshold:   1,                       // Open circuit after 1 failure (not 2)
        BackoffDuration:    100 * time.Millisecond,  // Very short backoff (100ms)  
        MaxBackoffDuration: 1 * time.Second,         // Max 1s wait (not 30s)
    }
}
```

### Endpoint Health Optimization

Tests automatically reorder LLM endpoints to put healthy ones first:

```go
// OPTIMIZATION FOR TESTS: Reorder endpoints to put healthy ones first
cfg.ToolCorrectionEndpoints = reorderEndpointsByHealth(cfg.ToolCorrectionEndpoints)
```

This performs quick 2-second health checks and prioritizes working endpoints, reducing test timeouts from 30+ seconds to ~3-5 seconds.

### HTTP Timeout Optimization

Reduced HTTP client timeout from 30s to 10s for faster failover:

```go
client := &http.Client{
    Timeout: 10 * time.Second, // Reduced from 30s to 10s for faster failover
}
```

## Real LLM Integration

### Configuration

Tests use real LLM endpoints from `.env` configuration:

- **Endpoints**: `TOOL_CORRECTION_ENDPOINT` (comma-separated list)
- **API Key**: `TOOL_CORRECTION_API_KEY` 
- **Model**: `CORRECTION_MODEL`

Example `.env`:
```bash
TOOL_CORRECTION_ENDPOINT=http://192.168.0.46:11434/v1/chat/completions,http://192.168.0.50:11434/v1/chat/completions
TOOL_CORRECTION_API_KEY=ollama
CORRECTION_MODEL=qwen2.5-coder:latest
```

### Test Configuration Helper

```go
func NewMockConfigProvider() *config.Config {
    // Load config from environment variables (uses .env file)
    cfg, err := config.LoadConfigWithEnv()
    
    // Apply test optimizations
    cfg.ToolCorrectionEndpoints = reorderEndpointsByHealth(cfg.ToolCorrectionEndpoints)
    cfg.CircuitBreaker = getTestCircuitBreakerConfig()
    
    return cfg
}
```

## Performance Results

**Before Optimization**: 30+ seconds per test (waiting for timeouts)
**After Optimization**: 3-5 seconds per test (8.6x faster)

## Test Suite Statistics

- **Total Tests**: 137 test functions across 36 test files
- **Fast Unit Tests**: ~96 functions (circuit breaker, config, parsing, loop detection)
- **LLM Integration Tests**: ~41 functions (context analysis, tool correction, validation)
- **Estimated Runtime**: 15-20 minutes for full suite with real LLM endpoints
- **Core Tests Runtime**: 3-5 minutes (fast tests without LLM calls)

## LLM Behavior Notes

### ExitPlanMode Validation

The real LLM (qwen2.5-coder:latest) exhibits conservative behavior:

- **Blocks**: Clear completion summaries with "✅ **All tasks completed successfully**"
- **Blocks**: Empty plan content (considered inappropriate)
- **Allows**: Ambiguous completion language that could be valid planning
- **Allows**: Forward-looking planning language

### Test Expectations

Tests are updated to match real LLM behavior rather than using brittle pattern-based expectations. This ensures:

1. **Realistic validation**: Tests actual system behavior
2. **Robust testing**: No dependence on mock server complexity
3. **Conservative fallback**: When LLM is unavailable, system defaults to allowing usage

## Running Tests

### Test Categories and Timeouts

Due to real LLM endpoint dependencies and circuit breaker testing, tests require generous timeouts:

```bash
# Fast unit tests (96 tests, no LLM calls) - 5 min timeout
go test ./test/ -run "TestCircuitBreaker|TestConfig|TestSkipTools|TestDefault|TestHealthy|TestAllEndpoints|TestEnv|TestRetry|TestLoop|TestTimeout|TestModel|TestStream|TestTransform|TestTool|TestRule|TestSchema|TestEmpty|TestMultiEdit|TestWebsearch|TestTodowrite|TestNeedsCorrection|TestEnhanced" -timeout 300s -v

# Context-aware filtering tests (uses real LLM) - 2 min timeout
go test ./test/ -run "TestContextAwareToolFiltering" -timeout 120s -v

# ExitPlanMode validation tests (7 tests, uses real LLM) - 5 min timeout  
go test ./test/ -run "TestExitPlanModeValidation" -timeout 300s -v

# All ExitPlanMode tests (21 tests) - 10 min timeout
go test ./test/ -run "ExitPlanMode" -timeout 600s -v

# Tool correction tests (13 tests, uses real LLM) - 8 min timeout
go test ./test/ -run "TestCorrection.*Integration|TestCorrection.*Service|TestSimpleRealLLM" -timeout 480s -v
```

### Full Test Suite

**⚠️ Warning**: Full test suite can take 15-20 minutes due to real LLM endpoint testing (137 tests total).

```bash
# Full test suite (137 tests) - 20 min timeout
go test ./test/ -timeout 1200s

# Run with verbose output to monitor progress
go test ./test/ -timeout 1200s -v

# Run tests in parallel (faster but less predictable output)
go test ./test/ -timeout 1200s -parallel 4

# Recommended: Run test categories separately to avoid timeouts
# 1. Fast tests first (96 tests, 5 min)
go test ./test/ -run "TestCircuitBreaker|TestConfig|TestSkipTools|TestDefault|TestHealthy|TestAllEndpoints|TestEnv|TestRetry|TestLoop|TestTimeout|TestModel|TestStream|TestTransform|TestTool|TestRule|TestSchema|TestEmpty|TestMultiEdit|TestWebsearch|TestTodowrite|TestNeedsCorrection|TestEnhanced" -timeout 300s -v

# 2. Then LLM-dependent tests (41 tests, 15 min)
go test ./test/ -run "TestExitPlanMode|TestContextAware|TestCorrection|TestSimpleRealLLM" -timeout 900s -v
```

### Troubleshooting Timeouts

If tests are timing out:

1. **Check LLM endpoints**: Ensure endpoints in `.env` are responsive
2. **Use shorter test sets**: Run specific test categories instead of full suite
3. **Check network**: Circuit breaker tests depend on network connectivity
4. **Increase timeout**: Some LLM tests may need 10+ minutes on slow networks

```bash
# Check if LLM endpoint is responsive
curl -X POST http://192.168.0.46:11434/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{"model":"qwen2.5-coder:latest","messages":[{"role":"user","content":"test"}],"max_tokens":1}'

# Skip LLM-dependent tests entirely (not recommended for CI)
go test ./test/ -run "TestCircuitBreaker|TestConfig|TestSkipTools" -timeout 30s -v
```

## Circuit Breaker Behavior

Tests demonstrate the circuit breaker working correctly:

1. **First endpoint fails** → Circuit opens after 1 failure
2. **Automatic failover** → Tries next healthy endpoint  
3. **Success logging** → Records endpoint recovery
4. **Performance** → Fast failover prevents long waits

Example test output:
```
🎯 Test optimization: Reordered endpoints - healthy first: 1 working, 1 failing
🚨 Circuit breaker opened for endpoint http://192.168.0.46:11434/v1/chat/completions (failures: 1, retry in: 100ms)
✅ Tool correction succeeded on fallback endpoint (attempt 2)
```

## Architecture Benefits

1. **Real system testing**: Tests actual LLM behavior, not mocked responses
2. **Performance optimized**: Fast endpoint discovery and failover
3. **Resilient**: Circuit breaker handles endpoint failures gracefully  
4. **Maintainable**: No complex mock server logic to maintain
5. **Conservative**: Safe fallback behavior when LLM is unavailable

## Common Testing Issues & Solutions

### 1. Tool Schema Consistency

**❌ Problem**: Tests using inconsistent tool schemas
```go
// BAD: Manual schema definition
{Name: "Read", InputSchema: types.ToolSchema{Type: "object"}}
```

**✅ Solution**: Use standardized test helpers
```go
// GOOD: Centralized schema from types package
GetStandardTestTool("Read")  // Uses types.GetFallbackToolSchema()
GetStandardTestTools()       // Common tool set
```

**Benefits**: Consistent with production, catches unknown tools early (panics on undefined tools)

### 2. Circuit Breaker Initialization

**❌ Problem**: Nil pointer crashes from missing HealthManager
```go
// BAD: Manual config creation
cfg := &config.Config{...}  // Missing HealthManager
```

**✅ Solution**: Use proper initialization
```go
// GOOD: Proper initialization
cfg := config.GetDefaultConfig()  // Always includes HealthManager
// OR
cfg, _ := config.LoadConfigWithEnv()  // Production initialization
```

### 3. Graceful Fallback Testing

**❌ Problem**: Tests expecting errors when system now has graceful fallbacks
```go
// BAD: Old test expectations
if err == nil {
    t.Error("Expected error when endpoints fail")
}
```

**✅ Solution**: Test graceful fallback behavior  
```go
// GOOD: Test fallback behavior
if err != nil {
    t.Errorf("Expected graceful fallback, got: %v", err)
}
if result != false {  // Should default to tool_choice=optional
    t.Errorf("Expected false fallback, got: %v", result)
}
```

### 4. Test Configuration Best Practices

**Required for LLM tests**:
```bash
# .env file must contain real endpoints
TOOL_CORRECTION_ENDPOINT=http://192.168.0.46:11434/v1/chat/completions
TOOL_CORRECTION_API_KEY=ollama  
CORRECTION_MODEL=qwen2.5-coder:latest
```

**Use test helpers**:
```go
cfg := NewMockConfigProvider()  // Includes endpoint reordering + circuit breaker config
```

### 5. Avoiding Test Flakiness

**Timeouts**: Use appropriate timeouts for LLM tests
```go
// Fast unit tests: 5min, LLM tests: 10-15min
go test -timeout 900s -run "TestExitPlanMode"
```

**Circuit breaker**: Tests use optimized settings (1 failure = open, 100ms backoff)

**Endpoint health**: Tests automatically reorder endpoints (healthy first)