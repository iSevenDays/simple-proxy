# Simple Proxy Test Suite

This test suite follows CLAUDE.md SPARC principles: **Simplicity, Iterate, Focus, Quality, Collaboration**.

## Test Structure

### Unit Tests
Following **Simplicity** and **Focus** principles:

- `config_test.go` - Configuration loading and validation
- `transform_test.go` - Request/response format transformations  
- `stream_test.go` - Streaming response processing
- `correction_test.go` - Tool call validation and correction logic

### Integration Tests
Following **Quality** principle with comprehensive end-to-end testing:

- `integration_test.go` - Complete proxy workflow testing

## Test Categories

### 1. Configuration Tests (`config_test.go`)
- ✅ Valid CCR config loading
- ✅ Missing provider handling
- ✅ Invalid JSON handling
- ✅ Default configuration fallback

### 2. Transformation Tests (`transform_test.go`)  
- ✅ Anthropic → OpenAI request conversion
- ✅ OpenAI → Anthropic response conversion
- ✅ System message handling
- ✅ Tool definition transformation
- ✅ Tool call conversion (both directions)

### 3. Streaming Tests (`stream_test.go`)
- ✅ Chunk-by-chunk processing
- ✅ Response reconstruction from chunks
- ✅ Tool call streaming assembly
- ✅ Mixed content and tool streaming
- ✅ Finish reason detection (`finish_reason != null`)

### 4. Tool Correction Tests (`correction_test.go`)
- ✅ Tool call schema validation
- ✅ Parameter name validation (`filename` vs `file_path`)
- ✅ Required parameter checking
- ✅ Unknown tool detection
- ✅ Correction prompt generation

### 5. Integration Tests (`integration_test.go`)
- ✅ End-to-end proxy workflow
- ✅ Mock backend integration
- ✅ Tool calling workflow
- ✅ Error handling scenarios
- ✅ HTTP method validation

## Key Test Patterns

### Following SPARC Principles

**Simplicity:**
- Clear test names that describe exact behavior
- Single responsibility per test function
- Minimal setup and teardown

**Iterate:**
- Tests build on each other (unit → integration)
- Easy to extend with new scenarios
- Refactoring-friendly structure

**Focus:**
- Each test targets specific functionality
- No test overlap or redundancy
- Clear assertion messages

**Quality:**
- Comprehensive edge case coverage
- Error scenario testing
- Type safety validation

**Collaboration:**
- Well-documented test intentions
- Easy to understand for team members
- Clear failure messages for debugging

## Running Tests

```bash
# Run all tests
go test ./test

# Run with verbose output
go test -v ./test

# Run specific test file
go test -v ./test -run TestConfigLoading

# Run with coverage
go test -cover ./test
```

## Test Dependencies

- `github.com/stretchr/testify` - Assertions and test utilities
- Standard Go testing package
- No external mocking frameworks (following Simplicity principle)

## Test Coverage Goals

- ✅ **Configuration**: 100% coverage of config loading scenarios
- ✅ **Transformations**: Complete format conversion testing
- ✅ **Streaming**: Full chunk processing validation  
- ✅ **Tool Correction**: Schema validation and correction logic
- ✅ **Integration**: End-to-end workflow verification
- ✅ **Error Handling**: Comprehensive error scenario testing

This test suite ensures the Go proxy implementation is robust, reliable, and maintains the high quality standards outlined in CLAUDE.md.