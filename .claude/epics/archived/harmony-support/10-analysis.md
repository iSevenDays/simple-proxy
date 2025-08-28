---
issue: 10
title: Fix harmony parsing when tool calls are made
epic: harmony-support
analyzed: 2025-08-28T09:30:00Z
complexity: medium
estimated_hours: 4-6
parallel: true
priority: high
---

# Issue #10 Analysis - Fix Harmony Parsing When Tool Calls Are Made

## Problem Analysis

### Evidence Summary
- **Claude Code UI**: Raw Harmony tokens displayed instead of processed thinking content
- **Simple Proxy logs**: `🔍 No Harmony tokens detected in content` despite clear Harmony format
- **Content sample**: `<|channel|>analysis<|message|>...` clearly present but undetected
- **Context**: Issue occurs specifically when tool calls are involved

### Root Cause Hypotheses
1. **Harmony Detection Failure**: `parser.IsHarmonyFormat()` regex patterns may not handle tool call context
2. **Content Structure Differences**: Tool call responses may have different formatting
3. **Parser Configuration**: Detection logic may need tool-call-specific handling

## Work Stream Analysis

### Stream A: Harmony Detection Investigation
- **Focus**: Debug why Harmony tokens aren't being detected in tool call scenarios
- **Files**: `parser/harmony.go`
- **Tasks**:
  - Add debug logging to `IsHarmonyFormat()` function
  - Test detection with exact content from logs
  - Compare tool call vs non-tool call content structure
  - Identify regex pattern failures

### Stream B: Content Structure Analysis
- **Focus**: Analyze how tool call content differs from regular Harmony responses
- **Files**: Response samples, test data
- **Tasks**:
  - Extract exact content from Claude Code UI logs
  - Compare with working Harmony examples
  - Identify structural differences
  - Document tool call specific patterns

### Stream C: Parser Logic Enhancement
- **Focus**: Fix detection and parsing logic for tool call scenarios
- **Files**: `parser/harmony.go`, test files
- **Tasks**:
  - Update regex patterns if needed
  - Add tool call specific detection logic
  - Implement comprehensive test cases
  - Verify parsing works with tool call content

### Stream D: Integration Testing and Validation
- **Focus**: End-to-end testing of tool call + Harmony scenarios
- **Files**: `proxy/transform.go`, integration tests
- **Tasks**:
  - Create test cases with tool call + Harmony content
  - Verify complete transformation pipeline
  - Test Claude Code UI compatibility
  - Validate no regressions in existing functionality

## Coordination Plan

### Parallel Execution
- **Stream A & B**: Can run in parallel (investigation phase)
- **Stream C**: Depends on findings from A & B
- **Stream D**: Depends on completion of C

### Communication Points
1. **After Stream A**: Share detection failure analysis
2. **After Stream B**: Share content structure findings  
3. **Before Stream C**: Coordinate solution approach
4. **After Stream C**: Ready for integration testing

## Dependencies

### Internal Dependencies
- Issue #9 completion (thinking content assignment) - ✅ **COMPLETED**
- Existing Harmony parsing infrastructure
- Parser and transform pipeline

### External Dependencies
- Access to Claude Code UI logs for testing
- Tool call scenarios for comprehensive testing

## Success Metrics

### Technical Validation
- Harmony detection works correctly for tool call scenarios
- All existing tests continue to pass
- New test cases cover tool call + Harmony combinations
- Debug logs show proper processing flow

### User Experience Validation  
- No raw Harmony tokens in Claude Code UI
- Thinking content properly separated and displayed
- Tool call results display correctly
- Performance impact < 10ms additional processing

## Risk Assessment

### Medium Risk
- **Complex Content Parsing**: Tool calls may have unexpected format variations
- **Regex Complexity**: Pattern matching for multiple scenarios can be fragile

### Low Risk
- **Infrastructure Ready**: Thinking content assignment is already implemented
- **Limited Scope**: Focused on detection logic, not major architecture changes

## Timeline Estimate

- **Stream A**: 1-2 hours (investigation)
- **Stream B**: 1-2 hours (analysis) 
- **Stream C**: 2-3 hours (implementation)
- **Stream D**: 1-2 hours (testing)
- **Total**: 4-6 hours with parallel execution