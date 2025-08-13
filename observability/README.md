# Simple Proxy Observability

Complete observability stack for Simple Proxy with structured logging, real-time monitoring, and comprehensive analytics.

## 🏗️ Architecture

**[ARCHITECTURE.md](./ARCHITECTURE.md)** - Complete system architecture
- Data flow and component interactions
- Log schema and field documentation  
- Processing pipeline details
- Performance considerations

## 🚀 Quick Start

**[setup.md](./setup.md)** - Basic setup instructions
```bash
cd observability/
docker compose up -d
```

**[STRUCTURED_LOGS_SETUP.md](./STRUCTURED_LOGS_SETUP.md)** - Comprehensive setup guide
- Structured log formatting
- Field mapping configuration
- Advanced troubleshooting

## 🔧 Troubleshooting

**[ALLOY_TROUBLESHOOTING.md](./ALLOY_TROUBLESHOOTING.md)** - Quick troubleshooting reference
- Alloy offset issues (most common)
- Configuration problems
- Emergency reset procedures

## 📊 Querying & Analytics

**[grafana-queries.md](./grafana-queries.md)** - LogQL query examples
- Component-specific queries
- Performance metrics
- Hybrid classifier analysis
- Custom display formats

## ⚡ Services

| Service | Port | Purpose |
|---------|------|---------|
| **Loki** | 3100 | Log storage and querying |
| **Alloy** | 12345 | Log collection (Web UI) |
| **Grafana** | 3000 | Dashboards (admin/admin) |

## 📋 Key Features

✅ **Structured Logging** - Clean key-value display instead of raw JSON  
✅ **Hybrid Classifier Observability** - Complete Stage A/B/C analysis  
✅ **Circuit Breaker Monitoring** - Real-time endpoint health  
✅ **Tool Correction Tracking** - Parameter correction attempts  
✅ **Request Correlation** - Full request tracing with `request_id`  
✅ **Performance Metrics** - Response times, error rates, volumes  

## 🛠️ Configuration Files

| File | Purpose |
|------|---------|
| `docker-compose.yml` | Container orchestration |
| `alloy-config.alloy` | Log collection & processing |
| `loki-config.yml` | Log storage configuration |
| `generate-test-logs.sh` | Test log generation |

## 🎯 Common Use Cases

**Debug Hybrid Classifier Decisions:**
```logql
{component="hybrid_classifier", request_id="req_123"}
```

**Monitor Circuit Breaker Health:**
```logql
{component="circuit_breaker", level="error"}
```

**Trace Complete Request Flow:**
```logql
{request_id="req_456"} | line_format "{{.timestamp}} {{.component}}: {{.message}}"
```

## 🚨 Critical Configuration

⚠️ **Volume Mount** - Must match actual log location:
```yaml
volumes:
  - ./logs:/var/log/simple-proxy:ro  # observability/logs/
```

⚠️ **Alloy Offset Issues** - Use `tail_from_end=true` to prevent stuck reads

⚠️ **Single Pipeline** - Only use JSON processing, remove text pipelines

---

**Quick Commands:**
```bash
# Start stack
docker compose up -d

# Check logs flowing
tail -f logs/simple-proxy.jsonl

# Verify Alloy position
docker compose logs alloy | grep "Seeked"

# Reset if stuck
docker compose down && docker volume rm observability_alloy-data && docker compose up -d
```