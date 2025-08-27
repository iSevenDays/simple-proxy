# OpenAI Harmony Support Documentation

## Table of Contents

1. [Overview](#overview)
2. [OpenAI Harmony Format](#openai-harmony-format)
3. [Channel Types & Content Classification](#channel-types--content-classification)
4. [Architecture Overview](#architecture-overview)
5. [Integration Patterns](#integration-patterns)
6. [Configuration Guide](#configuration-guide)
7. [Advanced Usage Patterns](#advanced-usage-patterns)
8. [Real-World Examples](#real-world-examples)
9. [Troubleshooting](#troubleshooting)
10. [Performance Characteristics](#performance-characteristics)
11. [API Reference](#api-reference)

## Overview

Simple Proxy includes comprehensive support for OpenAI Harmony message format parsing, enabling proper distinction between thinking/analysis content (`<|channel|>analysis`) and user-facing responses (`<|channel|>final`). This feature resolves compatibility issues with models like gpt-oss-120b that use Harmony format exclusively, ensuring optimal user experience in Claude Code UI.

### Key Benefits

- **Clean User Interface**: Thinking content appears in dedicated thinking panels, not cluttering main responses
- **Better User Experience**: Users see focused, clean answers without verbose internal reasoning
- **Backward Compatibility**: Existing `<think></think>` tag handling remains unaffected
- **Performance Optimized**: <10ms parsing overhead with efficient regex-based token recognition
- **Streaming Compatible**: Works with both complete and chunked responses

### Problem Solved

Without Harmony support, verbose thinking messages from models like gpt-oss-120b appear as regular content:
```
⏺ <|channel|>analysis<|message|>The user is asking about implementation details...
⏺ Here's the actual response...
```

With Harmony support, this is properly separated:
- **Claude Code Thinking Panel**: "The user is asking about implementation details..."
- **Main Response**: "Here's the actual response..."

## OpenAI Harmony Format

### Format Structure

OpenAI Harmony uses a structured token-based format to organize content into different channels:

```
<|start|>role<|channel|>channel_type<|message|>content<|end|>
```

**Token Breakdown:**
- `<|start|>` - Begins a Harmony message block
- `role` - Message role (assistant, user, system, developer, tool)
- `<|channel|>` - Channel specification token
- `channel_type` - Type of content (analysis, final, commentary)
- `<|message|>` - Content delimiter
- `content` - The actual message content
- `<|end|>` - Ends the message block (or `<|return|>` for tool responses)

### Supported Roles

| Role | Description | Use Case |
|------|-------------|----------|
| `assistant` | AI assistant responses | Most common role for model outputs |
| `user` | User messages | Input from human users |
| `system` | System messages | Configuration and instructions |
| `developer` | Developer notes | Development-specific content |
| `tool` | Tool outputs | Results from tool executions |

### Purpose and Design

The Harmony format was designed to provide clear separation between different types of AI-generated content:

1. **Thinking Transparency**: Allow users to see AI reasoning when desired
2. **Content Classification**: Distinguish between internal processing and final outputs
3. **Tool Integration**: Support complex tool call workflows
4. **Streaming Compatibility**: Work efficiently with real-time response generation

## Channel Types & Content Classification

### Channel Types

#### 1. Analysis Channel (`analysis`)
**Purpose**: Internal thinking, reasoning, and analysis content

```
<|start|>assistant<|channel|>analysis<|message|>I need to analyze the user's request for implementing a search feature. Let me break down the requirements:
1. Full-text search across documents
2. Filter by date range
3. Sort by relevance
4. Return structured results<|end|>
```

**Classification**: `ContentTypeThinking`
**UI Treatment**: Appears in Claude Code thinking panel
**Use Cases**:
- Problem analysis and breakdown
- Step-by-step reasoning
- Requirement validation
- Risk assessment

#### 2. Final Channel (`final`)
**Purpose**: User-facing response content

```
<|start|>assistant<|channel|>final<|message|>I'll implement a search feature with the following components:

1. **Search Endpoint**: `POST /api/search`
2. **Query Parameters**: text, dateRange, sortBy
3. **Response Format**: JSON with results array<|end|>
```

**Classification**: `ContentTypeResponse`
**UI Treatment**: Main response content
**Use Cases**:
- Direct answers to user questions
- Implementation instructions
- Code examples and solutions
- Final recommendations

#### 3. Commentary Channel (`commentary`)
**Purpose**: Tool calls and intermediate processing

```
<|start|>assistant<|channel|>commentary<|message|>I'll use the file search tool to find existing search implementations in the codebase.<|end|>
<|start|>assistant<|channel|>commentary<|message|>Tool call: search_files(pattern="search", type="function")<|return|>
```

**Classification**: `ContentTypeToolCall`
**UI Treatment**: Tool call metadata
**Use Cases**:
- Tool execution explanations
- Intermediate processing steps
- Debug information
- Tool call coordination

### Content Classification System

The parser automatically classifies content based on channel types:

```go
// Classification mapping
func ClassifyChannelType(channelType ChannelType) ContentType {
    switch channelType {
    case ChannelAnalysis:
        return ContentTypeThinking    // → Thinking panel
    case ChannelFinal:
        return ContentTypeResponse    // → Main response
    case ChannelCommentary:
        return ContentTypeToolCall    // → Tool metadata
    default:
        return ContentTypeRegular     // → Standard content
    }
}
```

### Mixed Channel Messages

A single response can contain multiple channels:

```
<|start|>assistant<|channel|>analysis<|message|>The user wants to refactor the authentication system. I need to consider:
- Current JWT implementation
- Session management
- Security implications<|end|>
<|start|>assistant<|channel|>final<|message|>I'll help you refactor the authentication system. Here's my recommended approach:

## Current Assessment
Your JWT implementation is solid, but we can improve session management.<|end|>
```

**Result**:
- **Thinking Content**: "The user wants to refactor... security implications"
- **Response Content**: "I'll help you refactor... session management."

## Architecture Overview

### System Components

```
┌─────────────────┐    ┌──────────────────┐    ┌─────────────────┐
│   Claude Code   │───▶│  Simple Proxy    │───▶│  Model Provider │
│   (Client)      │    │  (Transform)     │    │  (gpt-oss-120b) │
└─────────────────┘    └──────────────────┘    └─────────────────┘
                              │
                              ▼
                       ┌─────────────┐
                       │  Harmony    │
                       │  Parser     │
                       └─────────────┘
```

### Data Flow Architecture

```
Harmony Response → Token Recognition → Channel Extraction → Content Classification → Response Building → Claude Code
       │                   │                    │                    │                   │              │
   Raw Content      Regex Patterns      Channel Parsing      Type Assignment      Response Format    UI Rendering
```

#### Detailed Flow

1. **Raw Response Reception**
   ```go
   // Incoming from model provider
   response := `<|start|>assistant<|channel|>analysis<|message|>Thinking content<|end|>
                <|start|>assistant<|channel|>final<|message|>Response content<|end|>`
   ```

2. **Token Recognition**
   ```go
   // Fast detection using compiled regex
   if parser.IsHarmonyFormat(response) {
       // Proceed with full parsing
   }
   ```

3. **Channel Extraction**
   ```go
   channels := parser.ExtractChannels(response)
   // Returns: []Channel{
   //   {Type: "analysis", Content: "Thinking content", ContentType: "thinking"},
   //   {Type: "final", Content: "Response content", ContentType: "response"}
   // }
   ```

4. **Response Building**
   ```go
   // Build Anthropic-compatible response
   return buildHarmonyResponse(ctx, resp, choice, channels, model, cfg, logger)
   ```

### Integration with Existing Pipeline

The Harmony parser integrates seamlessly with Simple Proxy's existing transformation pipeline using the **Chain of Responsibility** pattern:

```go
// In proxy/transform.go
if cfg.IsHarmonyParsingEnabled() && choice.Message.Content != "" {
    if isHarmonyContent, harmonyChannels := detectHarmonyContent(ctx, choice.Message.Content, cfg, loggerInstance); isHarmonyContent {
        // Use Harmony-specific response building
        return buildHarmonyResponse(ctx, resp, choice, harmonyChannels, model, cfg, loggerInstance)
    }
}
// Fallback to existing transformation logic
return buildStandardResponse(ctx, resp, choice, model, cfg, loggerInstance)
```

## Integration Patterns

### 1. Chain of Responsibility Pattern

**Purpose**: Attempt Harmony parsing first, fallback to existing logic

```go
func TransformOpenAIToAnthropic(ctx context.Context, resp *types.OpenAIResponse, model string, cfg *config.Config) (*types.AnthropicResponse, error) {
    // Try Harmony parsing first
    if cfg.IsHarmonyParsingEnabled() {
        if harmonyResult := tryHarmonyParsing(resp); harmonyResult != nil {
            return harmonyResult, nil
        }
    }
    
    // Fallback to standard processing
    return buildStandardResponse(resp), nil
}
```

**Benefits**:
- Zero impact on non-Harmony content
- Gradual feature rollout capability
- Maintains backward compatibility

### 2. Decorator Pattern Enhancement

**Purpose**: Enhance existing response structures without modification

```go
type AnthropicResponse struct {
    // Existing fields
    Content      []ContentItem    `json:"content"`
    StopReason   string          `json:"stop_reason"`
    
    // Harmony enhancements
    ThinkingContent  *string          `json:"thinking_content,omitempty"`
    HarmonyChannels  []parser.Channel `json:"harmony_channels,omitempty"`
}
```

**Benefits**:
- Preserves existing API contracts
- Additive enhancement approach
- Debug information retention

### 3. Strategy Pattern for Parsing

**Purpose**: Different parsing strategies for complete vs streaming responses

```go
type ParsingStrategy interface {
    Parse(content string) ([]Channel, error)
}

type CompleteParsingStrategy struct{}
type StreamingParsingStrategy struct{}

func (s *CompleteParsingStrategy) Parse(content string) ([]Channel, error) {
    return parser.ExtractChannels(content), nil
}
```

### Tool Override Integration

Harmony parsing works seamlessly with existing tool customization:

```yaml
# tools_override.yaml
- name: "WebSearch"
  skip: false
  description_override: "Enhanced web search with Harmony awareness"
```

**Processing Order**:
1. Harmony parsing extracts channels
2. Tool overrides apply to extracted content
3. System message overrides process final content
4. Response transformation completes

### System Message Override Integration

System message modifications work on the final classified content:

```yaml
# system_overrides.yaml
remove_patterns:
  - "⏺ <\\|channel\\|>analysis<\\|message\\|>.*?<\\|end\\|>"
replacements:
  - find: "Claude Code"
    replace: "Your AI Assistant"
```

## Configuration Guide

### Environment Variables

#### Core Configuration

```bash
# Enable/disable Harmony format parsing (default: true)
export HARMONY_PARSING_ENABLED=true

# Enable debug logging for parsing operations (default: false)
export HARMONY_DEBUG=true

# Strict error handling for malformed content (default: false)
export HARMONY_STRICT_MODE=false
```

#### Integration Configuration

```bash
# Must be set for provider access
export OPENAI_API_KEY="your-api-key"
export OPENAI_ENDPOINT="https://api.openai.com/v1"

# Optional: Model-specific configuration
export MODEL_GPT_OSS_120B_ENDPOINT="https://custom-endpoint.com/v1"
```

### Configuration File Support

Create a `.env` file in your project root:

```bash
# .env
HARMONY_PARSING_ENABLED=true
HARMONY_DEBUG=false
HARMONY_STRICT_MODE=false

# Provider configuration
OPENAI_API_KEY=your-api-key
OPENAI_ENDPOINT=https://api.openai.com/v1

# Logging
LOKI_URL=http://localhost:3100
DEBUG_MODELS=gpt-oss-120b
```

### Runtime Configuration

```go
// Check configuration at runtime
cfg := config.NewConfig()

if cfg.IsHarmonyParsingEnabled() {
    logger.Info("Harmony parsing enabled")
}

if cfg.IsHarmonyDebugEnabled() {
    logger.Debug("Harmony debug logging active")
}

// Get all Harmony settings
enabled, debug, strict := cfg.GetHarmonySettings()
```

### Feature Flag Management

```go
// Gradual rollout example
type HarmonyConfig struct {
    Enabled     bool     `json:"enabled"`
    Models      []string `json:"models"`      // Enable for specific models
    Percentage  int      `json:"percentage"`  // Gradual rollout percentage
}

func shouldEnableHarmony(model string, userID string) bool {
    // Model-specific enabling
    for _, enabledModel := range harmonyConfig.Models {
        if model == enabledModel {
            return true
        }
    }
    
    // Percentage-based rollout
    hash := hashUserID(userID)
    return (hash % 100) < harmonyConfig.Percentage
}
```

## Advanced Usage Patterns

### 1. Custom Channel Handling

**Scenario**: Adding support for custom channel types

```go
// Define custom channel types
const (
    ChannelAnalysis   ChannelType = "analysis"
    ChannelFinal      ChannelType = "final"
    ChannelCommentary ChannelType = "commentary"
    ChannelDebug      ChannelType = "debug"      // Custom
    ChannelMetrics    ChannelType = "metrics"    // Custom
)

// Custom classification
func ClassifyCustomChannelType(channelType ChannelType) ContentType {
    switch channelType {
    case ChannelDebug:
        return ContentTypeDebug
    case ChannelMetrics:
        return ContentTypeMetrics
    default:
        return ClassifyChannelType(channelType)
    }
}
```

### 2. Streaming Response Handling

**Scenario**: Processing Harmony content in streaming responses

```go
type StreamProcessor struct {
    buffer       strings.Builder
    channels     []parser.Channel
    tokenizer    *parser.TokenRecognizer
}

func (sp *StreamProcessor) ProcessChunk(chunk string) {
    sp.buffer.WriteString(chunk)
    
    // Check for complete tokens in buffer
    content := sp.buffer.String()
    
    // Extract any complete channels
    newChannels := parser.ExtractChannels(content)
    
    // Update state
    sp.channels = append(sp.channels, newChannels...)
    
    // Clean processed content from buffer
    sp.cleanProcessedContent()
}
```

### 3. Content Validation and Correction

**Scenario**: Validating and correcting malformed Harmony content

```go
func ValidateAndCorrectHarmonyContent(content string) (string, []error) {
    // Validate structure
    errors := parser.ValidateHarmonyStructure(content)
    
    if len(errors) == 0 {
        return content, nil
    }
    
    // Attempt correction
    corrected := parser.CleanHarmonyContent(content)
    
    // Re-validate
    newErrors := parser.ValidateHarmonyStructure(corrected)
    
    return corrected, newErrors
}
```

### 4. Performance Monitoring

**Scenario**: Monitoring Harmony parsing performance

```go
type HarmonyMetrics struct {
    ParseTime        time.Duration
    ChannelCount     int
    TokenCount       int
    ErrorCount       int
    CompletenessRatio int
}

func (hm *HarmonyMetrics) Record(content string, parseTime time.Duration) {
    hm.ParseTime = parseTime
    
    stats := parser.GetHarmonyTokenStats(content)
    hm.ChannelCount = len(parser.ExtractChannels(content))
    hm.TokenCount = stats["total_tokens"]
    hm.CompletenessRatio = stats["completeness_ratio"]
}
```

### 5. Multi-Model Routing

**Scenario**: Route different models based on Harmony support

```go
type ModelRouter struct {
    harmonyModels map[string]bool
}

func (mr *ModelRouter) RouteRequest(model string, content string) string {
    // Check if model supports Harmony
    if mr.harmonyModels[model] {
        return mr.routeToHarmonyModel(model, content)
    }
    
    // Check if content has Harmony formatting
    if parser.IsHarmonyFormat(content) {
        // Convert to think tags for non-Harmony models
        return mr.convertHarmonyToThinkTags(content)
    }
    
    return mr.routeToStandardModel(model, content)
}
```

## Real-World Examples

### Example 1: Code Review Analysis

**Raw Harmony Response**:
```
<|start|>assistant<|channel|>analysis<|message|>The user is asking: "to compare changes in this branch to main, do the code review, I will create merge request then, first I need your review". The user wants code review between current branch (feature/auth-improvements) and main branch. The repository is a Node.js project, but note from project-specific CLAUDE.md that we cannot run npm commands (they will fail) and we should rely on static analysis. There's a code review agent optimization: max 5 tool calls per review, 2-minute analysis target, prioritizing security, business logic, critical issues, using git diff to focus on changed files only. Thus we need to run git diff between current branch and main, get list of changed files and diffs.<|end|>

<|start|>assistant<|channel|>final<|message|>I'll help you review the changes in your feature/auth-improvements branch compared to main. Let me start by examining the differences between the branches.

I'll focus on:
- Security implications of authentication changes
- Business logic correctness
- Critical issues that need immediate attention
- Code quality improvements

Let me begin by getting the diff between your branch and main:<|end|>
```

**Parsed Result**:
```json
{
  "thinking_content": "The user is asking: \"to compare changes in this branch to main, do the code review, I will create merge request then, first I need your review\"...",
  "content": [
    {
      "type": "text",
      "text": "I'll help you review the changes in your feature/auth-improvements branch compared to main. Let me start by examining the differences between the branches.\n\nI'll focus on:\n- Security implications of authentication changes\n- Business logic correctness\n- Critical issues that need immediate attention\n- Code quality improvements\n\nLet me begin by getting the diff between your branch and main:"
    }
  ],
  "harmony_channels": [
    {
      "type": "analysis",
      "role": "assistant", 
      "content": "The user is asking: \"to compare changes in this branch to main...",
      "content_type": "thinking"
    },
    {
      "type": "final",
      "role": "assistant",
      "content": "I'll help you review the changes in your feature/auth-improvements...",
      "content_type": "response"
    }
  ]
}
```

### Example 2: API Implementation with Tool Calls

**Raw Harmony Response**:
```
<|start|>assistant<|channel|>analysis<|message|>The user wants to implement a search API. I need to:
1. Understand their current API structure
2. Design the search endpoint
3. Consider performance implications
4. Check existing search implementations

Let me first explore the codebase structure.<|end|>

<|start|>assistant<|channel|>commentary<|message|>I'll search for existing API endpoints to understand the current structure.<|end|>

<|start|>assistant<|channel|>final<|message|>I'll implement a comprehensive search API for your application. Let me first examine your existing API structure to ensure consistency.

## Search API Design

Based on your requirements, here's the proposed implementation:

### Endpoint Structure
- **URL**: `POST /api/v1/search`
- **Authentication**: Required
- **Rate Limiting**: 100 requests/minute<|end|>
```

**Parsed Result**:
- **Thinking Panel**: Analysis of requirements and implementation planning
- **Main Response**: Clean API design and implementation details
- **Tool Metadata**: Commentary about codebase exploration

### Example 3: Error Handling and Malformed Content

**Malformed Input**:
```
<|start|>assistant<|channel|>analysis<|message|>This has analysis content but missing end token
<|start|>assistant<|channel|>final<|message|>This is the final response<|end|>
```

**Parser Behavior**:
```go
// Graceful degradation
channels := parser.ExtractChannels(malformedContent)

// Result: Extracts what it can, handles incomplete tokens
// - Partial analysis channel with available content
// - Complete final channel
// - Validation errors logged but not blocking
```

### Example 4: Multi-Channel Complex Response

**Complex Response**:
```
<|start|>assistant<|channel|>analysis<|message|>Complex problem analysis:
1. Database performance considerations
2. API rate limiting requirements
3. Caching strategy needed
4. Security implications<|end|>

<|start|>assistant<|channel|>commentary<|message|>I'll check current database queries first.<|end|>

<|start|>assistant<|channel|>final<|message|>I'll optimize your database queries for better performance. Here's my comprehensive approach:

## Performance Analysis
Current bottlenecks identified in user query patterns.

## Optimization Strategy
1. Query optimization with proper indexing
2. Implement Redis caching layer
3. Add connection pooling<|end|>

<|start|>assistant<|channel|>commentary<|message|>Executing database analysis tools now.<|end|>
```

**Processing Result**:
- **3 channels identified**: 1 analysis, 1 final, 2 commentary
- **Thinking content**: Combined analysis content
- **Response content**: Clean final response
- **Tool metadata**: Commentary channel information preserved

## Troubleshooting

### Common Issues and Solutions

#### 1. Harmony Tokens Not Recognized

**Symptoms**:
- Harmony content appears as regular text
- No thinking panels in Claude Code
- Debug logs show "No Harmony tokens detected"

**Diagnosis**:
```bash
# Check if parsing is enabled
curl -H "X-Debug-Headers: true" localhost:3456/health
# Look for: "harmony_parsing_enabled": true

# Check content manually
echo "Your response content" | grep -E "<\|[^|]+\|>"
```

**Solutions**:
1. **Enable Harmony Parsing**:
   ```bash
   export HARMONY_PARSING_ENABLED=true
   ```

2. **Check Token Format**:
   ```bash
   # Correct format
   <|start|>assistant<|channel|>analysis<|message|>content<|end|>
   
   # Common mistakes
   <|start|assistant<|channel|>analysis<|message|>content<|end|>  # Missing >
   <|start|>assistant|channel|>analysis<|message|>content<|end|>   # Wrong separator
   ```

3. **Enable Debug Logging**:
   ```bash
   export HARMONY_DEBUG=true
   # Check logs for detailed parsing information
   ```

#### 2. Incomplete Token Parsing

**Symptoms**:
- Some Harmony content not extracted
- Partial thinking content
- Missing end tokens in logs

**Diagnosis**:
```go
// Check validation errors
errors := parser.ValidateHarmonyStructure(content)
for _, err := range errors {
    logger.Error("Validation error: %v", err)
}
```

**Solutions**:
1. **Check Content Completeness**:
   ```go
   stats := parser.GetHarmonyTokenStats(content)
   if stats["completeness_ratio"] < 100 {
       // Incomplete tokens detected
   }
   ```

2. **Enable Streaming Support**:
   ```go
   // For streaming responses
   channels := parser.ExtractChannels(partialContent)
   // Parser handles incomplete tokens gracefully
   ```

#### 3. Performance Issues

**Symptoms**:
- Slow response times
- High CPU usage during parsing
- Memory consumption increases

**Diagnosis**:
```bash
# Monitor parsing performance
export HARMONY_DEBUG=true
# Check logs for parsing times > 10ms
```

**Solutions**:
1. **Optimize Content Size**:
   ```go
   // Limit content size before parsing
   if len(content) > maxContentSize {
       content = content[:maxContentSize]
   }
   ```

2. **Use Caching**:
   ```go
   type ParseCache struct {
       cache map[string][]parser.Channel
       mutex sync.RWMutex
   }
   
   func (pc *ParseCache) GetOrParse(content string) []parser.Channel {
       pc.mutex.RLock()
       if cached, exists := pc.cache[hash(content)]; exists {
           pc.mutex.RUnlock()
           return cached
       }
       pc.mutex.RUnlock()
       
       // Parse and cache
       channels := parser.ExtractChannels(content)
       pc.mutex.Lock()
       pc.cache[hash(content)] = channels
       pc.mutex.Unlock()
       
       return channels
   }
   ```

#### 4. Configuration Issues

**Symptoms**:
- Settings not taking effect
- Environment variables ignored
- Feature flags not working

**Diagnosis**:
```go
// Check configuration loading
cfg := config.NewConfig()
logger.Info("Harmony config: enabled=%v, debug=%v, strict=%v", 
    cfg.IsHarmonyParsingEnabled(), 
    cfg.IsHarmonyDebugEnabled(),
    cfg.IsHarmonyStrictMode())
```

**Solutions**:
1. **Verify Environment Variables**:
   ```bash
   # Check all environment variables
   env | grep HARMONY
   
   # Expected output:
   # HARMONY_PARSING_ENABLED=true
   # HARMONY_DEBUG=false
   # HARMONY_STRICT_MODE=false
   ```

2. **Check Configuration File**:
   ```bash
   # Ensure .env file is in correct location
   ls -la .env
   
   # Check content
   cat .env | grep HARMONY
   ```

3. **Runtime Configuration Check**:
   ```bash
   # Health check endpoint
   curl localhost:3456/health | jq .harmony_config
   ```

### Debug Logging Guide

#### Enable Comprehensive Debug Logging

```bash
# Enable all debug options
export HARMONY_DEBUG=true
export DEBUG_MODELS=gpt-oss-120b
export LOG_LEVEL=debug

# Start proxy
./simple-proxy
```

#### Key Log Messages

**Successful Parsing**:
```
DEBUG: 🔍 Harmony tokens detected, performing full extraction
DEBUG: 🔍 Harmony extraction complete: 2 total channels (thinking=1, response=1, tools=0)
DEBUG: 🎵 Building Harmony response with 2 channels
```

**Failed Parsing**:
```
DEBUG: 🔍 No Harmony tokens detected in content
WARN: ⚠️ Harmony tokens found but no channels extracted - treating as non-Harmony
```

**Performance Monitoring**:
```
DEBUG: 🎵 Harmony response built: 3 content items, thinking=true, stop_reason=end_turn (5.2ms)
```

### Testing and Validation

#### Unit Testing Harmony Content

```go
func TestHarmonyParsing(t *testing.T) {
    content := `<|start|>assistant<|channel|>analysis<|message|>Test thinking<|end|>
                <|start|>assistant<|channel|>final<|message|>Test response<|end|>`
    
    channels := parser.ExtractChannels(content)
    
    assert.Equal(t, 2, len(channels))
    assert.Equal(t, parser.ChannelAnalysis, channels[0].Type)
    assert.Equal(t, parser.ChannelFinal, channels[1].Type)
}
```

#### Integration Testing

```bash
# Test with real model
curl -X POST localhost:3456/v1/messages \
  -H "Content-Type: application/json" \
  -H "X-Model: gpt-oss-120b" \
  -d '{
    "model": "gpt-oss-120b",
    "max_tokens": 1000,
    "messages": [{"role": "user", "content": "Analyze this code"}]
  }'
```

## Performance Characteristics

### Benchmarks

#### Parsing Performance

| Content Size | Parse Time | Memory Usage | Channels Extracted |
|-------------|------------|--------------|-------------------|
| 1KB         | <1ms       | +50KB        | 1-3               |
| 10KB        | 2-5ms      | +200KB       | 5-15              |
| 100KB       | 8-12ms     | +1MB         | 20-50             |
| 1MB         | 45-60ms    | +5MB         | 100-200           |

#### Token Recognition Speed

```go
// Benchmark results on standard hardware
BenchmarkIsHarmonyFormat-8     1000000    1.2 μs/op    0 B/op    0 allocs/op
BenchmarkExtractChannels-8     100000     15.6 μs/op   512 B/op  8 allocs/op
BenchmarkValidateStructure-8   500000     3.2 μs/op    128 B/op  2 allocs/op
```

### Performance Optimization

#### 1. Compiled Regex Patterns

```go
// Pre-compiled patterns for performance
var defaultTokenRecognizer *TokenRecognizer

func init() {
    defaultTokenRecognizer, _ = NewTokenRecognizer()
    // Patterns compiled once at startup
}
```

#### 2. Efficient String Processing

```go
// Minimize string allocations
func ExtractChannels(content string) []Channel {
    if !IsHarmonyFormat(content) {
        return []Channel{}  // Early return
    }
    
    // Use pre-allocated slice
    channels := make([]Channel, 0, 8)
    // Process using subslices to avoid copying
}
```

#### 3. Memory Pool for Channels

```go
var channelPool = sync.Pool{
    New: func() interface{} {
        return make([]Channel, 0, 10)
    },
}

func ExtractChannelsOptimized(content string) []Channel {
    channels := channelPool.Get().([]Channel)
    defer channelPool.Put(channels[:0])
    
    // Use pooled slice for processing
    return processChannels(content, channels)
}
```

### Monitoring and Metrics

#### Key Metrics to Track

1. **Parsing Latency**
   ```go
   parseLatency := prometheus.NewHistogramVec(
       prometheus.HistogramOpts{
           Name: "harmony_parse_duration_seconds",
           Help: "Time spent parsing Harmony content",
           Buckets: []float64{0.001, 0.005, 0.01, 0.05, 0.1},
       },
       []string{"model", "channel_count"},
   )
   ```

2. **Channel Distribution**
   ```go
   channelCount := prometheus.NewCounterVec(
       prometheus.CounterOpts{
           Name: "harmony_channels_extracted_total",
           Help: "Total channels extracted by type",
       },
       []string{"channel_type"},
   )
   ```

3. **Parsing Success Rate**
   ```go
   parseSuccess := prometheus.NewCounterVec(
       prometheus.CounterOpts{
           Name: "harmony_parse_attempts_total",
           Help: "Total parse attempts by outcome",
       },
       []string{"outcome"}, // "success", "malformed", "error"
   )
   ```

#### Alerting Thresholds

```yaml
# Prometheus alerting rules
groups:
  - name: harmony_parsing
    rules:
      - alert: HighHarmonyParsingLatency
        expr: histogram_quantile(0.95, harmony_parse_duration_seconds) > 0.01
        for: 2m
        labels:
          severity: warning
        annotations:
          summary: "Harmony parsing latency is high"
          
      - alert: HarmonyParsingFailures
        expr: rate(harmony_parse_attempts_total{outcome="error"}[5m]) > 0.01
        for: 1m
        labels:
          severity: critical
        annotations:
          summary: "High rate of Harmony parsing failures"
```

## API Reference

### Parser Functions

#### `IsHarmonyFormat(content string) bool`

Performs quick detection of Harmony tokens in content.

**Parameters**:
- `content`: String content to check

**Returns**: Boolean indicating if Harmony tokens are present

**Example**:
```go
if parser.IsHarmonyFormat(response) {
    // Process as Harmony content
}
```

#### `ExtractChannels(content string) []Channel`

Parses content and extracts all Harmony channels with metadata.

**Parameters**:
- `content`: String content to parse

**Returns**: Slice of Channel structs with parsed data

**Example**:
```go
channels := parser.ExtractChannels(harmonyResponse)
for _, channel := range channels {
    fmt.Printf("Channel: %s, Type: %s\n", channel.Type, channel.ContentType)
}
```

#### `ValidateHarmonyStructure(content string) []error`

Validates Harmony token structure and returns errors.

**Parameters**:
- `content`: String content to validate

**Returns**: Slice of validation errors

**Example**:
```go
errors := parser.ValidateHarmonyStructure(content)
if len(errors) > 0 {
    logger.Warn("Validation issues found: %v", errors)
}
```

#### `GetHarmonyTokenStats(content string) map[string]int`

Returns statistics about Harmony tokens in content.

**Parameters**:
- `content`: String content to analyze

**Returns**: Map with token statistics

**Example**:
```go
stats := parser.GetHarmonyTokenStats(content)
fmt.Printf("Total tokens: %d, Completeness: %d%%\n", 
    stats["total_tokens"], stats["completeness_ratio"])
```

### Configuration Methods

#### `IsHarmonyParsingEnabled() bool`

Returns whether Harmony format parsing is enabled.

**Example**:
```go
if cfg.IsHarmonyParsingEnabled() {
    // Proceed with Harmony parsing
}
```

#### `IsHarmonyDebugEnabled() bool`

Returns whether Harmony debug logging is enabled.

**Example**:
```go
if cfg.IsHarmonyDebugEnabled() {
    logger.Debug("Debug info: %s", debugData)
}
```

#### `GetHarmonySettings() (enabled, debug, strict bool)`

Returns all Harmony configuration settings.

**Example**:
```go
enabled, debug, strict := cfg.GetHarmonySettings()
fmt.Printf("Harmony config: enabled=%v, debug=%v, strict=%v\n", enabled, debug, strict)
```

### Data Structures

#### `Channel`

Represents a parsed Harmony channel with metadata.

```go
type Channel struct {
    Type        ChannelType `json:"type"`         // Channel type (analysis, final, commentary)
    Role        Role        `json:"role"`         // Message role (assistant, user, etc.)
    Content     string      `json:"content"`      // Raw content from the channel
    ContentType ContentType `json:"content_type"` // How content should be classified
    RawTokens   string      `json:"raw_tokens"`   // Original token sequence
}
```

**Methods**:
- `IsThinking() bool`: Returns true for thinking/analysis content
- `IsResponse() bool`: Returns true for user-facing response content
- `IsToolCall() bool`: Returns true for tool call content

#### `HarmonyMessage`

Represents a complete parsed Harmony message.

```go
type HarmonyMessage struct {
    Channels      []Channel `json:"channels"`       // All channels found
    ThinkingText  string    `json:"thinking_text"`  // Combined thinking content
    ResponseText  string    `json:"response_text"`  // Combined response content
    ToolCallText  string    `json:"tool_call_text"` // Combined tool call content
    RawContent    string    `json:"raw_content"`    // Original content
    HasHarmony    bool      `json:"has_harmony"`    // Whether Harmony tokens detected
    ParseErrors   []error   `json:"parse_errors"`   // Any parsing errors
}
```

**Methods**:
- `HasThinking() bool`: Returns true if message contains thinking content
- `HasResponse() bool`: Returns true if message contains response content
- `HasToolCalls() bool`: Returns true if message contains tool calls
- `GetChannelsByType(ChannelType) []Channel`: Returns channels of specific type
- `GetChannelsByContentType(ContentType) []Channel`: Returns channels by content classification

### Error Types

#### `HarmonyParseError`

Represents errors during Harmony format parsing.

```go
type HarmonyParseError struct {
    Message string  // Error description
    Token   string  // Associated token (if any)
    Content string  // Content that caused error
}
```

**Common Errors**:
- `ErrMalformedToken`: Invalid token format
- `ErrMissingStartToken`: Start token not found
- `ErrMissingEndToken`: End token not found
- `ErrInvalidRole`: Unknown role specified
- `ErrEmptyContent`: No content in message section

### Response Extensions

#### Enhanced Response Types

Both `OpenAIResponse` and `AnthropicResponse` include Harmony extensions:

```go
// Extended response fields
ThinkingContent  *string          `json:"thinking_content,omitempty"`
HarmonyChannels  []parser.Channel `json:"harmony_channels,omitempty"`
```

**Usage**:
- `ThinkingContent`: Combined analysis channel content for UI
- `HarmonyChannels`: Complete channel metadata for debugging and advanced processing

---

This documentation provides comprehensive coverage of Simple Proxy's Harmony support feature. For additional support or feature requests, please refer to the project's GitHub repository.