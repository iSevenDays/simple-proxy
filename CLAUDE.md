# Simple Proxy - AI Development Framework

## Project Overview

Simple Proxy is a Claude Code proxy that transforms Anthropic API requests to OpenAI-compatible format with extensive customization capabilities. It provides comprehensive request/response transformation, tool customization, system message overrides, and intelligent model routing.

**📋 Architecture**: See [ARCHITECTURE.md](./ARCHITECTURE.md) for detailed system architecture, component interactions, and data flow diagrams.

## SPARC Framework

### Core Philosophy

1. **Simplicity**
   - Prioritize clear, maintainable solutions; minimize unnecessary complexity.

2. **Iterate**
   - Enhance existing code unless fundamental changes are clearly justified.

3. **Focus**
   - Stick strictly to defined tasks; avoid unrelated scope changes.

4. **Quality**
   - Deliver clean, well-tested, documented, and secure outcomes through structured workflows.

5. **Collaboration**
   - Foster effective teamwork between human developers and autonomous agents.

### Methodology & Workflow

- **Structured Workflow**
  - Follow clear phases from specification through deployment.
- **Flexibility**
  - Adapt processes to diverse project sizes and complexity levels.
- **Intelligent Evolution**
  - Continuously improve codebase using advanced symbolic reasoning and adaptive complexity management.
- **Conscious Integration**
  - Incorporate reflective awareness at each development stage.

### Agentic Integration

- **Agent Configuration**
  - Embed concise, workspace-specific rules to guide autonomous behaviors, prompt designs, and contextual decisions.
  - Clearly define project-specific standards for code style, consistency, testing practices, and symbolic reasoning integration points.

## Context Preservation

- **Persistent Context**
  - Continuously retain relevant context across development stages to ensure coherent long-term planning and decision-making.
- **Reference Prior Decisions**
  - Regularly review past decisions stored in memory to maintain consistency and reduce redundancy.
- **Adaptive Learning**
  - Utilize historical data and previous solutions to adaptively refine new implementations.

### Track Across Iterations:
- Original requirements and any changes
- Key decisions made and rationale
- Human feedback and how it was incorporated
- Alternative approaches considered

### Maintain Session Context:
**Problem:** [brief description + problem scope]
**Requirements:** [key requirements]
**Decisions:** [key decisions with rationale and trade-offs]
**Status:** [progress/blockers/next actions]

## Project Context & Understanding

### Architecture Reference
- **System Architecture**: [ARCHITECTURE.md](./ARCHITECTURE.md) - Complete system architecture documentation
- **Component Interactions**: Request/response transformation pipeline
- **Configuration Systems**: Multi-source configuration with overrides
- **Data Flow**: Detailed request and response processing flows

### Documentation First
Review essential documentation before implementation:
- [ARCHITECTURE.md](./ARCHITECTURE.md) - System architecture and design
- [README.md](./README.md) - Project overview and setup (if exists)
- Configuration files: `.env.example`, `tools_override.yaml`, `system_overrides.yaml`

## Project-Specific Architecture

### Component Overview

**Core Components:**
- **Configuration System** (`config/`) - Multi-source configuration management
- **Request/Response Transformation** (`proxy/transform.go`) - Format conversion
- **Model Routing** (`proxy/handler.go`) - Intelligent model routing
- **Tool Customization** - Tool description overrides and filtering
- **System Message Overrides** - Regex-based system message modifications
- **Correction Service** (`correction/`) - Tool call validation and correction

### Data Flow Architecture

```
Claude Code → Request Transform → Model Router → Provider → Response Transform → Claude Code
     ↑              ↑                 ↑             ↑             ↑              ↑
Configuration   Tool Overrides   Model Config   Provider    Tool Correction  Final Response
```

### Key Configuration Files

- **`.env`** - Model endpoints, API keys, debug options
- **`tools_override.yaml`** - Custom tool descriptions
- **`system_overrides.yaml`** - System message modifications

## Workspace-Specific Rules

### Go Development Standards

1. **Code Organization**
   - Package structure follows domain boundaries
   - Clear separation of concerns (config, proxy, types, correction)
   - Comprehensive error handling with detailed logging

2. **Type Safety**
   - Strong typing for all API transformations
   - Proper JSON marshaling/unmarshaling
   - Schema validation for tool calls

3. **Testing Strategy**
   - Unit tests for all transformation logic
   - Integration tests for complete request flows
   - TDD approach for new features

4. **Logging Standards**
   - Structured logging with request ID tracking
   - Different log levels for different component types
   - Security-conscious logging (API key masking)

### Configuration Management

1. **Environment Variables (.env)**
   - All model configurations and API keys
   - Optional feature flags
   - Debug and development settings

2. **YAML Overrides**
   - Tool description customization
   - System message modification rules
   - Graceful fallback when files don't exist

### Request/Response Transformation

1. **Format Conversion**
   - Anthropic ↔ OpenAI API format transformation
   - Preserve semantic meaning across formats
   - Handle edge cases and malformed data

2. **Tool Processing**
   - Filter unwanted tools based on configuration
   - Apply custom tool descriptions
   - Validate tool call parameters

3. **System Message Processing**
   - Apply regex-based pattern removal
   - Execute find/replace operations
   - Add custom prepend/append content

## Code Quality & Testing

### Testing Framework
- **Unit Tests**: All transformation and configuration logic
- **Integration Tests**: Complete request/response cycles
- **TDD Approach**: Write tests first, implement to pass
- **Comprehensive Coverage**: Critical paths and edge cases

### Code Standards
1. **Clarity and Readability**
   - Self-documenting code with clear variable names
   - Comprehensive comments for complex logic
   - Consistent formatting and structure

2. **Error Handling**
   - Graceful degradation when possible
   - Detailed error logging with context
   - Clean error responses to clients

3. **Security Practices**
   - API key masking in logs
   - Input validation and sanitization
   - Secure configuration management

## Advanced Features

### Tool Customization System
- **Dynamic Tool Filtering**: Skip tools based on configuration
- **Description Overrides**: Replace tool descriptions with custom content
- **YAML-Based Configuration**: Easy modification without code changes

### System Message Override System
- **Pattern-Based Removal**: Regex patterns to remove unwanted content
- **Text Replacement**: Find/replace operations for branding
- **Content Addition**: Prepend/append custom instructions
- **Detailed Logging**: Track all modifications applied

### Model Routing Intelligence
- **Size-Based Routing**: Route to appropriate model based on Claude model type
- **Provider Flexibility**: Support multiple OpenAI-compatible providers
- **Configuration-Driven**: Easy provider changes via environment variables

### Streaming Support
- **Efficient Processing**: Handle streaming responses from providers
- **Response Reconstruction**: Assemble complete responses from chunks
- **Tool Call Handling**: Proper streaming tool call reconstruction

## Development Workflow

### Feature Development Process
1. **Architecture Review**: Check [ARCHITECTURE.md](./ARCHITECTURE.md) for system understanding
2. **TDD Implementation**: Write tests first, implement to pass
3. **Configuration Integration**: Update configuration system if needed
4. **Documentation Updates**: Update architecture docs for significant changes
5. **Integration Testing**: Verify complete request/response flows

### Testing Strategy
- **Unit Tests**: Individual component testing
- **Integration Tests**: Full system flow testing
- **Configuration Tests**: All configuration combinations
- **Error Handling Tests**: Failure scenarios and recovery

### Deployment Considerations
- **Environment Configuration**: Proper `.env` file setup
- **Override Files**: Optional YAML files for customization
- **Logging Configuration**: Appropriate log levels for production
- **Health Monitoring**: Built-in health check endpoints

## Important Notes

### Configuration Requirements
- All model configurations are required in `.env` file
- YAML override files are optional but provide powerful customization
- Environment variable validation ensures required settings are present

### Security Considerations
- API keys are masked in all log output
- Configuration files should be excluded from version control
- Input validation prevents injection attacks

### Performance Optimization
- Stateless design enables horizontal scaling
- Efficient streaming response handling
- Model-specific routing reduces unnecessary load