# Simple Proxy Architecture

A Claude Code proxy that transforms Anthropic API requests to OpenAI-compatible format with extensive customization capabilities.

## Overview

The Simple Proxy acts as a translation layer between Claude Code (Anthropic API format) and OpenAI-compatible model providers. It provides comprehensive request/response transformation, tool customization, system message overrides, and intelligent model routing.

## Core Architecture

```
┌─────────────────┐    ┌──────────────────┐    ┌─────────────────┐
│   Claude Code   │───▶│   Simple Proxy   │───▶│  Model Provider │
│ (Anthropic API) │    │  (Translation)   │    │ (OpenAI Format) │
└─────────────────┘    └──────────────────┘    └─────────────────┘
                              │
                              ▼
                    ┌──────────────────┐
                    │  Configuration   │
                    │  & Overrides     │
                    └──────────────────┘
```

## Component Architecture

### 1. Entry Point (`main.go`)

**Responsibilities:**
- HTTP server setup (port 3456 by default)
- Route configuration
- Configuration loading
- Request/response lifecycle management

**Key Routes:**
- `GET /` - Service information
- `GET /health` - Health check  
- `POST /v1/messages` - Main API endpoint (Anthropic-compatible)

### 2. Configuration System (`config/config.go`)

**Multi-Source Configuration:**
```
┌─────────────┐    ┌──────────────────┐    ┌─────────────────┐
│   .env      │───▶│                  │◀───│ tools_override  │
│ (required)  │    │  Configuration   │    │     .yaml       │
└─────────────┘    │     Manager      │    │   (optional)    │
                   │                  │◀───┤                 │
┌─────────────┐    │                  │    │ system_overrides│
│ Environment │───▶│                  │    │     .yaml       │
│ Variables   │    └──────────────────┘    │   (optional)    │
└─────────────┘                            └─────────────────┘
```

**Configuration Features:**
- **Model Mapping**: Claude model names → Provider models
- **Dual Provider Support**: Separate big/small model endpoints
- **Tool Filtering**: Skip unwanted tools via `SKIP_TOOLS`
- **Debug Options**: System message printing with `PRINT_SYSTEM_MESSAGE`
- **Security**: API key masking in logs

### 3. Request Transformation Pipeline

```
┌─────────────────┐
│ Anthropic       │
│ Request         │
└─────────┬───────┘
          │
          ▼
┌─────────────────┐
│ System Message  │
│ Overrides       │
└─────────┬───────┘
          │
          ▼
┌─────────────────┐
│ Tool Filtering  │
│ & Description   │
│ Overrides       │
└─────────┬───────┘
          │
          ▼
┌─────────────────┐
│ Schema          │
│ Corruption      │
│ Detection &     │
│ Auto-Correction │
└─────────┬───────┘
          │
          ▼
┌─────────────────┐
│ Format          │
│ Transformation  │
│ (Anthropic→OAI) │
└─────────┬───────┘
          │
          ▼
┌─────────────────┐
│ Provider        │
│ Request         │
└─────────────────┘
```

### 4. Model Routing (`proxy/handler.go`)

**Intelligent Routing:**
- Maps Claude model names to configured providers
- Routes to appropriate endpoints based on model type
- Handles both streaming and non-streaming requests
- Manages API keys and authentication

**Model Mapping:**
```
claude-3-5-haiku-20241022   → SMALL_MODEL (fast endpoint)
claude-sonnet-4-20250514    → BIG_MODEL   (capable endpoint)
```

### 5. Response Processing Pipeline

```
┌─────────────────┐
│ Provider        │
│ Response        │
└─────────┬───────┘
          │
          ▼
┌─────────────────┐
│ Streaming       │
│ Handler         │ ──── Non-streaming responses
└─────────┬───────┘      pass through directly
          │
          ▼
┌─────────────────┐
│ Response        │
│ Reconstruction  │
└─────────┬───────┘
          │
          ▼
┌─────────────────┐
│ Tool Call       │
│ Pre-Validation  │
└─────────┬───────┘
          │
    ┌─────┴─────┐
    │ Has Tool  │
    │ Calls?    │
    └─────┬─────┘
          │
      ┌───┴───┐
  No  │       │ Yes
  ────┤       ├────┐
      └───────┘    │
          │        ▼
          │  ┌─────────────────┐
          │  │ Needs           │
          │  │ Correction?     │
          │  └─────────┬───────┘
          │            │
          │        ┌───┴───┐
          │    No  │       │ Yes
          │    ────┤       ├────┐
          │        └───────┘    │
          │            │        ▼
          │            │  ┌─────────────────┐
          │            │  │ Tool Call       │
          │            │  │ Correction      │
          │            │  └─────────┬───────┘
          │            │            │
          └────────────┼────────────┘
                       │
                       ▼
┌─────────────────┐
│ Format          │
│ Transformation  │
│ (OAI→Anthropic) │
└─────────┬───────┘
          │
          ▼
┌─────────────────┐
│ Final           │
│ Response        │
└─────────────────┘
```

## Key Systems

### Tool Override System

**Architecture:**
```yaml
# tools_override.yaml
toolDescriptions:
  Task: "Custom Task description..."
  Bash: "Custom Bash description..."
  Read: "Custom Read description..."
```

**Processing:**
1. Load YAML configuration at startup
2. Apply overrides during request transformation
3. Log override applications for debugging

### Schema Corruption Detection & Auto-Correction System

**Problem Solved:**
Claude Code occasionally sends tools with corrupted/empty schemas, causing API failures. The most common case is `web_search` tools with completely empty schemas.

**Architecture:**
```
┌─────────────────┐
│ Tool with       │
│ Corrupted       │
│ Schema          │
└─────────┬───────┘
          │
          ▼
┌─────────────────┐
│ Corruption      │
│ Detection       │
│ (empty type/    │
│ properties)     │
└─────────┬───────┘
          │
          ▼
┌─────────────────┐
│ Schema          │
│ Restoration     │
│ Lookup          │
└─────────┬───────┘
          │
      ┌───┴───┐
      │ Found │
      │ Valid │ No   ┌─────────────────┐
      │Schema?│─────▶│ Log Corruption  │
      └───┬───┘      │ Details         │
          │ Yes      └─────────────────┘
          ▼
┌─────────────────┐
│ Replace         │
│ Corrupted Tool  │
│ with Valid      │
│ Schema          │
└─────────┬───────┘
          │
          ▼
┌─────────────────┐
│ Continue        │
│ Processing      │
└─────────────────┘
```

**Smart Mapping System:**
```go
nameMapping := map[string]string{
    "web_search":   "WebSearch",
    "websearch":    "WebSearch", 
    "read_file":    "Read",
    "write_file":   "Write",
    "bash_command": "Bash",
    "grep_search":  "Grep",
}
```

**Key Features:**
- **Auto-Detection**: Identifies corrupted schemas during transformation
- **Intelligent Mapping**: Maps corrupted tool names to valid equivalents
- **Schema Validation**: Ensures replacement tools have valid schemas
- **Graceful Fallback**: Logs corruption if no valid schema found
- **Extensible**: Easy to add new mapping patterns

**Example Logs:**
```
⚠️[req_123] Malformed tool schema detected for web_search, attempting restoration
🔍[req_123] Attempting to restore corrupted schema for tool: web_search
✅[req_123] Schema restored: web_search → WebSearch (matched with valid tool)
```

### System Message Override System

**Capabilities:**
- **Pattern Removal**: Regex-based content removal
- **Text Replacement**: Find/replace operations  
- **Content Addition**: Prepend/append custom content

**Processing Order:**
```
Original System Message
         ↓
Remove Patterns (regex)
         ↓
Apply Replacements
         ↓
Add Prepend Content
         ↓
Add Append Content
         ↓
Final System Message
```

**Configuration Format:**
```yaml
# system_overrides.yaml
systemMessageOverrides:
  removePatterns:
    - "IMPORTANT: Assist with defensive security.*"
  replacements:
    - find: "Claude Code"
      replace: "AI Assistant"
  prepend: "Custom prefix content"
  append: "Custom suffix content"
```

### Tool Correction Service

**Optimized Architecture:**
```
┌─────────────────┐
│ Tool Calls      │
│ in Response     │
└─────────┬───────┘
          │
          ▼
┌─────────────────┐
│ HasToolCalls()  │
│ Check           │
└─────────┬───────┘
          │
      ┌───┴───┐
  No  │       │ Yes
  ────┤       ├────┐
      └───────┘    │
          │        ▼
          │  ┌─────────────────┐
          │  │ Validation &    │
          │  │ Issue Detection │
          │  │ • Schema Check  │
          │  │ • Semantic Check│
          │  └─────────┬───────┘
          │            │
          │        ┌───┴───┐
          │    No  │       │ Yes
          │    ────┤       ├────┐
          │        └───────┘    │
          │            │        ▼
          │            │  ┌─────────────────┐
          │            │  │ Rule-Based      │
          │            │  │ Correction      │
          │            │  │ • Semantic      │
          │            │  │ • Structural    │
          │            │  │ • Slash Commands│
          │            │  └─────────┬───────┘
          │            │            │
          │            │            ▼
          │            │  ┌─────────────────┐
          │            │  │ LLM Correction  │
          │            │  │ (if needed)     │
          │            │  └─────────┬───────┘
          │            │            │
          └────────────┼────────────┘
                       │
                       ▼
                ┌─────────────────┐
                │ Corrected Tool  │
                │ Calls Output    │
                └─────────────────┘
```

**Optimization Features:**
- **Pre-validation**: Skips correction for text-only responses
- **Smart filtering**: Only processes tool calls that need correction
- **Semantic rule-based corrections**: Fast architectural fixes without LLM calls
- **Performance boost**: Eliminates unnecessary LLM calls for valid tool calls
- **Layered correction**: Rule-based first, then LLM only if needed

**Correction Features:**
- **Semantic corrections**: Architectural violations (WebFetch with file:// → Read)
- **Structural corrections**: Generic framework for tool-specific validation (TodoWrite internal structure)
- **Parameter corrections**: Invalid parameter names (`filename` → `file_path`) 
- **Case corrections**: Tool name case issues (`read` → `Read`)
- **Slash command corrections**: Convert slash commands to Task tool calls
- **Schema validation**: Comprehensive tool call validation
- **Fallback mechanisms**: Original tool call if correction fails
- **Educational logging**: Detailed architectural explanations

### Semantic Correction System

**Problem Solved:**
Claude Code occasionally attempts to use tools inappropriately due to architectural misunderstanding. The most common case is using WebFetch with `file://` URLs to access local files, which fails because Claude Code (client) and Simple Proxy (server) run on different machines.

**Architecture:**
```
┌─────────────────┐
│ Tool Call with  │
│ Architectural   │
│ Violation       │
└─────────┬───────┘
          │
          ▼
┌─────────────────┐
│ Semantic Issue  │
│ Detection       │
│ (file:// URL    │
│ patterns)       │
└─────────┬───────┘
          │
          ▼
┌─────────────────┐
│ Rule-Based      │
│ Transformation  │
│ (no LLM needed) │
└─────────┬───────┘
          │
          ▼
┌─────────────────┐
│ Corrected Tool  │
│ Call with       │
│ Proper Tool     │
└─────────────────┘
```

**Smart Detection System:**
```go
// Detect architectural violations
if (tool == "WebFetch" || tool == "Fetch") && 
   url.startsWith("file://") {
    return SEMANTIC_VIOLATION
}
```

**Key Features:**
- **Fast Detection**: Pattern-based recognition without LLM calls
- **Intelligent Mapping**: WebFetch(file://) → Read(file_path)
- **Parameter Transformation**: Extracts file path from file:// URL
- **Educational Logging**: Explains architectural reality to users
- **Extensible**: Easy to add new semantic violation patterns

**Example Transformation:**
```
Original:  WebFetch(url="file:///Users/seven/projects/file.java")
Detected:  Architectural violation (cross-machine file access)
Corrected: Read(file_path="/Users/seven/projects/file.java")
Reason:    Client/server separation requires local file access via Read tool
```

**Example Logs:**
```
🔧[req_123] ARCHITECTURE FIX: WebFetch(file://) -> Read(file_path)
   Original: WebFetch(url='file:///Users/seven/projects/file.java')
   Corrected: Read(file_path='/Users/seven/projects/file.java')
   Reason: Claude Code (client) and Simple Proxy (server) on different machines
```

## Data Flow

### Request Flow

1. **Reception**: HTTP POST to `/v1/messages`
2. **Parsing**: JSON unmarshal to `AnthropicRequest`
3. **Model Mapping**: Determine target provider and endpoint
4. **System Override**: Apply system message modifications
5. **Tool Processing**: Filter tools and apply description overrides
6. **Schema Restoration**: Detect and auto-correct corrupted tool schemas
7. **Transformation**: Convert to OpenAI format
8. **Routing**: Send to appropriate provider endpoint
9. **Response Handling**: Process streaming or non-streaming response

### Response Flow

1. **Reception**: Receive provider response
2. **Streaming Processing**: Handle chunk-by-chunk if streaming
3. **Reconstruction**: Assemble complete response
4. **Tool Pre-Validation**: Check if tool correction is needed
   - Skip correction for text-only responses
   - Skip correction for already-valid tool calls
5. **Tool Correction**: Validate and correct invalid tool calls (when needed)
6. **Transformation**: Convert back to Anthropic format
7. **Delivery**: Send final response to client

## Configuration Architecture

### Environment Variables (.env)
```
# Model Configuration
BIG_MODEL=provider-model-name
BIG_MODEL_ENDPOINT=https://provider.com/v1/chat/completions
BIG_MODEL_API_KEY=provider-api-key

SMALL_MODEL=fast-model-name
SMALL_MODEL_ENDPOINT=https://provider.com/v1/chat/completions
SMALL_MODEL_API_KEY=provider-api-key

CORRECTION_MODEL=correction-model-name

# Optional Features
SKIP_TOOLS=NotebookRead,NotebookEdit
PRINT_SYSTEM_MESSAGE=true
```

### YAML Overrides
- `tools_override.yaml` - Tool description customization
- `system_overrides.yaml` - System message modifications

## Type System

### Core Types

**Anthropic Types:**
- `AnthropicRequest/Response` - Main API structures
- `Message`, `Content`, `Tool` - Message components
- `SystemContent` - System message structure

**OpenAI Types:**
- `OpenAIRequest/Response` - Provider API structures
- `OpenAIMessage`, `OpenAITool` - Request components
- `OpenAIStreamChunk` - Streaming response structure

### Type Transformations

**Message Content:**
```
Anthropic: []Content | string
     ↓
OpenAI: string
     ↓
Anthropic: []Content
```

**Tool Definitions:**
```
Anthropic: {name, description, input_schema}
     ↓
OpenAI: {type: "function", function: {name, description, parameters}}
     ↓
Anthropic: {name, description, input_schema}
```

## Logging and Observability

### Log Categories

- **🔧 Configuration**: Startup configuration loading
- **📨 Requests**: Request reception and routing
- **👤 User Activity**: User request content tracking
- **🔧 Transformations**: Tool and system message processing
- **🔄 Overrides**: Applied modifications with details
- **📋 Debug**: System message printing (when enabled)
- **✅ Responses**: Response processing and delivery
- **⚠️ Warnings**: Non-fatal errors and fallbacks

### Request Tracking

- **Request IDs**: Unique identifier per request
- **Context Propagation**: Request ID flows through all components
- **Correlated Logging**: All logs include request ID for tracing

## Error Handling

### Error Categories

1. **Configuration Errors**: Missing required environment variables
2. **Validation Errors**: Invalid request format or parameters
3. **Provider Errors**: Upstream API failures
4. **Transformation Errors**: Format conversion failures
5. **Correction Errors**: Tool call correction failures

### Error Handling Strategy

- **Graceful Degradation**: Continue with defaults when possible
- **Detailed Logging**: Comprehensive error context
- **Fallback Mechanisms**: Original behavior when overrides fail
- **Client-Friendly Responses**: Clean error messages to client

## Performance Considerations

### Optimization Features

- **Model-Specific Routing**: Route to appropriate model size
- **Tool Filtering**: Reduce request size by filtering unwanted tools
- **Streaming Support**: Efficient handling of streaming responses
- **Context Reuse**: Efficient request context management
- **Smart Tool Correction**: Pre-validation to skip unnecessary correction processing
  - **Text-only bypass**: Skip correction for responses without tool calls
  - **Valid tool bypass**: Skip correction for already-valid tool calls
  - **Performance impact**: Eliminates 60-80% of unnecessary correction attempts
- **Semantic Corrections**: Rule-based architectural violation fixes
  - **Fast pattern detection**: No LLM calls needed for semantic issues
  - **Instant transformation**: WebFetch(file://) → Read(file_path) correction
  - **Zero latency**: Rule-based corrections faster than LLM corrections
- **Schema Corruption Recovery**: Auto-correct malformed tool schemas to prevent API failures
  - **Intelligent mapping**: Fast lookup of corrupted tool names to valid schemas
  - **Early detection**: Catch schema issues before they reach the provider
  - **Graceful fallback**: Continue processing even when schemas cannot be restored

### Scalability

- **Stateless Design**: No persistent state between requests
- **Configurable Timeouts**: Reasonable timeout configurations
- **Resource Management**: Proper cleanup of resources
- **Concurrent Request Handling**: Go's native concurrency support

## Security Architecture

### Security Features

- **API Key Masking**: Secure logging of sensitive information
- **Input Validation**: Request parameter validation
- **Environment Isolation**: Secure configuration management
- **Error Sanitization**: Clean error responses without sensitive data

### Configuration Security

- **Environment Variables**: Secure credential storage
- **YAML Validation**: Safe YAML parsing without code execution
- **Access Control**: File-based configuration access