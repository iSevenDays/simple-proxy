# Alloy Troubleshooting Quick Reference

## 🚨 Most Common Issue: Logs Not Appearing in Loki

### Symptom
- Simple Proxy is running and generating logs in JSON file
- Log file is growing with new entries
- But logs don't appear in Loki/Grafana
- May see test messages but not real application logs

### Root Cause
Alloy persistent state is stuck reading from old file position/offset

### Quick Diagnosis
```bash
# Check if this is the issue
wc -c observability/logs/simple-proxy.jsonl          # File size (e.g., 650550)
docker compose logs alloy | grep "Seeked"            # Alloy offset (e.g., 437)

# If file size >> offset → Alloy is stuck at old position
```

### Quick Fix Options

#### Option 1: Reset Alloy State (Nuclear)
```bash
docker compose down
docker volume rm observability_alloy-data
docker compose up -d
```

#### Option 2: Use tail_from_end Configuration (Preventive)
Add to your `alloy-config.alloy`:
```alloy
loki.source.file "simple_proxy_json" {
    targets      = local.file_match.simple_proxy_json_logs.targets
    forward_to   = [loki.process.json_parser.receiver]
    tail_from_end = true  // Always start from end of file
}
```
Then restart Alloy: `docker compose restart alloy`

#### Option 3: Manual Position File Reset
```bash
# Find Alloy container and reset position file
docker compose exec alloy sh -c "rm -f /var/lib/alloy/data/positions.yml"
docker compose restart alloy
```

### Verification
```bash
# Should see Alloy reading from fresh position
docker compose logs alloy | grep "Seeked"
# Output: Seeked ... &{Offset:0 Whence:0} (fresh start)

# Logs should appear in Loki within 1-2 minutes
```

## Other Common Issues

### Issue: Wrong Log Directory
**Symptom:** Test messages work, but no app logs  
**Fix:** Update docker-compose.yml volume mount

### Issue: Raw JSON Instead of Structured Logs  
**Symptom:** Seeing `{"field":"value"}` instead of `message | field=value`  
**Fix:** Remove duplicate processing pipelines in alloy-config.alloy

### Issue: Configuration Changes Not Applied
**Symptom:** Config changes don't take effect  
**Fix:** Always restart Alloy after config changes:
```bash
docker compose restart alloy
```

## When to Reset Alloy State

Reset Alloy persistent state when:
- ✅ Configuration changes while containers running
- ✅ Log file paths change
- ✅ File size much larger than Alloy offset
- ✅ Logs stop appearing after working previously
- ✅ Switching between different log files

## Root Cause Prevention

### ✅ Recommended: Use tail_from_end (Built-in Solution)
```alloy
loki.source.file "simple_proxy_json" {
    targets      = local.file_match.simple_proxy_json_logs.targets
    forward_to   = [loki.process.json_parser.receiver]
    tail_from_end = true  // Prevents stuck offset issues
}
```

**Benefits:**
- Always starts from end of file on restart
- Prevents getting stuck at old positions
- No manual intervention needed
- Built-in Alloy feature

### Additional Prevention

1. **Stop containers before major config changes:**
   ```bash
   docker compose down
   # Edit configurations
   docker compose up -d
   ```

2. **Monitor Alloy position vs file size regularly:**
   ```bash
   # Add to monitoring/alerting
   wc -c observability/logs/simple-proxy.jsonl
   docker compose logs alloy --tail=1 | grep "Seeked"
   ```

3. **Use log rotation for very large log files** (prevents offset issues)

4. **Alternative: Use log streaming instead of file tailing** (if applicable)

## Emergency Reset (Nuclear Option)

If all else fails, completely reset the observability stack:
```bash
docker compose down
docker volume rm observability_alloy-data observability_grafana-storage observability_loki-data
docker compose up -d
```
⚠️ **Warning:** This deletes all stored logs and Grafana dashboards!