# Simple Proxy Observability Architecture

## Overview

The Simple Proxy observability system provides comprehensive logging, monitoring, and analysis capabilities using the modern Grafana observability stack. The architecture enables real-time monitoring of proxy operations, hybrid classifier decisions, circuit breaker states, and tool corrections.

## System Architecture

### High-Level Data Flow
```
Simple Proxy → JSON Logs → Alloy Agent → Loki → Grafana
     ↓              ↓           ↓          ↓        ↓
Components    Structured   Processing   Storage  Visualization
```

### Core Components

#### 1. Log Generation Layer
- **ObservabilityLogger** (`logger/observability.go`)
  - Built on logrus for structured JSON logging
  - Flat JSON structure for optimal parsing
  - Component-based categorization
  - Request correlation via `request_id`

#### 2. Log Collection Layer
- **Grafana Alloy** (Container: `observability-alloy-1`)
  - Modern replacement for Promtail/Grafana Agent
  - File-based log collection with `tail_from_end=true`
  - JSON parsing and field extraction
  - Structured log template formatting
  - Port: 12345 (Web UI)

#### 3. Log Storage Layer
- **Loki** (Container: `observability-loki-1`)
  - Time-series log storage optimized for labels
  - TSDB index with filesystem storage
  - Label-based querying with LogQL
  - Port: 3100 (API)

#### 4. Visualization Layer
- **Grafana** (Container: `observability-grafana-1`)
  - Dashboard and alerting platform
  - LogQL query interface
  - Pre-built Simple Proxy dashboard
  - Port: 3000 (Web UI)

## Data Architecture

### Log Schema

#### Standard Fields (All Logs)
```json
{
  "timestamp": "2025-08-13T10:30:04.282+02:00",
  "level": "info|warn|error",
  "service": "simple-proxy", 
  "component": "proxy_core|hybrid_classifier|tool_correction|circuit_breaker|configuration",
  "category": "request|classification|transformation|health|error|warning",
  "message": "Human-readable log message",
  "request_id": "req_123" // Request correlation
}
```

#### Component-Specific Fields

**Hybrid Classifier Observability:**
```json
{
  "stage": "A_extract_pairs|B_rule_engine|C_llm_fallback",
  "rule_name": "StrongVerbWithFile|ContextualNegation|...",
  "matched": true,
  "final_decision": "require_tools|optional", 
  "final_confident": true,
  "user_prompt": "Full user message content",
  "pairs_count": 5,
  "total_pairs": 15,
  "analysis_result": "yes|no"
}
```

**Circuit Breaker Monitoring:**
```json
{
  "endpoint": "http://192.168.0.46:11434",
  "failure_count": 2,
  "success_rate": 0.85,
  "circuit_open": false,
  "response_time_ms": 1500
}
```

**Tool Correction Tracking:**
```json
{
  "tool_name": "Read|Write|Edit|...",
  "original_param": "filename",
  "corrected_param": "file_path", 
  "retry_count": 1
}
```

### Label Strategy

**High-Cardinality Labels** (Efficient Querying):
- `service`: "simple-proxy" 
- `level`: "info|warn|error"
- `component`: Component identifier
- `category`: Event category

**Low-Cardinality Labels** (Specific Filtering):
- `request_id`: Request correlation
- `stage`: Hybrid classifier stage
- `rule_name`: Matched rule name

## Processing Pipeline

### Alloy Configuration (`alloy-config.alloy`)

#### File Discovery
```alloy
local.file_match "simple_proxy_json_logs" {
    path_targets = [{
        __path__ = "/var/log/simple-proxy/*.jsonl",
        service  = "simple-proxy", 
        job      = "simple-proxy-structured",
        log_type = "json",
    }]
}
```

#### Log Processing
```alloy
loki.source.file "simple_proxy_json" {
    targets      = local.file_match.simple_proxy_json_logs.targets
    forward_to   = [loki.process.json_parser.receiver]
    tail_from_end = true  // Prevents offset issues
}

loki.process "json_parser" {
    // JSON field extraction (50+ fields)
    stage.json { expressions = {...} }
    
    // Structured formatting template
    stage.template {
        template = `{{ .message }}{{ if .request_id }} | request_id={{ .request_id }}{{ end }}...`
    }
    
    // Label assignment for efficient querying
    stage.labels { values = {...} }
}
```

## Component Integration

### Simple Proxy Integration
The observability system is deeply integrated into Simple Proxy components:

#### Main Application (`main.go`)
```go
// Initialize observability logger
obsLogger, err := logger.NewObservabilityLogger(logDir)
cfg.SetObservabilityLogger(obsLogger)

// Startup logging
obsLogger.Info(logger.ComponentProxy, logger.CategoryRequest, "", 
    "Claude Code Proxy starting", map[string]interface{}{
        "port": cfg.Port,
        "tool_correction_enabled": cfg.ToolCorrectionEnabled,
    })
```

#### Hybrid Classifier (`correction/hybrid_classifier.go`)
- **Stage A**: Action pair extraction logging
- **Stage B**: Rule engine evaluation with match details  
- **Stage C**: LLM fallback analysis with full prompts
- Complete decision trace with confidence metrics

#### Circuit Breaker (`circuitbreaker/`)
- Endpoint health monitoring
- Failure threshold tracking
- State transition logging

#### Tool Correction (`correction/service.go`)
- Parameter correction attempts
- Success/failure tracking
- Retry logic monitoring

## Deployment Architecture

### Container Layout
```
observability/
├── observability-loki-1      # Log storage
├── observability-alloy-1     # Log collection  
└── observability-grafana-1   # Visualization
```

### Volume Management
```yaml
volumes:
  - ./logs:/var/log/simple-proxy:ro           # Log file access
  - ./alloy-config.alloy:/etc/alloy/config.alloy:ro  # Configuration
  - alloy-data:/var/lib/alloy/data            # Persistent state
  - loki-data:/loki                           # Log storage
  - grafana-storage:/var/lib/grafana          # Dashboards/config
```

### Network Configuration
```yaml
networks:
  loki:                    # Internal communication
    driver: bridge
    
ports:
  - "3100:3100"           # Loki API
  - "12345:12345"         # Alloy UI  
  - "3000:3000"           # Grafana UI
```

## Query Architecture

### LogQL Query Patterns

**Component Filtering:**
```logql
{component="hybrid_classifier"}
{component="circuit_breaker", level="error"}
```

**Request Tracing:**
```logql
{request_id="req_123"}
```

**Stage Analysis:**
```logql
{component="hybrid_classifier", stage="B_rule_engine"}
```

**Performance Metrics:**
```logql
{service="simple-proxy"} | json | unwrap response_time_ms | rate(5m)
```

## Configuration Files

### Critical Configuration Files

#### `docker-compose.yml`
- Container orchestration
- Volume mounts (**CRITICAL**: Must match log locations)
- Network configuration
- Service dependencies

#### `alloy-config.alloy` 
- Log collection configuration
- JSON field parsing (50+ fields)
- Structured formatting templates
- Label assignment strategy

#### `loki-config.yml`
- Storage configuration
- Index management
- Retention policies

## Monitoring & Alerting

### Key Metrics to Monitor

**System Health:**
- Log ingestion rate
- Alloy file position vs file size
- Container health status

**Application Metrics:**
- Error rates by component
- Circuit breaker state changes
- Tool correction success rates
- Hybrid classifier decision distribution

### Alert Conditions

**Critical Issues:**
- Alloy stuck at old file position
- High error rates (>5% over 5min)
- Circuit breaker failures
- Log ingestion stopped

## Troubleshooting Architecture

### Common Issues

1. **Alloy Offset Stuck** → `tail_from_end=true` + position file reset
2. **Volume Mount Mismatch** → Docker Compose volume configuration  
3. **Raw JSON Display** → Alloy template processing conflicts
4. **Missing Fields** → JSON parsing expression updates

### Diagnostic Tools
- File position monitoring
- Container log analysis
- Volume mount verification
- Configuration validation

## Performance Considerations

### Optimization Strategies

**Label Cardinality:**
- Keep high-cardinality labels minimal (4-6 labels)
- Use structured display for detailed information
- Avoid request_id as primary label for high-traffic systems

**Storage Efficiency:**
- JSON field flattening for faster parsing
- Appropriate retention policies
- Index optimization for common query patterns

**Processing Efficiency:**
- Single log processing pipeline
- Template-based structured formatting
- Efficient regex patterns for field extraction

## References

### Related Documentation
- **[STRUCTURED_LOGS_SETUP.md](./STRUCTURED_LOGS_SETUP.md)** - Complete setup guide with structured log formatting
- **[ALLOY_TROUBLESHOOTING.md](./ALLOY_TROUBLESHOOTING.md)** - Troubleshooting reference for Alloy issues
- **[grafana-queries.md](./grafana-queries.md)** - LogQL query examples and patterns
- **[setup.md](./setup.md)** - Basic setup instructions

### Code References
- **`logger/observability.go`** - Core logging implementation
- **`main.go:21-46`** - Observability logger initialization
- **`correction/hybrid_classifier.go`** - Stage A/B/C observability integration
- **`correction/rules.go`** - Rule engine observability
- **`correction/service.go`** - Tool correction logging

### Configuration Files
- **`alloy-config.alloy`** - Complete Alloy configuration
- **`docker-compose.yml`** - Container orchestration
- **`loki-config.yml`** - Loki storage configuration
- **`grafana/dashboards/simple-proxy-dashboard.json`** - Pre-built dashboard