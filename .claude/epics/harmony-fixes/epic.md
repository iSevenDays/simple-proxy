---
epic: harmony-fixes
prd: harmony-fixes
status: active
priority: high
progress: 25%
created: 2025-08-28
updated: 2025-08-29T11:36:07Z
sprint_duration: 4_weeks
github: https://github.com/iSevenDays/simple-proxy/issues/14
---

# Epic: Harmony Format Parsing Fixes

**PRD Reference**: [harmony-fixes PRD](.claude/prds/harmony-fixes.md)

## Epic Overview

Fix critical Harmony content parsing where channel extraction fails, causing raw structural tokens (`<|start|>`, `<|channel|>`, `<|message|>`, `<|return|>`) to appear in Claude Code UI instead of clean, formatted content.

## Problem Context

- **Root Cause**: `proxy/transform.go` Harmony parser fails to extract clean content from `final` channel
- **Impact**: User experience degradation with visible structural tokens and lost formatting
- **Evidence**: TDD test successfully reproduces issue in `test/newline_formatting_test.go`

## Key Technical Findings

From TDD analysis:
- Harmony parsing recognizes format but fails to extract clean content
- Expected: Clean markdown with preserved newlines
- Actual: Raw tokens like `<|start|>assistant<|channel|>final<|message|>content<|return|>`
- Newlines are preserved (same count) but content extraction is broken

## Sprint Breakdown

### Sprint 1: Foundation & Analysis (Week 1)
**Goal**: Deep dive analysis and comprehensive test coverage

**Issues**:
1. **Analyze Current Harmony Parsing Implementation**
   - Review `proxy/transform.go` Harmony logic
   - Identify specific failure points in content extraction
   - Document current vs expected behavior patterns

2. **Expand TDD Test Suite**
   - Extend `test/newline_formatting_test.go` with more edge cases
   - Add tests for all three channels (analysis, commentary, final)
   - Test malformed Harmony format handling
   - Performance benchmarks for parsing overhead

3. **Design Enhanced Parsing Architecture**
   - Design improved channel content extraction logic
   - Plan backward compatibility strategy
   - Define error handling and fallback mechanisms

### Sprint 2: Core Implementation (Week 2)  
**Goal**: Implement fixed Harmony parsing with clean content extraction

**Issues**:
4. **Implement Enhanced Harmony Pattern Recognition**
   - Update regex patterns to match full OpenAI Harmony specification
   - Support `<|start|>assistant<|channel|>TYPE<|message|>CONTENT<|return|>` format
   - Handle both `<|return|>` and `<|end|>` termination tokens

5. **Build Channel-Specific Content Extraction**
   - Implement clean extraction from `final` channel (priority)
   - Add support for `analysis` and `commentary` channels
   - Strip all structural tokens while preserving content formatting

6. **Integrate Content Cleaning Pipeline**
   - Add content validation and integrity checks
   - Ensure newlines, whitespace, and Unicode preservation  
   - Implement graceful fallback when extraction fails

### Sprint 3: Testing & Validation (Week 3)
**Goal**: Comprehensive testing and performance validation

**Issues**:
7. **Execute Comprehensive Test Suite**
   - Run all existing tests to ensure zero regression
   - Validate all new TDD tests pass
   - Test edge cases and malformed input handling

8. **Performance Testing & Optimization**  
   - Benchmark parsing overhead (<10ms target)
   - Memory usage analysis for large responses
   - Optimize critical paths if needed

9. **Integration Testing with Real Content**
   - Test with actual Harmony-formatted responses from LLM providers
   - Validate Claude Code UI displays content correctly
   - User experience validation with formatted markdown

### Sprint 4: Deployment & Monitoring (Week 4)
**Goal**: Production deployment with comprehensive monitoring

**Issues**:
10. **Production Deployment**
    - Deploy with monitoring and rollback capability
    - Configure detailed logging for parsing success/failure
    - Set up performance monitoring dashboards

11. **User Experience Validation**
    - Collect user feedback on improved formatting
    - Monitor error rates and parsing success metrics
    - Validate elimination of visible structural tokens

12. **Documentation & Knowledge Transfer**
    - Update architecture documentation with new parsing logic
    - Create troubleshooting guide for Harmony parsing issues
    - Document configuration and monitoring procedures

## Success Criteria

### Must Have (Sprint 1-2)
- [ ] TDD test `TestNewlineFormattingInHarmonyContent` passes 100%
- [ ] Clean content extraction from `final` channel without structural tokens
- [ ] All newlines and markdown formatting preserved exactly
- [ ] Zero regression in existing non-Harmony content processing

### Should Have (Sprint 3)
- [ ] Performance impact <10ms per response
- [ ] ≥95% test coverage for all parsing functions
- [ ] Graceful handling of malformed Harmony content
- [ ] Detailed error logging for debugging

### Could Have (Sprint 4)
- [ ] Support for all three Harmony channels with priority
- [ ] Enhanced error messages for debugging
- [ ] Performance metrics collection and monitoring

## Risk Mitigation

### High Priority Risks
1. **Regression Risk**: Break existing non-Harmony parsing
   - *Mitigation*: Comprehensive regression test suite
   
2. **Performance Impact**: Complex parsing adds latency
   - *Mitigation*: Performance benchmarks and optimization
   
3. **Edge Case Failures**: Unforeseen Harmony variations
   - *Mitigation*: Extensive edge case testing and graceful fallbacks

## Definition of Done

**Epic Complete When**:
- [ ] All visible Harmony structural tokens eliminated from Claude Code UI
- [ ] Newlines and formatting preserved perfectly in extracted content
- [ ] All existing functionality unaffected (zero regression)
- [ ] Performance impact <1% average response time increase
- [ ] ≥95% test coverage achieved and maintained
- [ ] Production monitoring shows >99.9% parsing success rate

## Dependencies

- **Technical**: `parser/harmony.go` utilities and `proxy/transform.go` pipeline
- **Testing**: Existing test framework and TDD test infrastructure  
- **Deployment**: Monitoring and logging systems
- **Validation**: Claude Code UI compatibility with formatted responses

## Next Steps

1. **Immediate**: Start Sprint 1 with deep analysis of current parsing logic
2. **Week 1**: Complete foundation work and expand test coverage
3. **Week 2**: Core implementation of enhanced parsing
4. **Week 3-4**: Testing, validation, and production deployment

---

**Epic Owner**: Development Team  
**Stakeholders**: Claude Code Users, System Administrators  
**Priority**: High (User Experience Critical)  
**Estimated Effort**: 4 weeks / 12 issues