# Grafana LogQL Queries for Simple Proxy

> 📋 **Log Schema Reference**: See [ARCHITECTURE.md](./ARCHITECTURE.md#data-architecture) for complete field documentation.

## Structured Log Display Queries

With Alloy processing, logs display as structured key-value pairs:

### 1. Basic Structured Logs (Default View)
```logql
{service="simple-proxy"}
```
*Shows: "Claude Code Proxy starting | component=proxy_core | category=request | port=3456 | big_model_endpoints=1 | tool_correction_enabled=true"*

### 2. Filter by Component
```logql
{component="circuit_breaker"}
```
*Shows: "Circuit breaker opened for endpoint | component=circuit_breaker | category=error | endpoint=http://192.168.0.46:11434 | failures=3"*

### 3. Filter by Log Level
```logql
{level="error"}
```

### 4. Request Tracing
```logql
{request_id="req_123"}
```
*Shows all logs for a specific request with structured context*

### 5. Component-Specific Queries

#### Hybrid Classifier Analysis
```logql
{component="hybrid_classifier"}
```
*Shows complete decision pipeline with stages A, B, C*

#### Hybrid Classifier by Stage
```logql
{component="hybrid_classifier", stage="A_extract_pairs"}
{component="hybrid_classifier", stage="B_rule_engine"} 
{component="hybrid_classifier", stage="C_llm_fallback"}
```

#### Rule Engine Decisions
```logql
{component="hybrid_classifier", rule_name="StrongVerbWithFile"}
{component="hybrid_classifier", matched="true"}
```

#### LLM Fallback Analysis
```logql
{component="hybrid_classifier", stage="C_llm_fallback"} |= "full_prompt"
```

#### Circuit Breaker Health
```logql
{component="circuit_breaker"}
```
*Shows: "Endpoint failure recorded | component=circuit_breaker | endpoint=http://192.168.0.50:11434 | failures=2 | failure_threshold=3"*

#### Tool Corrections
```logql
{component="tool_correction"}
```
*Shows: "Parameter correction applied | component=tool_correction | tool=Read | original_param=filename | corrected_param=file_path"*

#### Streaming Processing
```logql
{component="proxy_core", category="request"} |= "streaming"
```
*Shows: "Processing streaming response | component=proxy_core | request_id=req_456 | chunks=15 | tool_calls=2"*

### 6. Performance Metrics

#### Response Times (requires JSON parsing for metrics)
```logql
{service="simple-proxy"} | json | unwrap response_time_ms | rate(5m)
```

#### Error Rates by Component
```logql
sum by (component) (rate({level="error"}[5m]))
```

#### Request Volume
```logql
sum by (component) (rate({service="simple-proxy"}[5m]))
```

### 7. Advanced Filtering

#### Successful Operations
```logql
{category="success"}
```

#### Warning Events
```logql
{level="warn"}
```

#### Tool-Related Events
```logql
{service="simple-proxy"} | json | tool_name != ""
```

#### Endpoint Health Issues
```logql
{component="circuit_breaker"} |= "failure"
```

### 8. Custom Display Formats

#### Custom Message Format (if you want different display)
```logql
{service="simple-proxy"} | json | line_format "{{.timestamp}} [{{.level}}] {{.component}}: {{.message}}"
```

#### Request Summary
```logql
{service="simple-proxy"} | json | line_format "{{.message}} ({{.component}}/{{.category}})"
```

## Log Display Format

### Structured Display (Default in Grafana)
With the enhanced Alloy configuration, logs now display as structured key-value pairs instead of raw JSON:

**Before (Raw JSON):**
```
{"timestamp":"2025-08-12T19:31:39.237+02:00","level":"info","component":"proxy_core","category":"request","message":"Claude Code Proxy starting","port":"3456","big_model_endpoints":1,"tool_correction_enabled":true}
```

**After (Structured Display):**
```
Claude Code Proxy starting | component=proxy_core | category=request | port=3456 | big_model_endpoints=1 | tool_correction_enabled=true
```

### Available Structured Fields

All fields from logrus JSON are automatically parsed and displayed in pipe-separated format:

- `timestamp` - Log timestamp
- `level` - Log level (info, warn, error)
- `message` - Log message
- `component` - Architecture component
- `category` - Event category
- `request_id` - Request correlation ID
- `endpoint` - API endpoint
- `tool_name` - Tool name for corrections
- `decision` - Hybrid classifier decision
- `reason` - Decision reasoning
- `confident` - Decision confidence
- `port` - Server port
- `model` - AI model name
- `tokens` - Token count
- `error` - Error details
- And many more...

## Notes

- All queries use `| json` to parse the JSON log structure
- Fields are directly accessible (no nested `fields` object)
- Use `| line_format` to create custom display formats
- Use `| unwrap` for numeric field analysis

## Fixed Issues

✅ **JSON Field Parsing**: Replaced custom logger with logrus for proper JSON structure
✅ **Simplified Architecture**: Removed complex nested `fields` structure  
✅ **Standard Library**: Using industry-standard logrus instead of custom code
✅ **Flat JSON Structure**: All fields at root level for easier parsing
✅ **Grafana Compatible**: Standard LogQL `| json` parsing works correctly