# Simple Proxy

A Claude Code proxy that transforms Anthropic API requests to OpenAI-compatible format.

## Development Commands

### Log Processing

To pretty-format existing log files (JSON Lines format):

```bash
# Pretty-format and overwrite log file
jq . logs/conversation-*.log > temp.json && mv temp.json logs/conversation-*.log

# Pretty-format specific log file
jq . logs/conversation-session_54000-20250810-180539.log > pretty.json && mv pretty.json logs/conversation-session_54000-20250810-180539.log

# Pretty-format all log files in place
for log in logs/*.log; do jq . "$log" > temp.json && mv temp.json "$log"; done
```

Note: New logs are automatically pretty-formatted as of the latest update.