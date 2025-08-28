# Harmony Format Support

OpenAI Harmony format parsing support for enhanced message structure and thinking chains in Simple Proxy.

## Overview

Harmony format is OpenAI's structured message format that uses special tokens to organize content into different channels like thinking, analysis, and responses. Simple Proxy automatically detects and parses Harmony format content, providing structured access to different content types.

## Format Structure

Harmony format uses these token patterns:

```
<|start|>role,channel_type
Content for this channel
<|channel|>channel_type
Additional content
<|message|>
Final message content
<|end|>
```

### Supported Tokens

- `<|start|>role,channel_type` - Begin a content block with role and channel
- `<|channel|>channel_type` - Switch to a different channel within the same block  
- `<|message|>` - Mark the final response content
- `<|end|>` - End the content block

### Channel Types

- **thinking** - Internal reasoning and analysis
- **analysis** - Structured analytical content
- **response** - Final user-facing response
- **debug** - Debug information and diagnostics

## Configuration

### Environment Variables

```bash
# Enable/disable Harmony parsing (default: true)
HARMONY_PARSING_ENABLED=true

# Enable detailed Harmony debug logging (default: false) 
HARMONY_DEBUG=true

# Strict error handling for malformed content (default: false)
HARMONY_STRICT_MODE=false
```

### Quick Start

1. **Enable Harmony parsing** (enabled by default):
   ```bash
   export HARMONY_PARSING_ENABLED=true
   ```

2. **Start the proxy**:
   ```bash
   ./simple-proxy
   ```

3. **Send requests** - Harmony format is automatically detected and parsed when present in responses.

## Usage Examples

### Basic Harmony Content Detection

```go
import "github.com/iSevenDays/simple-proxy/parser"

content := `<|start|>assistant,thinking
Let me analyze this request carefully...
<|channel|>response  
Here's my response to the user.
<|end|>`

// Quick detection
isHarmony := parser.IsHarmonyFormat(content)
// Returns: true

// Full parsing
message, err := parser.ParseHarmonyMessage(content)
if err != nil {
    log.Printf("Parse error: %v", err)
}

if message.HasHarmony {
    fmt.Printf("Found %d channels\n", len(message.Channels))
    fmt.Printf("Consolidated text: %s\n", message.ConsolidatedText)
}
```

### Channel-Specific Content Access

```go
message, _ := parser.ParseHarmonyMessage(content)

// Get thinking channels only
thinkingChannels := message.GetThinkingChannels()
for _, channel := range thinkingChannels {
    fmt.Printf("Thinking: %s\n", channel.Content)
}

// Get response channels only  
responseChannels := message.GetResponseChannels()
for _, channel := range responseChannels {
    fmt.Printf("Response: %s\n", channel.Content)
}

// Get channels by specific type
analysisChannels := message.GetChannelsByType(parser.ChannelAnalysis)
```

### Request/Response Flow Example

**Request to Provider:**
```json
{
  "model": "gpt-4",
  "messages": [
    {"role": "user", "content": "Explain quantum computing"}
  ]
}
```

**Provider Response (with Harmony):**
```
<|start|>assistant,thinking
This is a complex topic. I should break it down into:
1. Basic concepts
2. Key principles  
3. Applications
<|channel|>analysis
Quantum computing leverages quantum mechanical phenomena...
<|channel|>response
Quantum computing is a revolutionary approach to computation...
<|end|>
```

**Parsed Structure:**
```json
{
  "has_harmony": true,
  "role": "assistant", 
  "channels": [
    {
      "type": "thinking",
      "content": "This is a complex topic. I should break it down into:\n1. Basic concepts\n2. Key principles\n3. Applications"
    },
    {
      "type": "analysis", 
      "content": "Quantum computing leverages quantum mechanical phenomena..."
    },
    {
      "type": "response",
      "content": "Quantum computing is a revolutionary approach to computation..."
    }
  ],
  "consolidated_text": "Quantum computing is a revolutionary approach to computation..."
}
```

## Integration Examples

### Custom Response Processing

```go
func processHarmonyResponse(content string) (*ProcessedResponse, error) {
    message, err := parser.ParseHarmonyMessage(content)
    if err != nil {
        return nil, fmt.Errorf("harmony parsing failed: %w", err)
    }
    
    if !message.HasHarmony {
        // Handle non-Harmony content
        return &ProcessedResponse{
            Content: content,
            HasThinking: false,
        }, nil
    }
    
    // Extract thinking for internal use
    thinking := message.GetThinkingChannels()
    thinkingText := ""
    for _, ch := range thinking {
        thinkingText += ch.Content + "\n"
    }
    
    return &ProcessedResponse{
        Content: message.ConsolidatedText,
        ThinkingChain: thinkingText,
        HasThinking: len(thinking) > 0,
        ChannelCount: len(message.Channels),
    }, nil
}
```

### Middleware Integration

```go
func harmonyParsingMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // Capture response
        recorder := &responseRecorder{ResponseWriter: w}
        next.ServeHTTP(recorder, r)
        
        // Parse if Harmony format detected
        if parser.IsHarmonyFormat(recorder.body) {
            message, err := parser.ParseHarmonyMessage(recorder.body)
            if err != nil {
                // Log error but continue with original content
                log.Printf("Harmony parsing error: %v", err)
                w.Write(recorder.body)
                return
            }
            
            // Add Harmony metadata headers
            w.Header().Set("X-Harmony-Detected", "true")
            w.Header().Set("X-Harmony-Channels", fmt.Sprintf("%d", len(message.Channels)))
            
            // Return consolidated text for clean client experience
            w.Write([]byte(message.ConsolidatedText))
            return
        }
        
        // Non-Harmony content - pass through unchanged
        w.Write(recorder.body)
    })
}
```

### Development and Debugging

```go
// Enable detailed debugging
os.Setenv("HARMONY_DEBUG", "true")

// Get comprehensive token statistics  
stats := parser.GetHarmonyTokenStats(content)
fmt.Printf("Token stats: %+v\n", stats)

// Validate structure before processing
errors := parser.ValidateHarmonyStructure(content)
if len(errors) > 0 {
    for _, err := range errors {
        log.Printf("Validation error: %v", err)
    }
}

// Find all token positions for debugging
positions := parser.FindHarmonyTokens(content)
for _, pos := range positions {
    fmt.Printf("Token %s at position %d\n", pos.Type, pos.Position)
}
```

## API Reference

### Core Functions

#### `IsHarmonyFormat(content string) bool`
Fast detection of Harmony format tokens in content.

#### `ParseHarmonyMessage(content string) (*HarmonyMessage, error)`  
Complete parsing of Harmony content into structured format.

#### `ExtractChannels(content string) []Channel`
Extract all valid Harmony channels from content.

### Types

#### `HarmonyMessage`
```go
type HarmonyMessage struct {
    Role             Role      `json:"role"`
    Channels         []Channel `json:"channels"`
    ConsolidatedText string    `json:"consolidated_text"`
    HasHarmony       bool      `json:"has_harmony"`
    ParsedAt         time.Time `json:"parsed_at"`
}
```

#### `Channel`
```go
type Channel struct {
    Type        ChannelType `json:"type"`
    Content     string      `json:"content"`
    Position    int         `json:"position"`
    ContentType ContentType `json:"content_type"`
}
```

#### `ChannelType`
- `ChannelThinking` - Internal reasoning
- `ChannelAnalysis` - Structured analysis  
- `ChannelResponse` - Final response
- `ChannelDebug` - Debug information

## Performance Characteristics

### Parsing Performance
- **Detection**: ~0.1ms for typical responses (regex-based)
- **Full parsing**: ~1-5ms depending on content complexity
- **Memory usage**: Minimal overhead, content is parsed in-place

### Scaling Considerations
- Parsing is stateless and thread-safe
- No external dependencies or network calls
- Suitable for high-throughput production environments
- Memory usage scales linearly with content size

### Optimization Tips
1. Use `IsHarmonyFormat()` for quick detection before full parsing
2. Cache parsed results if processing the same content multiple times
3. Enable `HARMONY_DEBUG=false` in production for optimal performance
4. Consider `HARMONY_STRICT_MODE=false` for better error tolerance

## Troubleshooting

### Common Issues

#### Harmony Format Not Detected
```bash
# Check if parsing is enabled
echo $HARMONY_PARSING_ENABLED

# Enable debug logging
export HARMONY_DEBUG=true

# Look for format issues
./simple-proxy 2>&1 | grep harmony
```

#### Malformed Harmony Content
```go
// Validate before parsing
errors := parser.ValidateHarmonyStructure(content)
if len(errors) > 0 {
    // Handle validation errors
    for _, err := range errors {
        log.Printf("Validation error: %v", err) 
    }
}
```

#### Performance Issues
```go
// Use quick detection first
if !parser.IsHarmonyFormat(content) {
    // Skip parsing for non-Harmony content
    return processRegularContent(content)
}

// Only parse when needed
message, err := parser.ParseHarmonyMessage(content)
```

### Debug Logging

When `HARMONY_DEBUG=true` is enabled, look for these log patterns:

```
harmony detected in response, parsing...
harmony parsing completed successfully, channels: 3
harmony format not detected, skipping parse
harmony parsing error: mismatched tokens at position 156
```

### Error Handling

#### Parse Errors
```go
message, err := parser.ParseHarmonyMessage(content)
if err != nil {
    if harmonyErr, ok := err.(*parser.HarmonyParseError); ok {
        log.Printf("Harmony parse error at position %d: %s", 
            harmonyErr.Position, harmonyErr.Message)
    }
    // Fallback to original content
    return content
}
```

#### Graceful Degradation
```go
// Always provide fallback behavior
func safeHarmonyParse(content string) string {
    message, err := parser.ParseHarmonyMessage(content)
    if err != nil || !message.HasHarmony {
        return content // Return original on any error
    }
    return message.ConsolidatedText
}
```

## Advanced Usage

### Custom Channel Handlers

```go
func processChannelsByType(message *parser.HarmonyMessage) map[string]string {
    result := make(map[string]string)
    
    // Process thinking chains
    thinking := message.GetThinkingChannels()
    if len(thinking) > 0 {
        var thoughts []string
        for _, ch := range thinking {
            thoughts = append(thoughts, ch.Content)
        }
        result["thinking"] = strings.Join(thoughts, "\n---\n")
    }
    
    // Extract analysis sections
    analysis := message.GetChannelsByType(parser.ChannelAnalysis)
    if len(analysis) > 0 {
        var analyses []string
        for _, ch := range analysis {
            analyses = append(analyses, ch.Content)
        }
        result["analysis"] = strings.Join(analyses, "\n\n")
    }
    
    return result
}
```

### Token Statistics Monitoring

```go
func monitorHarmonyUsage(content string) {
    if !parser.IsHarmonyFormat(content) {
        return
    }
    
    stats := parser.GetHarmonyTokenStats(content)
    
    // Log metrics for monitoring
    log.Printf("Harmony usage - Channels: %d, Tokens: %d, Avg content: %.1f chars", 
        stats.ChannelCount, 
        stats.TotalTokens,
        float64(stats.ContentLength)/float64(stats.ChannelCount))
}
```

## Migration Guide

### Updating from Non-Harmony Implementations

1. **Enable feature flag**:
   ```bash
   export HARMONY_PARSING_ENABLED=true
   ```

2. **Update response handling**:
   ```go
   // Before
   response := string(body)
   
   // After  
   message, err := parser.ParseHarmonyMessage(string(body))
   if err != nil || !message.HasHarmony {
       response = string(body) // Fallback
   } else {
       response = message.ConsolidatedText
   }
   ```

3. **Add error handling**:
   ```go
   if config.HarmonyStrictMode && err != nil {
       return fmt.Errorf("harmony parsing required but failed: %w", err)
   }
   ```

### Backward Compatibility

- Non-Harmony content is processed unchanged
- Feature can be disabled via `HARMONY_PARSING_ENABLED=false`
- Graceful degradation on parsing errors (unless strict mode enabled)
- No changes required to existing client code