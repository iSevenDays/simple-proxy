# Simple Proxy Structured Logs Setup Guide

This guide shows you how to set up Grafana Loki to display structured log objects instead of raw JSON.

## Overview

**Before:** Raw JSON logs in Grafana  
**After:** Clean structured display with key-value pairs

### Example Log Display Transformation

**Raw JSON (Before):**
```json
{"timestamp":"2025-08-12T19:31:39.237+02:00","level":"info","component":"proxy_core","category":"request","message":"Claude Code Proxy starting","port":"3456","big_model_endpoints":1,"tool_correction_enabled":true}
```

**Structured Display (After):**
```
Claude Code Proxy starting | component=proxy_core | category=request | port=3456 | big_model_endpoints=1 | tool_correction_enabled=true
```

## Quick Setup

### 1. Start the Observability Stack

```bash
cd observability/
docker-compose up -d
```

This starts:
- **Loki** (port 3100) - Log aggregation 
- **Alloy** (port 12345) - Log processing and ingestion
- **Grafana** (port 3000) - Visualization

### 2. Verify Alloy Configuration

The enhanced `alloy-config.alloy` automatically:
- ✅ Parses logrus JSON logs
- ✅ Extracts all structured fields  
- ✅ Formats logs as readable key-value pairs
- ✅ Applies proper labels for filtering

### 3. Access Grafana

1. **Open:** http://localhost:3000
2. **Login:** admin / admin
3. **Navigate:** Dashboards → Simple Proxy - Structured Logs Dashboard

## Viewing Structured Logs

### Basic Queries

1. **All Logs (Structured Display):**
   ```logql
   {service="simple-proxy"}
   ```

2. **Filter by Component:**
   ```logql
   {component="circuit_breaker"}
   ```

3. **Filter by Log Level:**
   ```logql
   {level="error"}
   ```

4. **Request Tracing:**
   ```logql
   {request_id="req_123"}
   ```

### Sample Structured Log Outputs

#### Configuration Logs
```
Configured BIG_MODEL | component=configuration | category=request | model=claude-sonnet-4
```

#### Circuit Breaker Events  
```
Endpoint failure recorded | component=circuit_breaker | category=warning | endpoint=http://192.168.0.50:11434 | failures=2 | failure_threshold=3
```

#### Tool Corrections
```
Parameter correction applied | component=tool_correction | category=transformation | tool=Read | original_param=filename | corrected_param=file_path
```

#### Streaming Processing
```
Processing streaming response | component=proxy_core | category=request | request_id=req_456
```

## Dashboard Features

The included Grafana dashboard provides:

1. **Log Volume by Component** - Pie chart showing activity distribution
2. **Log Level Distribution** - Error/warning/info breakdown  
3. **Recent Error Logs** - Table of structured error events
4. **Circuit Breaker Events** - Health monitoring table
5. **Tool Correction Events** - Correction activity tracking
6. **All Structured Logs** - Complete structured log view

## Configuration Details

### Alloy Processing Pipeline

The `alloy-config.alloy` file contains:

```alloy
// Enhanced JSON log processing pipeline for logrus-based logs
loki.process "json_parser" {
    // Parse all logrus fields at root level
    stage.json {
        expressions = {
            timestamp = "timestamp",
            level = "level", 
            component = "component",
            message = "message",
            // ... all structured fields
        }
    }

    // Format as structured display
    stage.template {
        source = "structured_log"
        template = `{{ .message }}{{ if .component }} | component={{ .component }}{{ end }}{{ if .endpoint }} | endpoint={{ .endpoint }}{{ end }}...`
    }

    // Set efficient labels
    stage.labels {
        values = {
            level = "level",
            service = "service",
            component = "component", 
            category = "category",
        }
    }
}
```

### Log File Path

Logs are written to: `./observability/logs/simple-proxy.jsonl`

The Alloy agent monitors this path and automatically processes new log entries.

## Critical Configuration Points

### ⚠️ Log Directory Mapping
**CRITICAL:** Ensure Alloy mounts the correct log directory in `docker-compose.yml`:

```yaml
volumes:
  - ./logs:/var/log/simple-proxy:ro  # Must match where Simple Proxy writes JSON logs
```

**Issue:** If mapping is wrong, you'll see:
- ✅ Test log messages (manually added)
- ❌ No actual Simple Proxy logs

**Fix:** Simple Proxy writes logs to `observability/logs/simple-proxy.jsonl` by default, so use `./logs:/var/log/simple-proxy:ro`

### ⚠️ Duplicate Pipeline Conflict
**CRITICAL:** Only use ONE log processing pipeline in `alloy-config.alloy`:

❌ **Wrong - Multiple pipelines cause raw JSON:**
```alloy
// DON'T DO THIS - causes conflicts
loki.source.file "simple_proxy" {        // Text log pipeline
    targets = local.file_match.simple_proxy_logs.targets
    forward_to = [loki.process.simple_proxy_parser.receiver]
}
loki.source.file "simple_proxy_json" {   // JSON log pipeline  
    targets = local.file_match.simple_proxy_json_logs.targets
    forward_to = [loki.process.json_parser.receiver]
}
```

✅ **Correct - Single JSON pipeline:**
```alloy
// Only JSON processing for structured logs
loki.source.file "simple_proxy_json" {
    targets = local.file_match.simple_proxy_json_logs.targets
    forward_to = [loki.process.json_parser.receiver]
}
```

### ⚠️ Template Output Stage
**CRITICAL:** Only have ONE `stage.output` block in the JSON parser:

❌ **Wrong - Duplicate outputs cause raw JSON:**
```alloy
stage.output { source = "output" }        // Raw JSON output
stage.output { source = "structured_log" } // Formatted output - CONFLICT!
```

✅ **Correct - Single structured output:**
```alloy
stage.template {
    source = "structured_log"
    template = `{{ .message }}{{ if .request_id }} | request_id={{ .request_id }}{{ end }}...`
}
stage.output { source = "structured_log" } // Only this one!
```

## Troubleshooting

### 1. Logs Not Appearing (But Test Messages Work)

**Root Cause:** Log directory mismatch
```bash
# Check where Simple Proxy actually writes logs
ls -la observability/logs/simple-proxy.jsonl
tail -3 observability/logs/simple-proxy.jsonl

# Check Alloy volume mount in docker-compose.yml
grep -A3 "volumes:" observability/docker-compose.yml
```

**Fix:** Update docker-compose.yml volume mount to match actual log location

### 1a. New Logs Not Appearing in Loki (But File is Growing)

**Symptom:** Simple Proxy is generating new JSON logs, but they don't appear in Loki
**Root Cause:** Alloy persistent state is stuck at old file offset

**Diagnosis:**
```bash
# Check current log file size
wc -c observability/logs/simple-proxy.jsonl

# Check Alloy's current read position
docker compose logs alloy | grep "Seeked"
# Example output: Seeked /var/log/simple-proxy/simple-proxy.jsonl - &{Offset:437 Whence:0}

# If file size >> Offset, Alloy is stuck at old position
```

**Fix Options:**

**Option A - Proper Configuration (Recommended):**
```alloy
// Add tail_from_end to alloy-config.alloy
loki.source.file "simple_proxy_json" {
    targets      = local.file_match.simple_proxy_json_logs.targets
    forward_to   = [loki.process.json_parser.receiver]
    tail_from_end = true  // Prevents stuck offset issues
}
```
```bash
docker compose restart alloy
```

**Option B - Reset Position File:**
```bash
docker compose exec alloy sh -c "rm -f /var/lib/alloy/data/positions.yml"
docker compose restart alloy
```

**Option C - Nuclear Reset (Last Resort):**
```bash
docker compose down
docker volume rm observability_alloy-data
docker compose up -d
```

**Prevention:** This typically happens when:
- Alloy configuration changes while containers are running
- Log file paths change
- Container restarts don't clear persistent state properly

### 2. Seeing Raw JSON Line by Line

**Root Cause:** Multiple conflicting processing pipelines
```bash
# Check for duplicate pipelines in alloy-config.alloy
grep -n "loki.source.file" observability/alloy-config.alloy
grep -n "stage.output" observability/alloy-config.alloy
```

**Fix:** Remove text log pipeline, keep only JSON pipeline with single output stage

### 3. Still Seeing Raw JSON

**Check Alloy Configuration:**
- Ensure `alloy-config.alloy` has the enhanced `json_parser` stage
- Restart Alloy: `docker-compose restart alloy`

### 4. Missing Fields in Display

**Update Field Mapping:**
Add new fields to the `stage.json` expressions in `alloy-config.alloy`

### 5. Performance Issues

**Optimize Labels:**
- Keep label cardinality low (service, component, level, category)
- Use structured display template for detailed field viewing

## Advanced Usage

### Custom Display Formats

Create custom log formats using LogQL:

```logql
{service="simple-proxy"} | json | line_format "{{.timestamp}} [{{.level}}] {{.component}}: {{.message}}"
```

### Metrics from Logs

Extract metrics from structured fields:

```logql
{service="simple-proxy"} | json | unwrap response_time_ms | rate(5m)
```

### Complex Filtering

Combine multiple filters:

```logql
{component="proxy_core", level="info"} |= "streaming"
```

## Next Steps

1. **Start the stack:** `docker-compose up -d`
2. **Generate logs:** Run your Simple Proxy application  
3. **View in Grafana:** http://localhost:3000
4. **Explore queries:** See `grafana-queries.md` for more examples

Your logs should now display as clean, structured objects instead of raw JSON! 🎉