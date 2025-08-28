# Simple Proxy

A Claude Code proxy that transforms Anthropic API requests to OpenAI-compatible format.

## API Endpoints

- `GET /` - Service information and status
- `GET /health` - Health check endpoint  
- `POST /v1/messages` - Anthropic-compatible chat completions
- `GET /metrics` - Prometheus metrics endpoint

**Default Port**: 3456

## Observability

Simple-proxy sends structured logs directly to **Loki** via HTTP for real-time monitoring.

### Configuration

```bash
# Set Loki endpoint (default: http://localhost:3100)
export LOKI_URL="http://localhost:3100"

# Start simple-proxy with direct Loki logging
./simple-proxy
```

### Viewing Logs

- **Grafana**: http://localhost:3000 (admin/admin)
- **Loki API**: Query logs with `{job="simple-proxy"}`

### Log Format

Structured JSON logs include:
- `timestamp`, `level`, `message`
- `service="simple-proxy"`, `component`, `category`
- `request_id` for request tracing
- Custom fields for circuit breaker, tool correction, etc.

## Harmony Format Support

Simple Proxy automatically detects and parses **OpenAI Harmony format** content, providing structured access to thinking chains, analysis, and response content.

### What is Harmony Format?

Harmony format uses special tokens to organize AI responses into different channels:

```
<|start|>assistant,thinking  
Let me think through this step by step...
<|channel|>analysis
Here's my detailed analysis...
<|channel|>response
Here's my final answer for the user.
<|end|>
```

### Quick Start

Harmony parsing is **enabled by default**. No configuration needed for basic usage.

**Optional Configuration:**
```bash
# Enable/disable Harmony parsing (default: true)
export HARMONY_PARSING_ENABLED=true

# Enable debug logging for Harmony processing
export HARMONY_DEBUG=true

# Strict error handling for malformed content  
export HARMONY_STRICT_MODE=false
```

### Key Benefits

- **Automatic Detection**: No code changes required, works with existing clients
- **Clean Responses**: Users see consolidated response text, thinking chains are processed internally
- **Developer Access**: Full API access to thinking chains and structured content
- **Performance Optimized**: Fast regex-based detection with minimal overhead
- **Backward Compatible**: Non-Harmony content processed unchanged

### Example Response Processing

**Provider Response:**
```
<|start|>assistant,thinking
I need to explain this complex topic clearly...
<|channel|>response  
Quantum computing uses quantum mechanics principles...
<|end|>
```

**Client Receives:**
```json
{
  "content": "Quantum computing uses quantum mechanics principles..."
}
```

**Internal Processing Access:**
```go
// Developers can access full structure
message, _ := parser.ParseHarmonyMessage(content)
thinkingChains := message.GetThinkingChannels()
response := message.ConsolidatedText
```

### Documentation

See [docs/harmony-support.md](docs/harmony-support.md) for:
- Complete API reference
- Integration examples  
- Performance characteristics
- Troubleshooting guide
- Advanced usage patterns