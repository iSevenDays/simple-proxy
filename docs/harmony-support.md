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

#### 5. Model-Specific Configuration Issues

**Symptoms**:
- Harmony parsing works with some models but not others
- Inconsistent behavior across different providers
- Model routing problems

**Common Model Configurations**:

**gpt-oss-120b (Harmony-native)**:
```bash
# .env configuration for gpt-oss-120b
BIG_MODEL=gpt-oss-120b
BIG_MODEL_ENDPOINT=https://api.gptfree.io/v1/chat/completions
BIG_MODEL_API_KEY=your-api-key

# Harmony settings
HARMONY_PARSING_ENABLED=true
HARMONY_DEBUG=true  # Enable for initial setup
HARMONY_STRICT_MODE=false
HARMONY_TOKEN_VALIDATION=true
```

**Claude Sonnet (think tags)**:
```bash
# Configuration for Claude models (uses <think></think> instead of Harmony)
BIG_MODEL=claude-3-5-sonnet-20241022
BIG_MODEL_ENDPOINT=https://api.anthropic.com/v1/messages
BIG_MODEL_API_KEY=sk-ant-your-key

# Harmony parsing can remain enabled (won't interfere)
HARMONY_PARSING_ENABLED=true
HARMONY_DEBUG=false  # Less useful for Claude models
```

**Local Models (Mixed Support)**:
```bash
# For local models with uncertain Harmony support
SMALL_MODEL=llama-3.1-70b-instruct
SMALL_MODEL_ENDPOINT=http://localhost:11434/v1/chat/completions
SMALL_MODEL_API_KEY=ollama

# Conservative Harmony settings
HARMONY_PARSING_ENABLED=true
HARMONY_STRICT_MODE=false  # Important: graceful fallback
HARMONY_DEBUG=true  # Monitor behavior
```

**Diagnosis Commands**:
```bash
# Test model-specific Harmony behavior
curl -X POST localhost:3456/v1/messages \
  -H "Content-Type: application/json" \
  -H "X-Model: gpt-oss-120b" \
  -H "X-Debug-Headers: true" \
  -d '{
    "model": "gpt-oss-120b",
    "max_tokens": 1000,
    "messages": [{"role": "user", "content": "Please explain your thinking process step by step, then provide a final answer."}]
  }' | jq '.harmony_channels'

# Check response headers for debugging info
curl -v -X POST localhost:3456/v1/messages \
  -H "Content-Type: application/json" \
  -H "X-Model: gpt-oss-120b" \
  -d '{"model": "gpt-oss-120b", "max_tokens": 100, "messages": [{"role": "user", "content": "test"}]}' \
  2>&1 | grep -i harmony
```

#### 6. Performance and Memory Issues

**Symptoms**:
- Slow parsing performance (>50ms for small content)
- High memory usage during parsing
- Memory leaks over time
- OOM errors with large responses

**Performance Diagnosis**:
```bash
# Enable performance monitoring
export HARMONY_DEBUG=true
export HARMONY_PERFORMANCE_OPTIMIZATION=true

# Monitor parsing times in logs
tail -f proxy.log | grep "Harmony response built" | grep -E "\([0-9.]+ms\)"

# Check memory usage
export HARMONY_CACHE_ENABLED=true
export HARMONY_MAX_CONTENT_SIZE=524288  # 512KB limit

# Profile memory usage
go tool pprof http://localhost:3456/debug/pprof/heap
```

**Performance Optimization Solutions**:

1. **Content Size Limiting**:
   ```bash
   # Limit content size for parsing
   export HARMONY_MAX_CONTENT_SIZE=262144  # 256KB limit
   
   # For production with large responses
   export HARMONY_MAX_CONTENT_SIZE=1048576  # 1MB limit
   ```

2. **Caching Configuration**:
   ```bash
   # Enable caching for repeated content (development)
   export HARMONY_CACHE_ENABLED=true
   export HARMONY_CACHE_SIZE=500  # Reduce if memory constrained
   
   # Disable caching for production (unless needed)
   export HARMONY_CACHE_ENABLED=false
   ```

3. **Performance-Optimized Configuration**:
   ```bash
   # Production performance profile
   export HARMONY_PARSING_ENABLED=true
   export HARMONY_DEBUG=false
   export HARMONY_STRICT_MODE=false
   export HARMONY_PERFORMANCE_OPTIMIZATION=true
   export HARMONY_TOKEN_VALIDATION=false  # Skip validation for performance
   export HARMONY_MAX_CONTENT_SIZE=524288
   ```

4. **Memory Monitoring**:
   ```go
   // Add memory monitoring to your application
   func monitorHarmonyMemory() {
       var m runtime.MemStats
       runtime.ReadMemStats(&m)
       
       logger.Info("Memory stats - Alloc: %d KB, TotalAlloc: %d KB, Sys: %d KB", 
           bToKb(m.Alloc), bToKb(m.TotalAlloc), bToKb(m.Sys))
   }
   ```

#### 7. Integration and Pipeline Issues

**Symptoms**:
- Tool overrides not working with Harmony content
- System message overrides ignored
- Streaming responses malformed
- Pipeline processing errors

**Integration Diagnosis**:
```bash
# Test tool override integration
cat tools_override.yaml
export PRINT_TOOL_SCHEMAS=true
export HARMONY_DEBUG=true

# Check system message processing
cat system_overrides.yaml
export PRINT_SYSTEM_MESSAGE=true

# Test streaming compatibility
curl -N -X POST localhost:3456/v1/messages \
  -H "Content-Type: application/json" \
  -H "Accept: text/event-stream" \
  -d '{
    "model": "gpt-oss-120b",
    "stream": true,
    "messages": [{"role": "user", "content": "Stream a Harmony response"}]
  }'
```

**Integration Solutions**:

1. **Tool Override Processing Order**:
   ```yaml
   # tools_override.yaml
   - name: "WebSearch"
     skip: false
     description_override: "Enhanced search with Harmony awareness"
   
   # Verify processing order in logs:
   # 1. Harmony parsing extracts channels
   # 2. Tool overrides apply to extracted content  
   # 3. System overrides process final content
   # 4. Response transformation completes
   ```

2. **System Message Override with Harmony**:
   ```yaml
   # system_overrides.yaml
   remove_patterns:
     # Remove raw Harmony tokens if they leak through
     - "⏺ <\\|channel\\|>analysis<\\|message\\|>.*?<\\|end\\|>"
     - "<\\|start\\|>assistant<\\|channel\\|>.*?<\\|end\\|>"
   
   replacements:
     - find: "Claude Code"
       replace: "Your AI Assistant"
   
   prepend: "You are an expert assistant with access to thinking capabilities.\n\n"
   ```

3. **Streaming Response Handling**:
   ```go
   // Ensure proper streaming buffer management
   type HarmonyStreamProcessor struct {
       buffer       strings.Builder
       channels     []parser.Channel
       tokenBuffer  string
   }
   
   func (hsp *HarmonyStreamProcessor) ProcessChunk(chunk string) error {
       // Append to buffer
       hsp.buffer.WriteString(chunk)
       
       // Process complete tokens only
       content := hsp.buffer.String()
       if completeChannels := extractCompleteChannels(content); len(completeChannels) > 0 {
           hsp.channels = append(hsp.channels, completeChannels...)
           hsp.cleanProcessedContent()
       }
       
       return nil
   }
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

#### Key Log Messages and Interpretation

**Successful Parsing**:
```
DEBUG: 🔍 Harmony tokens detected, performing full extraction
DEBUG: 🔍 Harmony extraction complete: 2 total channels (thinking=1, response=1, tools=0)
DEBUG: 🎵 Building Harmony response with 2 channels
DEBUG: 🎵 Harmony response built: 3 content items, thinking=true, stop_reason=end_turn (5.2ms)
```
**Interpretation**: Normal operation with 2 channels detected (thinking + response), parsed in 5.2ms

**Failed Parsing - No Tokens**:
```
DEBUG: 🔍 No Harmony tokens detected in content
DEBUG: 📝 Using standard response transformation
```
**Interpretation**: Content doesn't contain Harmony tokens, fallback to normal processing (expected for non-Harmony models)

**Failed Parsing - Malformed Content**:
```
DEBUG: 🔍 Harmony tokens detected, performing full extraction  
WARN: ⚠️ Harmony tokens found but no channels extracted - treating as non-Harmony
WARN: ⚠️ Malformed token sequence detected: missing end token for channel 'analysis'
```
**Interpretation**: Harmony tokens exist but parsing failed due to malformed structure

**Performance Issues**:
```
DEBUG: 🎵 Harmony response built: 1 content items, thinking=false, stop_reason=end_turn (127ms)
WARN: ⚠️ Harmony parsing took longer than expected: 127ms for 45KB content
```
**Interpretation**: Slow parsing performance indicates potential optimization needs

**Memory Issues**:
```
DEBUG: 🧠 Harmony cache enabled: 450/1000 entries (2.1MB memory usage)
WARN: ⚠️ Content size 2.5MB exceeds limit 1MB, truncating before parsing
ERROR: ❌ Harmony parsing failed: out of memory during channel extraction
```
**Interpretation**: Memory constraints affecting parsing, consider reducing content size or disabling cache

**Configuration Issues**:
```
INFO: ⚙️ Harmony parsing disabled via configuration
DEBUG: 🔧 Harmony config: enabled=false, debug=true, strict=false
WARN: ⚠️ HARMONY_DEBUG=true but HARMONY_PARSING_ENABLED=false - debug logs will be minimal
```
**Interpretation**: Configuration mismatch or intentional disabling

**Streaming-Specific Logs**:
```
DEBUG: 🌊 Processing streaming chunk 3/7: partial channel detected
DEBUG: 🌊 Streaming buffer: 1024 bytes, complete channels: 1
DEBUG: 🌊 Final streaming consolidation: 2 channels merged
```
**Interpretation**: Streaming response processing with gradual channel assembly

**Token Validation Errors**:
```
WARN: 🔍 Token validation failed: role 'assitant' not recognized (typo?)
WARN: 🔍 Token validation failed: channel 'analysys' not standard (typo?)
DEBUG: 🔍 Validation errors: 2, proceeding with graceful parsing
```
**Interpretation**: Typos in Harmony tokens, but graceful mode continues processing

### Common Parsing Issues and Advanced Solutions

#### Issue 1: Token Recognition Failures

**Problem**: Model output contains Harmony-like content but isn't detected
```
Response: "I need to think about this <|channel|>analysis but the format is wrong"
Log: DEBUG: 🔍 No Harmony tokens detected in content
```

**Root Cause Analysis**:
```bash
# Check exact token format
echo "Response content" | grep -E "<\|[^|]+\|>"

# Expected format
<|start|>assistant<|channel|>analysis<|message|>content<|end|>

# Common malformed patterns
<|channel|>analysis<|message|>content          # Missing start/end
<start>assistant<channel>analysis<message>     # Missing pipe delimiters
<|start|>assistant|channel|>analysis|message>  # Wrong separator usage
```

**Solutions**:
```go
// 1. Enable token validation to identify format issues
export HARMONY_TOKEN_VALIDATION=true
export HARMONY_DEBUG=true

// 2. Check regex pattern matching
func debugTokenMatching(content string) {
    patterns := []string{
        `<\|start\|>`,           // Start token
        `<\|channel\|>`,         // Channel delimiter  
        `<\|message\|>`,         // Message delimiter
        `<\|end\|>`,             // End token
    }
    
    for _, pattern := range patterns {
        if matched, _ := regexp.MatchString(pattern, content); matched {
            logger.Debug("Pattern matched: %s", pattern)
        } else {
            logger.Warn("Pattern missing: %s", pattern)
        }
    }
}
```

#### Issue 2: Incomplete Channel Extraction

**Problem**: Some channels extracted, others missed
```
Log: DEBUG: 🔍 Harmony extraction complete: 1 total channels (thinking=1, response=0, tools=0)
Expected: 2 channels (thinking + response)
```

**Common Patterns**:
```
# Pattern 1: Missing end token
<|start|>assistant<|channel|>analysis<|message|>thinking content
<|start|>assistant<|channel|>final<|message|>response content<|end|>

# Pattern 2: Nested tokens (unsupported)
<|start|>assistant<|channel|>analysis<|message|>
  I think <|channel|>final<|message|>this is wrong<|end|>
<|end|>

# Pattern 3: Content spanning multiple channels incorrectly
<|start|>assistant<|channel|>analysis<|message|>thinking<|channel|>final<|message|>response<|end|>
```

**Solutions**:
```go
// 1. Content preprocessing for common issues
func preprocessHarmonyContent(content string) string {
    // Fix missing end tokens (heuristic)
    content = regexp.MustCompile(`(<\|start\|>[^<]*<\|message\|>[^<]*?)(<\|start\|>)`).ReplaceAllString(content, "${1}<|end|>${2}")
    
    // Remove nested channel tokens
    content = regexp.MustCompile(`(<\|message\|>[^<]*?)<\|channel\|>[^<]*?<\|message\|>`).ReplaceAllString(content, "${1}")
    
    return content
}

// 2. Validation and reporting
func validateChannelCompleteness(content string, channels []parser.Channel) []error {
    errors := []error{}
    
    // Count expected vs actual channels
    expectedStart := strings.Count(content, "<|start|>")
    extractedChannels := len(channels)
    
    if expectedStart != extractedChannels {
        errors = append(errors, fmt.Errorf(
            "channel count mismatch: expected %d, extracted %d", 
            expectedStart, extractedChannels))
    }
    
    return errors
}
```

#### Issue 3: Content Truncation and Loss

**Problem**: Large responses truncated or content lost during parsing
```
Log: WARN: ⚠️ Content size 2.5MB exceeds limit 1MB, truncating before parsing
Log: WARN: ⚠️ Channel content truncated: original 15KB, extracted 8KB
```

**Analysis**:
```go
// Monitor content sizes throughout pipeline
func monitorContentFlow(stage string, content string) {
    size := len(content)
    channelCount := strings.Count(content, "<|start|>")
    
    logger.Debug("Content flow [%s]: size=%d bytes, channels=%d", 
        stage, size, channelCount)
    
    if size > 100000 { // 100KB
        logger.Warn("Large content detected at stage %s: %d bytes", stage, size)
    }
}
```

**Solutions**:
```bash
# 1. Increase content size limits
export HARMONY_MAX_CONTENT_SIZE=2097152  # 2MB

# 2. Implement smart truncation
export HARMONY_SMART_TRUNCATION=true  # Keep complete channels only

# 3. Stream processing for large content
export HARMONY_STREAMING_PARSE=true   # Process in chunks
```

#### Issue 4: Model-Specific Token Variations

**Problem**: Different models use slightly different Harmony formats
```
# gpt-oss-120b format
<|start|>assistant<|channel|>analysis<|message|>content<|end|>

# Custom model variations
<|begin|>assistant<|type|>thinking<|content|>content<|finish|>
[START]assistant[CHANNEL]analysis[MSG]content[END]
```

**Adaptive Parsing Solution**:
```go
// Multi-pattern parser with model-specific configurations
type ModelConfig struct {
    Name         string
    StartToken   string
    EndToken     string
    ChannelToken string
    MessageToken string
}

var modelConfigs = map[string]ModelConfig{
    "gpt-oss-120b": {
        StartToken:   `<\|start\|>`,
        EndToken:     `<\|end\|>`, 
        ChannelToken: `<\|channel\|>`,
        MessageToken: `<\|message\|>`,
    },
    "custom-harmony-model": {
        StartToken:   `<\|begin\|>`,
        EndToken:     `<\|finish\|>`,
        ChannelToken: `<\|type\|>`, 
        MessageToken: `<\|content\|>`,
    },
}

func parseWithModelConfig(content string, model string) []parser.Channel {
    config, exists := modelConfigs[model]
    if !exists {
        // Fallback to standard parsing
        return parser.ExtractChannels(content)
    }
    
    return parseWithCustomTokens(content, config)
}
```

#### Issue 5: Performance Degradation with Complex Content

**Problem**: Parsing performance degrades significantly with complex Harmony structures
```
Log: WARN: ⚠️ Harmony parsing took longer than expected: 2.3s for 500KB content
Log: ERROR: ❌ Parser timeout after 5s, falling back to standard processing
```

**Performance Analysis**:
```go
// Benchmark different content patterns
func BenchmarkComplexHarmonyParsing(b *testing.B) {
    // Test patterns
    simpleContent := generateHarmonyContent(2, 1000)    // 2 channels, 1KB each
    complexContent := generateHarmonyContent(50, 10000) // 50 channels, 10KB each
    nestedContent := generateNestedHarmonyContent(10)   // 10 levels deep
    
    b.Run("Simple", func(b *testing.B) {
        for i := 0; i < b.N; i++ {
            parser.ExtractChannels(simpleContent)
        }
    })
    
    b.Run("Complex", func(b *testing.B) {
        for i := 0; i < b.N; i++ {
            parser.ExtractChannels(complexContent)
        }
    })
}
```

**Optimization Solutions**:
```go
// 1. Early content analysis
func shouldUseOptimizedParsing(content string) bool {
    channelCount := strings.Count(content, "<|start|>")
    contentSize := len(content)
    
    // Use optimized parsing for complex content
    return channelCount > 10 || contentSize > 50000
}

// 2. Parallel channel processing
func extractChannelsParallel(content string) []parser.Channel {
    channelBoundaries := findChannelBoundaries(content)
    
    channels := make([]parser.Channel, len(channelBoundaries))
    var wg sync.WaitGroup
    
    for i, boundary := range channelBoundaries {
        wg.Add(1)
        go func(i int, boundary ChannelBoundary) {
            defer wg.Done()
            channels[i] = parseChannel(content[boundary.Start:boundary.End])
        }(i, boundary)
    }
    
    wg.Wait()
    return channels
}

// 3. Content caching with TTL
type ParseCache struct {
    cache map[string]CacheEntry
    mutex sync.RWMutex
}

type CacheEntry struct {
    channels []parser.Channel
    expiry   time.Time
}

func (pc *ParseCache) GetOrParse(content string) []parser.Channel {
    hash := hashContent(content)
    
    pc.mutex.RLock()
    if entry, exists := pc.cache[hash]; exists && time.Now().Before(entry.expiry) {
        pc.mutex.RUnlock()
        return entry.channels
    }
    pc.mutex.RUnlock()
    
    // Parse and cache with TTL
    channels := parser.ExtractChannels(content)
    pc.mutex.Lock()
    pc.cache[hash] = CacheEntry{
        channels: channels,
        expiry:   time.Now().Add(5 * time.Minute),
    }
    pc.mutex.Unlock()
    
    return channels
}
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

// Test malformed content handling
func TestMalformedHarmonyContent(t *testing.T) {
    testCases := []struct {
        name     string
        content  string
        expected int // Expected number of channels extracted
    }{
        {
            name: "missing_end_token",
            content: `<|start|>assistant<|channel|>analysis<|message|>thinking content
                     <|start|>assistant<|channel|>final<|message|>response<|end|>`,
            expected: 1, // Only second channel complete
        },
        {
            name: "invalid_role",
            content: `<|start|>assitant<|channel|>analysis<|message|>content<|end|>`,
            expected: 0, // Validation should reject
        },
        {
            name: "nested_tokens",
            content: `<|start|>assistant<|channel|>analysis<|message|>outer <|start|> inner <|end|> content<|end|>`,
            expected: 1, // Should handle gracefully
        },
    }
    
    for _, tc := range testCases {
        t.Run(tc.name, func(t *testing.T) {
            channels := parser.ExtractChannels(tc.content)
            assert.Equal(t, tc.expected, len(channels), "Unexpected channel count for %s", tc.name)
        })
    }
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

# Comprehensive integration test script
#!/bin/bash
# integration_test.sh

# Test different configurations
configurations=(
    "HARMONY_PARSING_ENABLED=true HARMONY_DEBUG=true HARMONY_STRICT_MODE=false"
    "HARMONY_PARSING_ENABLED=true HARMONY_DEBUG=false HARMONY_STRICT_MODE=true"
    "HARMONY_PARSING_ENABLED=false"
)

for config in "${configurations[@]}"; do
    echo "Testing configuration: $config"
    
    # Set environment
    eval "export $config"
    
    # Restart proxy
    pkill -f simple-proxy
    ./simple-proxy &
    PROXY_PID=$!
    sleep 2
    
    # Run test requests
    response=$(curl -s -X POST localhost:3456/v1/messages \
        -H "Content-Type: application/json" \
        -H "X-Model: gpt-oss-120b" \
        -d '{
            "model": "gpt-oss-120b",
            "max_tokens": 500,
            "messages": [{"role": "user", "content": "Please provide a detailed analysis with your thinking process."}]
        }')
    
    # Validate response structure
    if echo "$response" | jq -e '.thinking_content' > /dev/null; then
        echo "✅ Thinking content present"
    else
        echo "❌ Thinking content missing"
    fi
    
    if echo "$response" | jq -e '.harmony_channels' > /dev/null; then
        echo "✅ Harmony channels present"
    else
        echo "❌ Harmony channels missing" 
    fi
    
    # Clean up
    kill $PROXY_PID
    echo "---"
done
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

#### 4. Content Preprocessing Optimization

```go
// Optimize content before parsing
type ContentOptimizer struct {
    maxSize          int
    enableTruncation bool
    smartTruncation  bool
}

func (co *ContentOptimizer) OptimizeContent(content string) string {
    // 1. Size-based optimization
    if co.enableTruncation && len(content) > co.maxSize {
        if co.smartTruncation {
            // Keep complete channels only
            return co.truncateAtChannelBoundary(content)
        }
        return content[:co.maxSize]
    }
    
    // 2. Remove redundant whitespace
    content = regexp.MustCompile(`\s+`).ReplaceAllString(content, " ")
    
    // 3. Normalize token spacing
    content = regexp.MustCompile(`(<\|[^|]+\|>)\s*`).ReplaceAllString(content, "${1}")
    
    return content
}

func (co *ContentOptimizer) truncateAtChannelBoundary(content string) string {
    // Find last complete channel within size limit
    for i := co.maxSize; i > 0; i-- {
        if strings.HasPrefix(content[i:], "<|end|>") {
            return content[:i+7] // Include the end token
        }
    }
    return content[:co.maxSize]
}
```

#### 5. Parallel Processing for Large Content

```go
// Process multiple channels in parallel
type ParallelParser struct {
    workerCount int
    chunkSize   int
}

func (pp *ParallelParser) ExtractChannels(content string) []Channel {
    chunks := pp.splitIntoChunks(content)
    if len(chunks) <= 1 {
        // Small content, use sequential processing
        return parser.ExtractChannels(content)
    }
    
    // Parallel processing for large content
    channelChan := make(chan []parser.Channel, len(chunks))
    var wg sync.WaitGroup
    
    for _, chunk := range chunks {
        wg.Add(1)
        go func(chunk string) {
            defer wg.Done()
            channelChan <- parser.ExtractChannels(chunk)
        }(chunk)
    }
    
    wg.Wait()
    close(channelChan)
    
    // Merge results
    var allChannels []parser.Channel
    for channels := range channelChan {
        allChannels = append(allChannels, channels...)
    }
    
    return allChannels
}
```

#### 6. Intelligent Caching Strategy

```go
// Multi-tier caching system
type IntelligentCache struct {
    l1Cache    map[string][]parser.Channel  // In-memory, small & fast
    l2Cache    *lru.Cache                   // LRU cache, medium size  
    l3Cache    *redis.Client               // Redis cache, large & persistent
    
    l1Size     int
    l2Size     int
    l3TTL      time.Duration
}

func (ic *IntelligentCache) GetOrParse(content string) []parser.Channel {
    hash := ic.hashContent(content)
    
    // L1: In-memory cache (fastest)
    if channels, exists := ic.l1Cache[hash]; exists {
        return channels
    }
    
    // L2: LRU cache  
    if channels, exists := ic.l2Cache.Get(hash); exists {
        channelsSlice := channels.([]parser.Channel)
        ic.l1Cache[hash] = channelsSlice  // Promote to L1
        return channelsSlice
    }
    
    // L3: Redis cache (distributed)
    if channels, err := ic.getFromRedis(hash); err == nil {
        ic.l2Cache.Add(hash, channels)    // Promote to L2
        ic.l1Cache[hash] = channels       // Promote to L1
        return channels
    }
    
    // Parse and cache at all levels
    channels := parser.ExtractChannels(content)
    
    ic.l1Cache[hash] = channels
    ic.l2Cache.Add(hash, channels)
    ic.setInRedis(hash, channels, ic.l3TTL)
    
    return channels
}
```

#### 7. Environment-Specific Optimizations

```bash
# Development environment (debugging over performance)
export HARMONY_PARSING_ENABLED=true
export HARMONY_DEBUG=true
export HARMONY_STRICT_MODE=true
export HARMONY_TOKEN_VALIDATION=true
export HARMONY_CACHE_ENABLED=true
export HARMONY_CACHE_SIZE=100          # Small cache
export HARMONY_MAX_CONTENT_SIZE=262144 # 256KB limit

# Production environment (maximum performance)
export HARMONY_PARSING_ENABLED=true
export HARMONY_DEBUG=false
export HARMONY_STRICT_MODE=false
export HARMONY_PERFORMANCE_OPTIMIZATION=true
export HARMONY_TOKEN_VALIDATION=false  # Skip validation
export HARMONY_CACHE_ENABLED=true
export HARMONY_CACHE_SIZE=5000         # Large cache
export HARMONY_MAX_CONTENT_SIZE=1048576 # 1MB limit

# Memory-constrained environment
export HARMONY_PARSING_ENABLED=true
export HARMONY_DEBUG=false
export HARMONY_STRICT_MODE=false
export HARMONY_CACHE_ENABLED=false     # Disable caching
export HARMONY_MAX_CONTENT_SIZE=131072 # 128KB limit
export HARMONY_PERFORMANCE_OPTIMIZATION=true

# High-throughput environment
export HARMONY_PARSING_ENABLED=true
export HARMONY_DEBUG=false
export HARMONY_STRICT_MODE=false
export HARMONY_PERFORMANCE_OPTIMIZATION=true
export HARMONY_PARALLEL_PROCESSING=true
export HARMONY_WORKER_COUNT=4          # CPU cores
export HARMONY_CACHE_ENABLED=true
export HARMONY_CACHE_SIZE=10000        # Very large cache
```

#### 8. Performance Monitoring and Auto-Tuning

```go
// Auto-tuning performance system
type PerformanceMonitor struct {
    parseLatencies []time.Duration
    memoryUsage   []uint64
    cacheHitRate  float64
    
    config *config.Config
    mutex  sync.RWMutex
}

func (pm *PerformanceMonitor) RecordParseLatency(latency time.Duration) {
    pm.mutex.Lock()
    defer pm.mutex.Unlock()
    
    pm.parseLatencies = append(pm.parseLatencies, latency)
    
    // Auto-tune based on performance
    if len(pm.parseLatencies) >= 100 {
        pm.autoTune()
        pm.parseLatencies = []time.Duration{} // Reset
    }
}

func (pm *PerformanceMonitor) autoTune() {
    avgLatency := pm.calculateAverageLatency()
    p95Latency := pm.calculateP95Latency()
    
    // Auto-adjust cache size
    if p95Latency > 50*time.Millisecond {
        // Performance is slow, increase cache
        currentSize := pm.config.HarmonyCacheSize
        newSize := int(float64(currentSize) * 1.2)
        pm.config.SetHarmonyCacheSize(newSize)
        logger.Info("Auto-tuned: increased cache size to %d", newSize)
    }
    
    // Auto-adjust content size limit
    if avgLatency > 20*time.Millisecond {
        // Reduce content size limit
        currentLimit := pm.config.HarmonyMaxContentSize
        newLimit := int(float64(currentLimit) * 0.8)
        pm.config.SetHarmonyMaxContentSize(newLimit)
        logger.Info("Auto-tuned: reduced content size limit to %d", newLimit)
    }
    
    // Auto-enable parallel processing
    if p95Latency > 100*time.Millisecond && !pm.config.HarmonyParallelProcessing {
        pm.config.SetHarmonyParallelProcessing(true)
        logger.Info("Auto-tuned: enabled parallel processing")
    }
}
```

#### 9. Hardware-Specific Optimizations

```go
// CPU-optimized parsing
func optimizeForCPU() {
    // Detect CPU characteristics
    cpuCount := runtime.NumCPU()
    l1CacheSize := getCPUL1CacheSize() // Platform-specific
    
    // Configure based on CPU
    if cpuCount >= 8 {
        // High-core count: enable parallel processing
        os.Setenv("HARMONY_PARALLEL_PROCESSING", "true")
        os.Setenv("HARMONY_WORKER_COUNT", fmt.Sprintf("%d", cpuCount/2))
    }
    
    if l1CacheSize >= 32*1024 { // 32KB L1 cache
        // Large cache: increase pattern buffer sizes
        os.Setenv("HARMONY_PATTERN_BUFFER_SIZE", "8192")
    }
    
    // Set GOMAXPROCS for optimal Go performance
    runtime.GOMAXPROCS(cpuCount)
}

// Memory-optimized parsing  
func optimizeForMemory() {
    var m runtime.MemStats
    runtime.ReadMemStats(&m)
    
    availableMemory := m.Sys
    
    if availableMemory < 512*1024*1024 { // Less than 512MB
        // Low memory: disable caching, reduce limits
        os.Setenv("HARMONY_CACHE_ENABLED", "false")
        os.Setenv("HARMONY_MAX_CONTENT_SIZE", "65536") // 64KB
        os.Setenv("HARMONY_PARALLEL_PROCESSING", "false")
    } else if availableMemory > 4*1024*1024*1024 { // More than 4GB
        // High memory: enable large cache
        os.Setenv("HARMONY_CACHE_SIZE", "50000")
        os.Setenv("HARMONY_MAX_CONTENT_SIZE", "4194304") // 4MB
    }
}
```

#### 10. Profiling and Continuous Optimization

```go
// Built-in profiling support
import _ "net/http/pprof"

func enableProfiling() {
    go func() {
        log.Println(http.ListenAndServe("localhost:6060", nil))
    }()
}

// Usage: go tool pprof http://localhost:6060/debug/pprof/profile
// For memory: go tool pprof http://localhost:6060/debug/pprof/heap

// Continuous performance benchmarking
func runContinuousBenchmarks() {
    ticker := time.NewTicker(1 * time.Hour)
    defer ticker.Stop()
    
    for {
        select {
        case <-ticker.C:
            // Run performance benchmarks
            results := runHarmonyBenchmarks()
            
            // Log performance metrics
            logger.Info("Harmony performance metrics: "+
                "avg_parse_time=%dms, "+
                "memory_usage=%dMB, "+
                "cache_hit_rate=%.2f%%", 
                results.AvgParseTime.Milliseconds(),
                results.MemoryUsage/(1024*1024),
                results.CacheHitRate*100)
            
            // Alert on performance degradation
            if results.AvgParseTime > 50*time.Millisecond {
                logger.Warn("Harmony performance degradation detected")
                // Trigger auto-optimization
                optimizePerformance(results)
            }
        }
    }
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