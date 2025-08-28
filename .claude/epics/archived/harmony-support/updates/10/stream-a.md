---
issue: 10
stream: harmony_detection_investigation
agent: code-analyzer
started: 2025-08-28T10:31:54Z
status: in_progress
---

# Stream A: Harmony Detection Investigation

## Scope
Debug why Harmony tokens aren't being detected in tool call scenarios

## Files
- parser/harmony.go
- Test data from Claude Code UI logs

## Progress
- ✅ Investigation complete
- **Root Cause Found**: `HasHarmonyTokens()` only checks for `<|start|>` or `<|end|>` tokens
- **Failing Case**: Contains `<|channel|>` and `<|message|>` but no `<|start|>`/`<|end|>`
- **Fix Required**: Extend detection to include channel/message tokens
- **Location**: `parser/harmony.go` lines 634-635