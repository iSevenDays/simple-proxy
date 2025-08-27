# Simple Proxy Logging Architecture

## Current Implementation: Direct HTTP to Loki

```
Simple Proxy → HTTP POST → Loki → Grafana
```

### Key Benefits

✅ **Immediate logs** - No file intermediary, instant Loki ingestion  
✅ **Better performance** - No disk I/O, async HTTP delivery  
✅ **Simplified stack** - Eliminated Alloy file reader complexity  
✅ **Production ready** - Graceful fallback to stdout if Loki unavailable  

### Configuration

```go
// Environment variable
LOKI_URL="http://localhost:3100"  // Default value

// Log job label in Loki
job="simple-proxy"
```

### Log Structure

```json
{
  "timestamp": "2025-08-26T21:15:30.169845+02:00",
  "level": "INFO", 
  "message": "Claude Code Proxy started",
  "service": "simple-proxy",
  "component": "proxy_core",
  "category": "request",
  "request_id": "req-123",
  // ... additional fields
}
```

### Querying in Grafana

```logql
# All simple-proxy logs
{job="simple-proxy"}

# Specific component
{job="simple-proxy", component="circuit_breaker"}

# Request tracing
{job="simple-proxy", request_id="req-123"}

# Error logs only  
{job="simple-proxy", level="ERROR"}
```

## Previous Implementation (Deprecated)

~~Simple Proxy → .jsonl files → Alloy → Loki → Grafana~~

**Removed due to:**
- File I/O bottlenecks
- Alloy offset management complexity  
- Delayed log availability
- Additional failure points