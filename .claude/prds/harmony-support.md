---
name: harmony-support
description: Add OpenAI Harmony message format parsing to distinguish thinking content from user-facing responses
status: backlog
created: 2025-08-27T05:27:33Z
---

# PRD: OpenAI Harmony Support

## Executive Summary

Add comprehensive support for OpenAI Harmony message format parsing to Simple Proxy, enabling proper distinction between thinking/analysis content (`<|channel|>analysis`) and user-facing responses (`<|channel|>final`). This resolves compatibility issues with gpt-oss-120b LLM where verbose thinking messages currently appear as regular content in Claude Code UI.

## Problem Statement

### Current Issue
- Simple Proxy doesn't recognize OpenAI Harmony format messages like `<|channel|>analysis<|message|>The user request to do the changes.`
- Thinking/analysis content appears as verbose regular messages in Claude Code UI
- Users experience poor UX with mixed thinking and response content
- Claude Code has dedicated UI/formatting for thinking messages that isn't being utilized

### Why This Matters Now
- gpt-oss-120b LLM uses Harmony format exclusively
- Other LLMs use `<think></think>` tags which are properly handled
- Compatibility gap prevents optimal use of this specific model
- User experience degradation affects productivity

## User Stories

### Primary User (Developer using Simple Proxy with gpt-oss-120b)
**As a developer using Simple Proxy with gpt-oss-120b,**
**I want thinking/analysis content to be properly separated from responses,**
**So that I see clean, focused answers without verbose internal reasoning cluttering the interface.**

**Acceptance Criteria:**
- Harmony analysis channel content is parsed and marked as thinking
- Final channel content appears as the main response
- Claude Code UI renders thinking content in dedicated thinking format
- No regression in handling of other LLM formats (`<think></think>`)

### Edge Case Scenarios
**Scenario 1: Mixed Channel Messages**
- Given: Message contains both analysis and final channels
- When: Proxy processes the response
- Then: Analysis content is flagged as thinking, final content as response

**Scenario 2: Commentary Channel**
- Given: Message contains commentary channel (tool calls)
- When: Proxy processes tool-related content
- Then: Commentary content is properly parsed and routed

**Scenario 3: Malformed Harmony Format**
- Given: Partially malformed Harmony tokens
- When: Parser encounters invalid format
- Then: Graceful fallback to treat as regular content without errors

## Format Examples

### Real-World Example (Current Problem)
**Input from gpt-oss-120b:**
```
⏺ <|channel|>analysis<|message|>The user is asking: "to compare changes in
  this branch to main, do the code review, I will create merge request
  then, first I need your review". The user wants code review between
  current branch (feature/auth-improvements) and main branch. The
  repository is a Node.js project, but note from project-specific
  CLAUDE.md that we cannot run npm commands (they will fail) and we should
  rely on static analysis. There's a code review agent optimization: max 5
  tool calls per review, 2-minute analysis target, prioritizing security,
  business logic, critical issues, using git diff to focus on changed files
   only. Thus we need to run git diff between current branch and main, get
  list of changed files and diffs. Then we need to review changes focusing
  on security, business logic, critical issues, static analysis (e.g.,
  scanning for password, secret, token, try/catch, API keys). We also
  need to ensure code quality (e.g., proper error handling, async/await
  patterns, potential memory leaks). Since the branch is named
  feature/auth-improvements, it likely contains authentication changes.
  Let's explore repository. First, list files. Use LS.<|end|>

⏺ List(~/projects/my-app)
  ⎿  Listed 245 paths (ctrl+r to expand)

⏺ <|channel|>analysis<|message|>We saw a typical Node.js project structure with multiple modules...
```

**Expected Output (After Harmony Parsing):**
```json
{
  "thinking": "The user is asking: \"to compare changes in this branch to main, do the code review...\"\n\nWe saw a typical Node.js project structure with multiple modules...",
  "content": "[Clean final response without verbose analysis]"
}
```

### Well-Formed Harmony Messages

#### Analysis Channel (Thinking Content)
**Input:**
```
<|start|>assistant<|channel|>analysis<|message|>I need to analyze the user's request for implementing a search feature. Let me break down the requirements:
1. Full-text search across documents
2. Filter by date range
3. Sort by relevance
4. Return structured results

The user wants this integrated with the existing API, so I'll need to extend the current endpoints.<|end|>
```

**Parsed Output:**
- **Channel**: analysis
- **Role**: assistant
- **Content Type**: thinking
- **Content**: "I need to analyze the user's request for implementing a search feature..."

#### Final Channel (User Response)
**Input:**
```
<|start|>assistant<|channel|>final<|message|>I'll implement a search feature with the following components:

1. **Search Endpoint**: `POST /api/search`
2. **Query Parameters**: text, dateRange, sortBy
3. **Response Format**: JSON with results array

Here's the implementation plan:<|end|>
```

**Parsed Output:**
- **Channel**: final
- **Role**: assistant
- **Content Type**: response
- **Content**: "I'll implement a search feature with the following components..."

#### Commentary Channel (Tool Calls)
**Input:**
```
<|start|>assistant<|channel|>commentary<|message|>I'll use the file search tool to find existing search implementations in the codebase.<|end|>
<|start|>assistant<|channel|>commentary<|message|>Tool call: search_files(pattern="search", type="function")<|return|>
```

**Parsed Output:**
- **Channel**: commentary
- **Role**: assistant
- **Content Type**: tool_call
- **Content**: "I'll use the file search tool to find existing search implementations..."

### Mixed Channel Message
**Input:**
```
<|start|>assistant<|channel|>analysis<|message|>The user wants to refactor the authentication system. I need to consider:
- Current JWT implementation
- Session management
- Security implications
- Backward compatibility<|end|>
<|start|>assistant<|channel|>final<|message|>I'll help you refactor the authentication system. Here's my recommended approach:

## Current Assessment
Your JWT implementation is solid, but we can improve session management.

## Proposed Changes
1. Extract auth logic into dedicated service
2. Implement refresh token rotation
3. Add session cleanup job<|end|>
```

**Parsed Output:**
```json
{
  "thinking": "The user wants to refactor the authentication system. I need to consider:\n- Current JWT implementation\n- Session management\n- Security implications\n- Backward compatibility",
  "content": "I'll help you refactor the authentication system. Here's my recommended approach:\n\n## Current Assessment\nYour JWT implementation is solid, but we can improve session management.\n\n## Proposed Changes\n1. Extract auth logic into dedicated service\n2. Implement refresh token rotation\n3. Add session cleanup job"
}
```

### Edge Cases and Error Handling

#### Malformed Tokens
**Input:**
```
<|start|>assistant<|channel|>analysis<|message|>This is thinking content but missing end token
<|start|>assistant<|channel|>final<|message|>This is the actual response<|end|>
```

**Expected Behavior**: Gracefully parse what's possible, treat malformed content as regular text

#### Missing Channel
**Input:**
```
<|start|>assistant<|message|>Content without channel specification<|end|>
```

**Expected Behavior**: Default to treating as regular response content

#### Unknown Channel
**Input:**
```
<|start|>assistant<|channel|>unknown_channel<|message|>Content in unknown channel<|end|>
```

**Expected Behavior**: Treat as regular content, log warning for debugging

### Format Comparison

#### Harmony vs Think Tags vs Standard
```
# OpenAI Harmony (NEW - Target Format)
<|start|>assistant<|channel|>analysis<|message|>Thinking content here<|end|>
<|start|>assistant<|channel|>final<|message|>User response here<|end|>

# Think Tags (CURRENT - Already Supported)
<think>Thinking content here</think>
User response here

# Standard Format (CURRENT - Already Supported)  
User response here (no thinking separation)
```

### Transformation Examples

#### Before Harmony Support
```
User sees: "⏺ <|channel|>analysis<|message|>The user is asking about implementation details and I need to analyze their requirements carefully before responding with a solution...<|end|> ⏺ Here's the implementation approach you requested..."
```

#### After Harmony Support
```
Claude Code Thinking Panel: "The user is asking about implementation details and I need to analyze their requirements carefully before responding with a solution..."

Main Response: "Here's the implementation approach you requested..."
```

## Requirements

### Functional Requirements

#### FR1: Harmony Format Recognition
- Parse OpenAI Harmony message structure: `<|start|>role<|channel|>type<|message|>content<|end|>`
- Identify channel types: `analysis`, `final`, `commentary`
- Extract role information: `assistant`, `user`, `system`, `developer`, `tool`

#### FR2: Content Classification
- Mark `analysis` channel content as thinking/internal reasoning
- Treat `final` channel content as primary user-facing response
- Handle `commentary` channel for tool calls and intermediate processing

#### FR3: Response Transformation
- Transform Harmony format to Claude Code compatible format
- Preserve semantic meaning while adapting structure
- Maintain existing tool call and system override functionality

#### FR4: Backward Compatibility
- Preserve existing `<think></think>` tag handling
- Maintain support for standard OpenAI/Anthropic formats
- No impact on non-Harmony model responses

### Non-Functional Requirements

#### NFR1: Performance
- Harmony parsing adds <10ms latency to response processing
- Memory overhead <5% increase for typical responses
- Streaming responses maintain real-time performance

#### NFR2: Reliability
- 99.9% parsing accuracy for well-formed Harmony messages
- Graceful degradation for malformed input
- No crashes or errors from invalid Harmony tokens

#### NFR3: Maintainability
- Modular parser component for easy testing and updates
- Comprehensive unit test coverage (>95%)
- Clear separation from existing transformation logic

## Technical Implementation

### Architecture Components

#### Harmony Parser Module
- `parser/harmony.go` - Core parsing logic
- Token recognition and extraction
- Channel classification
- Error handling and validation

#### Message Transformer Integration
- Extend `proxy/transform.go` with Harmony support
- Integrate with existing response transformation pipeline
- Maintain compatibility with current tool/system overrides

#### Response Structure Enhancement
- Extend response types to include thinking/analysis metadata
- Preserve channel information for downstream processing
- Support mixed-format responses

### Data Flow
```
Harmony Response → Parse Tokens → Classify Channels → Transform to Claude Format → Apply Overrides → Final Response
```

### Configuration
- Optional feature flag: `HARMONY_PARSING_ENABLED` (default: true)
- Debug logging: `HARMONY_DEBUG` for detailed parsing logs
- Compatibility mode: `HARMONY_STRICT_MODE` for error handling behavior

## Success Criteria

### Primary Metrics
- **Parsing Accuracy**: >99% correct classification of Harmony channels
- **Response Latency**: <10ms additional processing time
- **User Experience**: Zero verbose thinking content in main responses
- **Test Coverage**: >95% for all Harmony parsing components

### Secondary Metrics
- **Error Rate**: <0.1% parsing failures
- **Memory Usage**: <5% increase in typical response processing
- **Compatibility**: 100% backward compatibility with existing formats

## Constraints & Assumptions

### Technical Constraints
- Must integrate with existing Go codebase
- Cannot break current request/response transformation pipeline
- Must support streaming responses
- Limited to text-based parsing (no complex rendering)

### Assumptions
- OpenAI Harmony format specification remains stable
- gpt-oss-120b continues using current Harmony format
- Claude Code UI can handle thinking content metadata
- Users want thinking content hidden by default

### Timeline Constraints
- Implementation should not affect existing functionality
- Extensive testing required due to parsing complexity
- Rollout can be gradual with feature flag

## Out of Scope

### Explicitly NOT Building
- Custom Harmony message generation/creation
- Support for Harmony formats beyond message parsing
- UI changes to Claude Code (handled downstream)
- Harmony-specific model routing logic
- Support for Harmony encoding (`o200k_harmony`)

### Future Considerations
- Advanced Harmony features (reasoning levels, complex tool calls)
- Performance optimizations for large Harmony responses
- Integration with other Harmony-compatible models

## Dependencies

### External Dependencies
- OpenAI Harmony specification compliance
- Claude Code UI support for thinking content rendering
- No additional Go libraries required

### Internal Dependencies
- Existing transformation pipeline in `proxy/transform.go`
- Current response type definitions in `types/`
- Test framework and existing unit tests
- Configuration system for feature flags

### Team Dependencies
- Testing: Comprehensive test suite creation
- Documentation: Update API documentation and examples
- Integration: Verification with Claude Code UI team

## Implementation Phases

### Phase 1: Core Parser (Week 1)
- Implement basic Harmony token parsing
- Add channel classification logic
- Create unit tests for parser

### Phase 2: Integration (Week 2)
- Integrate parser with transformation pipeline
- Add configuration options
- Implement error handling

### Phase 3: Testing & Validation (Week 3)
- Comprehensive integration testing
- Performance benchmarking
- Edge case validation

### Phase 4: Documentation & Rollout (Week 4)
- Update documentation
- Create migration guide
- Gradual feature flag rollout