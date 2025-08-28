---
name: harmony-fixes
description: Fix Harmony content parsing issues where channel content extraction fails and raw tokens are returned to Claude Code UI
status: backlog
created: 2025-08-28T13:35:23Z
---

# PRD: Harmony Format Parsing Fixes

## Executive Summary

Fix critical issues in Simple Proxy's Harmony format parsing where content extraction from channels (specifically `final` channel) fails, resulting in raw Harmony tokens being displayed in Claude Code UI instead of clean, formatted content. This addresses user experience degradation where properly formatted markdown responses appear as unformatted, continuous text with visible structural tokens.

## Problem Statement

### Current Issues
1. **Content Extraction Failure**: Harmony parser fails to properly extract content from `<|channel|>final<|message|>` sections
2. **Raw Token Display**: Users see raw Harmony structural tokens (`<|start|>`, `<|channel|>`, `<|message|>`, `<|return|>`) in Claude Code UI
3. **Format Degradation**: Well-structured markdown with newlines and formatting appears as continuous, unformatted text
4. **Inconsistent Behavior**: Parser works correctly for some formats but fails for proper OpenAI Harmony specification

### Root Cause Analysis
- Harmony parsing logic in `proxy/transform.go` doesn't correctly handle the full OpenAI Harmony message format
- Content extraction focuses on simplified patterns rather than full Harmony specification
- Channel separation logic fails to identify and extract clean content from `final` channel
- Fallback mechanisms inadequate when Harmony parsing partially succeeds but content extraction fails

### Impact
- **User Experience**: Degraded readability with visible structural tokens
- **Content Quality**: Loss of formatting and structure in responses
- **Developer Productivity**: Harder to read and understand AI responses in Claude Code UI
- **System Reliability**: Inconsistent behavior across different Harmony format variations

## User Stories

### Primary User Personas
1. **Claude Code Users**: Developers using Claude Code CLI for assistance
2. **System Integrators**: Users connecting Claude Code through Simple Proxy to various LLM providers

### User Journeys

#### Story 1: Developer Receives Formatted Response
**As a** Claude Code user  
**I want** to receive properly formatted responses with preserved newlines and markdown  
**So that** I can easily read and understand AI-generated content  

**Current Experience:**
- User asks: "Present me with your two best ideas to solve duplication issue"
- Receives: `<|start|>assistant<|channel|>final<|message|>**Solution A** Add a private method...` (raw tokens visible)
- Result: Confused, hard-to-read response

**Desired Experience:**
- User asks: "Present me with your two best ideas to solve duplication issue"
- Receives: Clean markdown with proper formatting and newlines
- Result: Easy to read, well-structured response

#### Story 2: System Administrator Troubleshoots Issues
**As a** system administrator  
**I want** consistent Harmony parsing behavior  
**So that** I can reliably predict and debug content formatting issues  

**Acceptance Criteria:**
- [ ] All Harmony format variations are handled consistently
- [ ] Clear error logging when parsing fails
- [ ] Graceful fallback to non-Harmony parsing when Harmony tokens malformed

## Requirements

### Functional Requirements

#### FR1: Proper Harmony Channel Parsing
- **Must** correctly parse OpenAI Harmony format: `<|start|>assistant<|channel|>final<|message|>{content}<|return|>`
- **Must** extract clean content from `final` channel without structural tokens
- **Must** preserve all newlines, formatting, and whitespace in extracted content
- **Must** handle both `<|return|>` and `<|end|>` termination tokens

#### FR2: Multi-Channel Support
- **Must** distinguish between `analysis`, `commentary`, and `final` channels
- **Must** extract content from appropriate channels based on configuration
- **Should** support channel-specific processing rules

#### FR3: Robust Fallback Mechanisms
- **Must** gracefully handle malformed Harmony content
- **Must** fallback to non-Harmony parsing when Harmony parsing fails
- **Must** preserve original content when extraction fails rather than showing partial tokens

#### FR4: Content Integrity
- **Must** preserve exact character sequences including newlines, tabs, and spacing
- **Must** maintain markdown formatting structure
- **Must** handle Unicode characters correctly

### Non-Functional Requirements

#### NFR1: Performance
- **Must** add minimal latency to response processing (<10ms overhead)
- **Should** fail fast on malformed content rather than attempting expensive parsing

#### NFR2: Reliability
- **Must** handle all Harmony format edge cases without crashes
- **Must** maintain backward compatibility with existing non-Harmony content
- **Should** provide detailed error logging for debugging

#### NFR3: Maintainability
- **Must** use testable, modular parsing functions
- **Should** follow existing codebase patterns and conventions
- **Should** include comprehensive unit test coverage

## Success Criteria

### Measurable Outcomes
1. **Parsing Accuracy**: 100% correct content extraction for valid Harmony formats
2. **Format Preservation**: 0% content formatting loss (newlines, markdown structure)
3. **Error Handling**: 100% graceful failure handling without system crashes
4. **User Experience**: Elimination of visible structural tokens in Claude Code UI

### Key Metrics and KPIs
- **Test Coverage**: ≥95% coverage for Harmony parsing functions
- **Regression Tests**: All existing functionality remains unaffected
- **Performance Impact**: <1% increase in response processing time
- **Error Rate**: <0.1% parsing failures on valid Harmony content

## Technical Implementation Details

### Core Changes Required

#### 1. Enhanced Harmony Pattern Recognition
```go
// Improved pattern matching for full Harmony format
harmonyPattern := `<\|start\|>assistant<\|channel\|>(analysis|commentary|final)<\|message\|>(.*?)<\|(return|end)\|>`
```

#### 2. Channel-Specific Content Extraction
- Parse multiple channels in single response
- Extract content from appropriate channel based on priority
- Handle mixed channel responses

#### 3. Content Cleaning Pipeline
- Strip all Harmony structural tokens
- Preserve content formatting and whitespace
- Validate content integrity

### Test Coverage Requirements
- **Unit Tests**: All parsing functions with edge cases
- **Integration Tests**: Full request/response cycle with Harmony content
- **Regression Tests**: Ensure non-Harmony content unaffected
- **Performance Tests**: Response time impact measurement

## Constraints & Assumptions

### Technical Constraints
- **Backward Compatibility**: Must not break existing non-Harmony parsing
- **Performance**: Cannot significantly impact response latency
- **Memory**: Parsing must not consume excessive memory for large responses

### Timeline Constraints
- **Critical Fix**: High priority due to user experience impact
- **Quick Resolution**: Should be implementable within existing architecture

### Resource Constraints
- **Development**: Single developer implementation
- **Testing**: Comprehensive test suite required before deployment

### Assumptions
- OpenAI Harmony format specification is stable
- Existing parser/harmony.go module provides necessary building blocks
- Claude Code UI correctly handles properly formatted responses

## Out of Scope

### Explicitly NOT Included
1. **New Harmony Features**: Only fixing existing parsing, not adding new capabilities
2. **UI Changes**: No changes to Claude Code UI rendering
3. **Configuration Options**: No new user-configurable parsing options
4. **Performance Optimization**: Focus on correctness over performance improvements
5. **Alternative Formats**: Only OpenAI Harmony format support

## Dependencies

### External Dependencies
- **OpenAI Harmony Specification**: Stable format definition
- **Claude Code Compatibility**: UI must handle formatted content correctly

### Internal Dependencies
- **parser/harmony.go**: Existing Harmony parsing utilities
- **proxy/transform.go**: Response transformation pipeline
- **Test Infrastructure**: Existing testing framework and patterns
- **Logging System**: Error reporting and debugging capabilities

### Team Dependencies
- **Development**: Implementation and testing
- **QA**: Validation of user experience improvements
- **DevOps**: Deployment and monitoring

## Risk Analysis

### High-Risk Areas
1. **Regression Risk**: Changes could break existing non-Harmony content parsing
2. **Performance Risk**: Complex parsing could impact response times
3. **Edge Case Risk**: Unforeseen Harmony format variations could cause failures

### Mitigation Strategies
1. **Comprehensive Testing**: Extensive unit and integration test coverage
2. **Gradual Rollout**: Feature flag for controlled deployment
3. **Monitoring**: Enhanced logging and error tracking
4. **Rollback Plan**: Quick revert capability if issues arise

## Implementation Plan

### Phase 1: Foundation (Week 1)
- Analyze existing Harmony parsing code in detail
- Create comprehensive test suite reproducing all known issues
- Design improved parsing architecture

### Phase 2: Core Implementation (Week 2)
- Implement enhanced channel parsing logic
- Add content extraction and cleaning functions
- Integrate with existing transformation pipeline

### Phase 3: Testing & Validation (Week 3)
- Execute full test suite
- Performance testing and optimization
- User experience validation with real content

### Phase 4: Deployment (Week 4)
- Production deployment with monitoring
- User feedback collection
- Performance monitoring and optimization

## Acceptance Criteria

### Must Have
- [ ] All visible Harmony tokens eliminated from Claude Code UI responses
- [ ] Newlines and markdown formatting preserved in extracted content
- [ ] Zero regression in non-Harmony content processing
- [ ] Comprehensive error handling with graceful fallbacks
- [ ] ≥95% test coverage for all parsing functions

### Should Have
- [ ] Detailed logging for debugging parsing issues
- [ ] Performance impact <10ms per response
- [ ] Support for all three Harmony channels (analysis, commentary, final)

### Could Have
- [ ] Configurable channel priority for content extraction
- [ ] Enhanced error messages for malformed Harmony content
- [ ] Metrics collection for Harmony parsing success rates

## Post-Launch Monitoring

### Success Indicators
1. **User Feedback**: Improved readability reports from Claude Code users
2. **Error Metrics**: Reduction in parsing-related error logs
3. **Performance**: Stable response times with new parsing logic
4. **Functionality**: All existing features working correctly

### Key Performance Indicators
- **Parsing Success Rate**: >99.9% for valid Harmony content
- **Response Time Impact**: <1% increase in average response time
- **Error Rate**: <0.1% parsing failures
- **User Satisfaction**: Improved feedback on response readability