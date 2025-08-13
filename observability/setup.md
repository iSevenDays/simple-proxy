# Simple Proxy Observability Quick Setup

> 📋 **Architecture Overview**: See [ARCHITECTURE.md](./ARCHITECTURE.md) for complete system design and component details.

Modern observability stack with structured log processing:
- **Grafana Alloy** - Log collection and processing
- **Loki** - Time-series log storage  
- **Grafana** - Visualization and dashboards

## Prerequisites

- Docker and Docker Compose installed
- Simple Proxy running and generating JSON logs

## Setup Instructions

1. **Create log directory structure:**
   ```bash
   mkdir -p observability/logs
   mkdir -p observability/grafana/provisioning/datasources
   mkdir -p observability/grafana/dashboards
   ```

2. **Configure Simple Proxy logging:**
   ⚠️ **CRITICAL:** Simple Proxy writes JSON logs to `./observability/logs/simple-proxy.jsonl` by default
   
   Verify the log directory mapping in docker-compose.yml:
   ```yaml
   volumes:
     - ./logs:/var/log/simple-proxy:ro  # Must match actual log location
   ```

3. **Start the Loki stack:**
   ```bash
   cd observability
   docker-compose up -d
   ```

4. **Verify services:**
   - Loki: http://localhost:3100/ready
   - Alloy UI: http://localhost:12345  
   - Grafana: http://localhost:3000 (admin/admin)

## Services Overview

- **Loki (port 3100)**: Log storage and querying
- **Alloy (port 12345)**: Log collection agent with web UI
- **Grafana (port 3000)**: Dashboard and visualization

## Log Labels Created

Alloy automatically creates these labels from Simple Proxy logs:
- `category`: transformation, request, success, warning, error, health, failover, classification, debug, blocked
- `component`: circuit_breaker, hybrid_classifier, tool_correction, exitplanmode_validation, schema_correction, endpoint_management, proxy_core
- `request_id`: req_123, req_456, etc.
- `tool_name`: Read, Write, Task, etc. (extracted from tool operations)
- `endpoint`: Extracted endpoint URLs for circuit breaker tracking

## Basic LogQL Queries

```logql
# All Simple Proxy logs
{service="simple-proxy"}

# Errors only  
{service="simple-proxy", category="error"}

# Circuit breaker events
{service="simple-proxy", component="circuit_breaker"}

# Specific request trace
{service="simple-proxy", request_id="req_123"}

# Tool correction attempts
{service="simple-proxy", component="tool_correction"}

# Rate of errors per minute
rate({service="simple-proxy", category="error"}[5m])
```

## ⚠️ Critical Configuration Points

### Log Directory Mapping
**MOST COMMON ISSUE:** Alloy not finding actual Simple Proxy logs

**Symptom:** You see test messages in Loki but no real Simple Proxy logs  
**Cause:** Wrong volume mount in docker-compose.yml  
**Fix:** Ensure volume mount matches where Simple Proxy actually writes logs:

```yaml
# Simple Proxy writes to: observability/logs/simple-proxy.jsonl
# So mount should be:
volumes:
  - ./logs:/var/log/simple-proxy:ro  # Correct for observability/logs/
  
# NOT:
volumes:  
  - ../logs:/var/log/simple-proxy:ro  # Wrong - looks in project root
```

### Raw JSON Issue  
**Symptom:** Seeing raw JSON line by line instead of structured logs  
**Cause:** Multiple conflicting log processing pipelines  
**Fix:** Use only JSON pipeline, remove text log pipeline from alloy-config.alloy

### Template Conflicts
**Symptom:** Raw JSON mixed with structured logs  
**Cause:** Multiple `stage.output` blocks in JSON parser  
**Fix:** Only have one `stage.output { source = "structured_log" }` block

## Troubleshooting

### Check Log Flow
```bash
# 1. Verify Simple Proxy is writing logs
tail -f observability/logs/simple-proxy.jsonl

# 2. Check Alloy is reading the correct file  
docker-compose logs alloy | grep "tail routine"

# 3. Check Alloy's current read position vs file size
wc -c observability/logs/simple-proxy.jsonl
docker-compose logs alloy | grep "Seeked"
# If Offset is much smaller than file size, Alloy is stuck

# 4. Verify in Grafana
# http://localhost:3000 → Explore → Loki → {service="simple-proxy"}
```

### Common Fixes
```bash
# Fix log directory mapping
docker-compose down
# Edit docker-compose.yml volume mount
docker-compose up -d

# Fix Alloy configuration 
# Edit alloy-config.alloy
docker-compose restart alloy

# Fix Alloy stuck at old log position (CRITICAL FIX)
docker-compose down
docker volume rm observability_alloy-data  # Clears persistent state
docker-compose up -d
```

## Next Steps

1. Verify logs are flowing: Check Grafana → Explore → Loki
2. Create custom dashboards for specific metrics  
3. Set up alerting for critical issues
4. See STRUCTURED_LOGS_SETUP.md for advanced configuration