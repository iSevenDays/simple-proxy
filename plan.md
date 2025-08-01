# Simple Proxy Logging Improvements Plan

## Current Session Context

### Problems Identified
1. **Inconsistent Small Model Logging**: `DISABLE_SMALL_MODEL_LOGGING=true` doesn't hide all logs for Haiku model (`claude-3-5-haiku-20241022`)
2. **Scattered Logging Control**: `shouldLogForModel` logic exists but isn't used consistently across all components
3. **Missing Context**: Some components (config.go, transform.go) can't access model info for logging decisions

### Evidence from Logs
```
2025/08/01 14:48:39 🔄[req_5000] Model mapping: claude-3-5-haiku-20241022 → qwen2.5-coder:latest
2025/08/01 14:48:39 ➕ prepend applied: 'You are an expert coding assistant with access to powerful tools.'
2025/08/01 14:48:39 ➕ append applied: 'Always prioritize code quality and best practices.'
2025/08/01 14:48:39 🔄 [req_5000] Applied system message overrides (original: 132 chars, modified: 248 chars)
```
These logs appear for Haiku model despite `DISABLE_SMALL_MODEL_LOGGING=true` being intended to hide them.

## Root Cause Analysis

### File Analysis

#### 1. `proxy/handler.go` (Lines 75-78, 349-356)
- **Has working `shouldLogForModel` logic**
- Controls: `📨 Received request`, `🔧 Available tools`, `🚀 Proxying to`, `✅ Response summary`
- **Status**: ✅ Working correctly

#### 2. `config/config.go` (Line 304)
- **Missing logging control**
- Always shows: `🔄 Model mapping: X → Y`
- **Issue**: No access to `shouldLogForModel` logic
- **Status**: ❌ Always logs regardless of settings

#### 3. `proxy/transform.go` (Lines 105-107)
- **Missing logging control**
- Always shows: `🔄 Applied system message overrides`
- **Issue**: Uses own `shouldLogForModel` but may not be working correctly
- **Status**: ❌ Not respecting small model settings

#### 4. `config/system_overrides.go` (Inferred)
- **Missing logging control**
- Always shows: `➕ prepend applied`, `➕ append applied`
- **Status**: ❌ Always logs regardless of settings

## SPARC-Compliant Solution Plan

### Phase 1: Fix Immediate Issues (High Impact, Low Risk)

#### 1.1 Fix Model Mapping Log (`config/config.go:304`)
**File**: `config/config.go`
**Current**:
```go
log.Printf("🔄[%s] Model mapping: %s → %s", requestID, claudeModel, mapped)
```
**Solution**: Add conditional logging
**Motivation**: Model mapping happens before handler has chance to filter

#### 1.2 Fix System Message Override Logs (`proxy/transform.go:105-107`)
**File**: `proxy/transform.go`
**Current**: Has `shouldLogForModel` but may not be working
**Solution**: Verify and fix the existing logic
**Motivation**: Transform happens early in pipeline, needs proper filtering

#### 1.3 Fix System Override Application Logs
**File**: `config/system_overrides.go` (or similar)
**Current**: Always logs prepend/append operations
**Solution**: Add model-aware logging
**Motivation**: These are frequent logs that should be filtered for small models

### Phase 2: Centralized Logging (Future Enhancement)
**Motivation**: While not immediately necessary, would prevent future inconsistencies
**Approach**: Create `proxy/logging.go` with centralized utility

## Implementation Strategy

### Decision: Minimal Changes Approach
Following SPARC **Simplicity** and **Iterate** principles:
1. ✅ **Enhance existing code** rather than rewriting
2. ✅ **Fix specific issues** rather than architectural changes
3. ✅ **Preserve all existing functionality**

### Decision: Context Passing
**Problem**: `config.go` doesn't have access to model context
**Solution**: Pass model information through function parameters where needed
**Alternative Rejected**: Global state (violates simplicity)

### Decision: Logging Control Pattern
**Approach**: Use conditional logging at call site
```go
// Before
log.Printf("🔄[%s] Model mapping: %s → %s", requestID, claudeModel, mapped)

// After  
if shouldLogForModel(claudeModel, cfg) {
    log.Printf("🔄[%s] Model mapping: %s → %s", requestID, claudeModel, mapped)
}
```

## File References and Locations

### Key Functions to Modify
1. **`config.MapModelName()`** - Add logging control
2. **`config.ApplySystemMessageOverrides()`** - Add logging control  
3. **`proxy.TransformAnthropicToOpenAI()`** - Verify existing logic

### Key Files
- `config/config.go:304` - Model mapping log
- `proxy/transform.go:105-107` - System message override log
- `config/system_overrides.go` - Prepend/append logs (need to locate)
- `proxy/handler.go:349-356` - Reference implementation (working correctly)

### Existing Logic to Leverage
```go
// From proxy/handler.go:349-356 (working correctly)
func (h *Handler) shouldLogForModel(ctx context.Context, claudeModel string) bool {
    if h.config.DisableSmallModelLogging && h.isSmallModel(ctx, claudeModel) {
        return false
    }
    return true
}

func (h *Handler) isSmallModel(ctx context.Context, claudeModel string) bool {
    return claudeModel == "claude-3-5-haiku-20241022" || 
           h.config.MapModelName(ctx, claudeModel) == h.config.SmallModel
}
```

## Expected Outcomes

### Immediate Benefits
- `DISABLE_SMALL_MODEL_LOGGING=true` will hide ALL small model logs consistently
- Cleaner logs when debugging large models while filtering noise from small models
- Consistent behavior across all proxy components

### Quality Improvements  
- Predictable logging behavior
- Easier debugging (less noise)
- Better adherence to user configuration

## Testing Strategy

### Test Cases
1. **Small model with logging disabled**: No logs should appear
2. **Small model with logging enabled**: All logs should appear  
3. **Large model**: All logs should appear regardless of small model setting
4. **Configuration edge cases**: Missing .env values, invalid settings

### Verification Commands
```bash
# Test with small model logging disabled
DISABLE_SMALL_MODEL_LOGGING=true ./simple-proxy

# Should see no logs for claude-3-5-haiku-20241022 requests
# Should see all logs for claude-sonnet-4-20250514 requests
```

## Next Session Action Items

1. **Locate system override logs**: Find where `➕ prepend applied` logs originate
2. **Implement conditional logging**: Add `shouldLogForModel` calls to identified locations
3. **Test configuration**: Verify `DISABLE_SMALL_MODEL_LOGGING` works end-to-end
4. **Consider centralized utility**: If pattern repeats, implement `proxy/logging.go`

## Architecture Notes

### Design Principles Applied
- **Simplicity**: Minimal changes to existing codebase
- **Iterate**: Enhance existing logging rather than rewrite
- **Focus**: Address specific small model logging issue
- **Quality**: Make logging behavior predictable
- **Collaboration**: Clear patterns for future development

### Trade-offs Made
- **Chosen**: Function-level conditional logging
- **Rejected**: Global logging utility (more complex, not immediately needed)
- **Chosen**: Parameter passing for context
- **Rejected**: Context manipulation (more invasive)

This plan provides a clear roadmap for the next session to implement consistent small model logging behavior across all Simple Proxy components.

## Next Session Prompt

**Copy this prompt to restart the session:**

```
I need to continue implementing improvements to the Simple Proxy logging system. Please read the plan.md file which contains:

1. Complete context from previous debugging session
2. Root cause analysis of inconsistent small model logging  
3. SPARC-compliant solution plan
4. File references and implementation strategy

The issue: DISABLE_SMALL_MODEL_LOGGING=true doesn't hide all logs for Haiku model (claude-3-5-haiku-20241022) because some logs bypass the shouldLogForModel logic.

Key files to modify:
- config/config.go:304 (model mapping log)
- proxy/transform.go:105-107 (system message override log)  
- System override files (prepend/append logs)

Start by reading plan.md then implement Phase 1 of the solution following SPARC principles (Simplicity, Iterate, Focus, Quality).
```

This prompt will allow you to quickly understand the context and continue from where we left off.