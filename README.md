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