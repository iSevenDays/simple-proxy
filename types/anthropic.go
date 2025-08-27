package types

import "strings"

// AnthropicRequest represents a complete incoming request from Claude Code to the
// proxy service, containing all necessary information for model routing and processing.
//
// This struct serves as the primary input structure for the proxy transformation
// pipeline, containing messages, tools, system instructions, and request parameters
// in Anthropic's native format before conversion to provider-specific formats.
//
// The request structure supports:
//   - Multi-turn conversations through the Messages field
//   - Tool/function calling capabilities through the Tools field
//   - System-level instructions through the System field
//   - Streaming and non-streaming response modes
//   - Token limit controls through MaxTokens
//
// AnthropicRequest is designed to be compatible with Anthropic's official API
// specification while supporting proxy-specific enhancements and routing logic.
type AnthropicRequest struct {
	Model     string          `json:"model"`
	Messages  []Message       `json:"messages"`
	System    []SystemContent `json:"system,omitempty"`
	Tools     []Tool          `json:"tools,omitempty"`
	MaxTokens int             `json:"max_tokens,omitempty"`
	Stream    bool            `json:"stream,omitempty"`
}

// AnthropicResponse represents a complete response from the proxy service back to
// Claude Code, formatted according to Anthropic's API specification with optional
// Harmony parsing enhancements.
//
// This struct serves as the final output of the proxy transformation pipeline,
// containing the processed response from the underlying provider converted back
// to Anthropic format, along with usage statistics and metadata.
//
// The response structure includes:
//   - Standard Anthropic response fields (ID, Type, Role, Model, Content)
//   - Completion metadata (StopReason, StopSequence, Usage)
//   - Harmony parsing extensions (ThinkingContent, HarmonyChannels)
//
// Harmony parsing metadata (Issue #4):
//   - ThinkingContent: Consolidated thinking text from analysis channels
//   - HarmonyChannels: Channel metadata for debugging and validation
//
// The Harmony extensions enable Claude Code to provide enhanced UI experiences
// with separate thinking and response content sections.
type AnthropicResponse struct {
	ID           string    `json:"id"`
	Type         string    `json:"type"`
	Role         string    `json:"role"`
	Model        string    `json:"model"`
	Content      []Content `json:"content"`
	StopReason   string    `json:"stop_reason"`
	StopSequence *string   `json:"stop_sequence"`
	Usage        Usage     `json:"usage"`
	
	// Harmony parsing metadata (Issue #4)
	ThinkingContent string            `json:"thinking_content,omitempty"` // Consolidated thinking text from analysis channels
	HarmonyChannels []HarmonyChannel  `json:"harmony_channels,omitempty"` // Channel metadata for debugging
}

// Message represents a single message within a conversation, supporting both
// simple text content and complex multi-part content structures.
//
// Messages form the core of conversation history in Claude Code interactions,
// with each message having a role identifier and content that can be either:
//   - Simple string content for basic text messages
//   - Complex Content slice for multi-part messages with text and tool interactions
//
// The flexible content structure enables support for:
//   - Plain text messages
//   - Tool use requests and responses
//   - Mixed content types within a single message
//   - Future content type extensions
//
// Role values follow standard chat completion conventions:
// "user", "assistant", "system", "tool", "developer".
type Message struct {
	Role    string      `json:"role"`
	Content interface{} `json:"content"` // Can be string or []Content
}

// SystemContent represents structured system-level instructions or context
// provided to the model, following Anthropic's system message specification.
//
// System content provides instructions, context, or behavioral guidance that
// influences the model's responses without being part of the conversation
// history. Each SystemContent entry has a type identifier and text content.
//
// The structured format enables:
//   - Multiple system instruction types
//   - Modular system message composition
//   - Type-specific processing and validation
//   - Future extension with additional content types
//
// Typically, the Type field is "text" for standard system instructions.
type SystemContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// Content represents individual content blocks within messages, supporting
// text content, tool use requests, and tool result responses.
//
// Content blocks enable rich, multi-part messages that can contain:
//   - Text content for human-readable responses
//   - Tool use requests with function calls and parameters
//   - Tool result responses with execution outcomes
//
// The flexible structure supports Anthropic's content block specification:
//   - Type field identifies the content block type
//   - Text field contains human-readable content
//   - Tool use fields (ID, Name, Input) specify function calls
//   - Tool result field (ToolUseID) links results to requests
//
// This structure enables complex multi-turn tool interactions while maintaining
// compatibility with simple text-only conversations.
type Content struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`

	// Tool use fields
	ID    string                 `json:"id,omitempty"`
	Name  string                 `json:"name,omitempty"`
	Input map[string]interface{} `json:"input,omitempty"`

	// Tool result fields
	ToolUseID string `json:"tool_use_id,omitempty"`
}

// Tool represents a complete tool/function definition in Anthropic format,
// providing all information necessary for the model to understand and use
// the tool appropriately.
//
// Tool definitions enable function calling capabilities by providing:
//   - Name: Unique identifier for the tool
//   - Description: Human-readable explanation of tool purpose and behavior
//   - InputSchema: JSON Schema defining required and optional parameters
//
// The tool definition structure follows Anthropic's tool calling specification,
// ensuring compatibility with Claude's function calling capabilities while
// supporting proxy-specific tool filtering and description overrides.
//
// Tools are used by the model to determine when and how to call functions,
// with the InputSchema providing validation for tool call parameters.
type Tool struct {
	Name        string     `json:"name"`
	Description string     `json:"description"`
	InputSchema ToolSchema `json:"input_schema"`
}

// ToolSchema represents a JSON Schema definition for tool parameters,
// providing structured validation and documentation for tool inputs.
//
// The schema follows standard JSON Schema conventions:
//   - Type: Schema type (typically "object" for tool parameters)
//   - Properties: Map of parameter names to their property definitions
//   - Required: Array of required parameter names
//
// This structure enables:
//   - Parameter validation before tool execution
//   - Auto-generated documentation for tools
//   - IDE support and auto-completion for tool calls
//   - Type safety in tool parameter handling
//
// The schema is used both by the model for understanding tool requirements
// and by the proxy for validating tool call parameters before forwarding.
type ToolSchema struct {
	Type       string                  `json:"type"`
	Properties map[string]ToolProperty `json:"properties"`
	Required   []string                `json:"required"`
}

// ToolProperty represents an individual parameter definition within a tool schema,
// providing detailed specification for tool input validation and documentation.
//
// Each property defines:
//   - Type: Data type (string, number, boolean, array, object)
//   - Description: Human-readable parameter explanation
//   - Items: Schema for array element types (when Type is "array")
//
// ToolProperty enables rich parameter definitions that support complex data types
// and nested structures while providing clear documentation for tool usage.
type ToolProperty struct {
	Type        string               `json:"type"`
	Description string               `json:"description,omitempty"`
	Items       *ToolPropertyItems   `json:"items,omitempty"`
}

// ToolPropertyItems represents the schema definition for elements within
// array-type tool properties, enabling validation of array contents.
//
// This struct provides the element type specification for array parameters,
// ensuring that array contents conform to expected data types and structures.
//
// The Type field specifies the data type for all elements in the array,
// supporting primitive types (string, number, boolean) and complex types
// (object, array) for nested data structures.
type ToolPropertyItems struct {
	Type string `json:"type"`
}

// Usage represents detailed token consumption statistics for a request/response
// cycle, providing billing and performance monitoring information.
//
// Token usage tracking includes:
//   - InputTokens: Tokens consumed by the input prompt and context
//   - OutputTokens: Tokens generated in the response
//
// This information enables:
//   - Cost calculation and billing
//   - Performance monitoring and optimization
//   - Rate limiting and quota management
//   - Usage analytics and reporting
//
// Usage statistics are typically provided by the underlying model provider
// and passed through to Claude Code for display and monitoring.
type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// GetFallbackToolSchema provides comprehensive fallback tool definitions for
// Claude Code's standard toolkit, preventing "Unknown tool" errors when the
// model generates tool calls for valid tools not included in the original request.
//
// This function serves as a critical safety net in the proxy's tool handling
// pipeline, ensuring that tool calls for valid Claude Code tools can be processed
// even when tool restoration from the original request fails or is incomplete.
//
// The function maintains complete, accurate schemas for all standard Claude Code tools:
//   - WebSearch: Web search with domain filtering
//   - Read: File system access and content reading
//   - Write: File creation and modification
//   - Edit: Precise string replacement in files
//   - Bash: Shell command execution
//   - Grep: Advanced pattern searching
//   - Glob: File pattern matching
//   - LS: Directory listing
//   - Task: Agent task delegation
//   - TodoWrite: Task management and tracking
//   - WebFetch: URL content fetching and processing
//   - MultiEdit: Batch file editing operations
//   - ExitPlanMode: Planning workflow control
//
// Parameters:
//   - toolName: The name of the tool to retrieve (case-insensitive)
//
// Returns:
//   - A complete Tool struct with accurate schema for the specified tool
//   - nil if the tool name is not recognized
//
// Performance: O(1) constant time lookup with switch statement.
//
// Usage:
// This function is typically called by the correction service when a tool
// call cannot be validated against the original request's tool list, providing
// a fallback mechanism to maintain tool call functionality.
//
// Example:
//
//	tool := GetFallbackToolSchema("WebSearch")
//	if tool != nil {
//		// Use tool schema for validation or correction
//		validateToolCall(toolCall, tool.InputSchema)
//	}
func GetFallbackToolSchema(toolName string) *Tool {
	// Complete coverage of Claude Code tools to prevent "Unknown tool" errors
	switch strings.ToLower(toolName) {
	case "websearch", "web_search":
		return &Tool{
			Name:        "WebSearch",
			Description: "\n- Allows Claude to search the web and use the results to inform responses\n- Provides up-to-date information for current events and recent data\n- Returns search result information formatted as search result blocks\n- Use this tool for accessing information beyond Claude's knowledge cutoff\n- Searches are performed automatically within a single API call\n\nUsage notes:\n  - Domain filtering is supported to include or block specific websites\n  - Web search is only available in the US\n  - Account for \"Today's date\" in <env>. For example, if <env> says \"Today's date: 2025-07-01\", and the user wants the latest docs, do not use 2024 in the search query. Use 2025.\n",
			InputSchema: ToolSchema{
				Type: "object",
				Properties: map[string]ToolProperty{
					"allowed_domains": {
						Type:        "array",
						Description: "Only include search results from these domains",
						Items:       &ToolPropertyItems{Type: "string"},
					},
					"blocked_domains": {
						Type:        "array",
						Description: "Never include search results from these domains",
						Items:       &ToolPropertyItems{Type: "string"},
					},
					"query": {
						Type:        "string",
						Description: "The search query to use",
					},
				},
				Required: []string{"query"},
			},
		}
	case "read", "read_file":
		return &Tool{
			Name:        "Read",
			Description: "Reads a file from the local filesystem. You can access any file directly by using this tool.\nAssume this tool is able to read all files on the machine. If the User provides a path to a file assume that path is valid. It is okay to read a file that does not exist; an error will be returned.\n\nUsage:\n- The file_path parameter must be an absolute path, not a relative path\n- By default, it reads up to 2000 lines starting from the beginning of the file\n- You can optionally specify a line offset and limit (especially handy for long files), but it's recommended to read the whole file by not providing these parameters\n- Any lines longer than 2000 characters will be truncated\n- Results are returned using cat -n format, with line numbers starting at 1\n- This tool allows Claude Code to read images (eg PNG, JPG, etc). When reading an image file the contents are presented visually as Claude Code is a multimodal LLM.\n- This tool can read PDF files (.pdf). PDFs are processed page by page, extracting both text and visual content for analysis.\n- This tool can read Jupyter notebooks (.ipynb files) and returns all cells with their outputs, combining code, text, and visualizations.\n- You have the capability to call multiple tools in a single response. It is always better to speculatively read multiple files as a batch that are potentially useful. \n- You will regularly be asked to read screenshots. If the user provides a path to a screenshot ALWAYS use this tool to view the file at the path. This tool will work with all temporary file paths like /var/folders/123/abc/T/TemporaryItems/NSIRD_screencaptureui_ZfB1tD/Screenshot.png\n- If you read a file that exists but has empty contents you will receive a system reminder warning in place of file contents.",
			InputSchema: ToolSchema{
				Type: "object",
				Properties: map[string]ToolProperty{
					"file_path": {
						Type:        "string",
						Description: "The absolute path to the file to read",
					},
					"limit": {
						Type:        "number",
						Description: "The number of lines to read. Only provide if the file is too large to read at once.",
					},
					"offset": {
						Type:        "number",
						Description: "The line number to start reading from. Only provide if the file is too large to read at once",
					},
				},
				Required: []string{"file_path"},
			},
		}
	case "write", "write_file":
		return &Tool{
			Name:        "Write",
			Description: "Writes a file to the local filesystem.\n\nUsage:\n- This tool will overwrite the existing file if there is one at the provided path.\n- If this is an existing file, you MUST use the Read tool first to read the file's contents. This tool will fail if you did not read the file first.\n- ALWAYS prefer editing existing files in the codebase. NEVER write new files unless explicitly required.\n- NEVER proactively create documentation files (*.md) or README files. Only create documentation files if explicitly requested by the User.\n- Only use emojis if the user explicitly requests it. Avoid writing emojis to files unless asked.",
			InputSchema: ToolSchema{
				Type: "object",
				Properties: map[string]ToolProperty{
					"content": {
						Type:        "string",
						Description: "The content to write to the file",
					},
					"file_path": {
						Type:        "string",
						Description: "The absolute path to the file to write (must be absolute, not relative)",
					},
				},
				Required: []string{"file_path", "content"},
			},
		}
	case "edit":
		return &Tool{
			Name:        "Edit",
			Description: "Performs exact string replacements in files. \n\nUsage:\n- You must use your `Read` tool at least once in the conversation before editing. This tool will error if you attempt an edit without reading the file. \n- When editing text from Read tool output, ensure you preserve the exact indentation (tabs/spaces) as it appears AFTER the line number prefix. The line number prefix format is: spaces + line number + tab. Everything after that tab is the actual file content to match. Never include any part of the line number prefix in the old_string or new_string.\n- ALWAYS prefer editing existing files in the codebase. NEVER write new files unless explicitly required.\n- Only use emojis if the user explicitly requests it. Avoid adding emojis to files unless asked.\n- The edit will FAIL if `old_string` is not unique in the file. Either provide a larger string with more surrounding context to make it unique or use `replace_all` to change every instance of `old_string`. \n- Use `replace_all` for replacing and renaming strings across the file. This parameter is useful if you want to rename a variable for instance.",
			InputSchema: ToolSchema{
				Type: "object",
				Properties: map[string]ToolProperty{
					"file_path": {
						Type:        "string",
						Description: "The absolute path to the file to modify",
					},
					"new_string": {
						Type:        "string",
						Description: "The text to replace it with (must be different from old_string)",
					},
					"old_string": {
						Type:        "string",
						Description: "The text to replace",
					},
					"replace_all": {
						Type:        "boolean",
						Description: "Replace all occurences of old_string (default false)",
					},
				},
				Required: []string{"file_path", "old_string", "new_string"},
			},
		}
	case "bash", "bash_command":
		return &Tool{
			Name:        "Bash",
			Description: "Executes bash commands",
			InputSchema: ToolSchema{
				Type: "object",
				Properties: map[string]ToolProperty{
					"command": {
						Type:        "string",
						Description: "The command to execute",
					},
					"description": {
						Type:        "string",
						Description: "Description of what the command does",
					},
					"timeout": {
						Type:        "number",
						Description: "Optional timeout in milliseconds",
					},
				},
				Required: []string{"command"},
			},
		}
	case "grep", "grep_search":
		return &Tool{
			Name:        "Grep",
			Description: "A powerful search tool built on ripgrep",
			InputSchema: ToolSchema{
				Type: "object",
				Properties: map[string]ToolProperty{
					"pattern": {
						Type:        "string",
						Description: "The regular expression pattern to search for",
					},
					"path": {
						Type:        "string",
						Description: "File or directory to search in",
					},
					"glob": {
						Type:        "string",
						Description: "Glob pattern to filter files",
					},
					"type": {
						Type:        "string",
						Description: "File type to search (js, py, rust, go, etc.)",
					},
					"output_mode": {
						Type:        "string",
						Description: "Output mode: content, files_with_matches, count",
					},
				},
				Required: []string{"pattern"},
			},
		}
	case "glob":
		return &Tool{
			Name:        "Glob",
			Description: "Fast file pattern matching tool",
			InputSchema: ToolSchema{
				Type: "object",
				Properties: map[string]ToolProperty{
					"pattern": {
						Type:        "string",
						Description: "The glob pattern to match files against",
					},
					"path": {
						Type:        "string",
						Description: "The directory to search in",
					},
				},
				Required: []string{"pattern"},
			},
		}
	case "ls":
		return &Tool{
			Name:        "LS",
			Description: "Lists files and directories",
			InputSchema: ToolSchema{
				Type: "object",
				Properties: map[string]ToolProperty{
					"path": {
						Type:        "string",
						Description: "The absolute path to the directory to list",
					},
					"ignore": {
						Type:        "array",
						Description: "List of glob patterns to ignore",
						Items:       &ToolPropertyItems{Type: "string"},
					},
				},
				Required: []string{"path"},
			},
		}
	case "task":
		return &Tool{
			Name:        "Task",
			Description: "Launch a new agent to handle complex tasks",
			InputSchema: ToolSchema{
				Type: "object",
				Properties: map[string]ToolProperty{
					"description": {
						Type:        "string",
						Description: "A short description of the task",
					},
					"prompt": {
						Type:        "string",
						Description: "The task for the agent to perform",
					},
				},
				Required: []string{"description", "prompt"},
			},
		}
	case "todowrite":
		return &Tool{
			Name:        "TodoWrite",
			Description: "Use this tool to create and manage a structured task list for your current coding session. This helps you track progress, organize complex tasks, and demonstrate thoroughness to the user.\nIt also helps the user understand the progress of the task and overall progress of their requests.\n\n## When to Use This Tool\nUse this tool proactively in these scenarios:\n\n1. Complex multi-step tasks - When a task requires 3 or more distinct steps or actions\n2. Non-trivial and complex tasks - Tasks that require careful planning or multiple operations\n3. User explicitly requests todo list - When the user directly asks you to use the todo list\n4. User provides multiple tasks - When users provide a list of things to be done (numbered or comma-separated)\n5. After receiving new instructions - Immediately capture user requirements as todos\n6. When you start working on a task - Mark it as in_progress BEFORE beginning work. Ideally you should only have one todo as in_progress at a time\n7. After completing a task - Mark it as completed and add any new follow-up tasks discovered during implementation\n\n## When NOT to Use This Tool\n\nSkip using this tool when:\n1. There is only a single, straightforward task\n2. The task is trivial and tracking it provides no organizational benefit\n3. The task can be completed in less than 3 trivial steps\n4. The task is purely conversational or informational\n\nNOTE that you should not use this tool if there is only one trivial task to do. In this case you are better off just doing the task directly.\n\n## Examples of When to Use the Todo List\n\n<example>\nUser: I want to add a dark mode toggle to the application settings. Make sure you run the tests and build when you're done!\nAssistant: I'll help add a dark mode toggle to your application settings. Let me create a todo list to track this implementation.\n*Creates todo list with the following items:*\n1. Create dark mode toggle component in Settings page\n2. Add dark mode state management (context/store)\n3. Implement CSS-in-JS styles for dark theme\n4. Update existing components to support theme switching\n5. Run tests and build process, addressing any failures or errors that occur\n*Begins working on the first task*\n\n<reasoning>\nThe assistant used the todo list because:\n1. Adding dark mode is a multi-step feature requiring UI, state management, and styling changes\n2. The user explicitly requested tests and build be run afterward\n3. The assistant inferred that tests and build need to pass by adding \"Ensure tests and build succeed\" as the final task\n</reasoning>\n</example>\n\n<example>\nUser: Help me rename the function getCwd to getCurrentWorkingDirectory across my project\nAssistant: Let me first search through your codebase to find all occurrences of 'getCwd'.\n*Uses grep or search tools to locate all instances of getCwd in the codebase*\nAssistant: I've found 15 instances of 'getCwd' across 8 different files. Let me create a todo list to track these changes.\n*Creates todo list with specific items for each file that needs updating*\n\n<reasoning>\nThe assistant used the todo list because:\n1. First, the assistant searched to understand the scope of the task\n2. Upon finding multiple occurrences across different files, it determined this was a complex task with multiple steps\n3. The todo list helps ensure every instance is tracked and updated systematically\n4. This approach prevents missing any occurrences and maintains code consistency\n</reasoning>\n</example>\n\n\n<example>\nUser: I need to implement these features for my e-commerce site: user registration, product catalog, shopping cart, and checkout flow.\nAssistant: I'll help implement these features. First, let's add all the features to the todo list.\n*Creates a todo list breaking down each feature into specific tasks based on the project architecture*\nAssistant: Let's start with user registration. This will involve creating database models, API endpoints, and frontend forms that integrate with your existing authentication system.\n\n<reasoning>\nThe assistant used the todo list because:\n1. The user provided multiple complex features to implement in a comma separated list\n2. The todo list helps organize these large features into manageable tasks\n3. This approach allows for tracking progress across the entire implementation\n</reasoning>\n</example>\n\n<example>\nUser: Can you help optimize my React application? It's rendering slowly and has performance issues.</user>\nAssistant: I'll help optimize your React application. First, let me examine your codebase to identify potential performance bottlenecks.\n*Reviews component structure, render patterns, state management, and data fetching*\nAssistant: After analyzing your codebase, I've identified several performance issues. Let me create a todo list to track our optimization efforts.\n*Creates todo list with items like: 1) Implement memoization for expensive calculations in ProductList, 2) Add virtualization for long lists in Dashboard, 3) Optimize image loading in Gallery component, 4) Fix state update loops in ShoppingCart, 5) Review bundle size and implement code splitting*\nLet's start by implementing memoization for the expensive calculations in your ProductList component.</assistant>\n\n<reasoning>\nThe assistant used the todo list because:\n1. First, the assistant examined the codebase to identify specific performance issues\n2. Based on this analysis, it identified multiple optimization opportunities\n3. Performance optimization is a non-trivial task requiring multiple steps\n4. The todo list helps methodically track improvements across different components\n5. This systematic approach ensures all performance bottlenecks are addressed\n</reasoning>\n</example>\n\n## Examples of When NOT to Use the Todo List\n\n<example>\nUser: How do I print 'Hello World' in Python?\nAssistant: In Python, you can print \"Hello World\" with this simple code:\n\npython\nprint(\"Hello World\")\n\nThis will output the text \"Hello World\" to the console when executed.</assistant>\n\n<reasoning>\nThe assistant did not use the todo list because this is a single, trivial task that can be completed in one step. There's no need to track multiple tasks or steps for such a straightforward request.\n</reasoning>\n</example>\n\n<example>\nUser: What does the git status command do?\nAssistant: The git status command shows the current state of your working directory and staging area. It displays which changes have been staged, which haven't, and which files aren't being tracked by Git. This is a fundamental command for understanding the current state of your repository before making commits.\n\n<reasoning>\nThe assistant did not use the todo list because this is an informational request with no actual coding task to complete. The user is simply asking for an explanation, not for the assistant to perform multiple steps or tasks.\n</reasoning>\n</example>\n\n<example>\nUser: Can you add a comment to the calculateTotal function to explain what it does?\nAssistant: Sure, let me add a comment to the calculateTotal function to explain what it does.\n* Uses the Edit tool to add a comment to the calculateTotal function *\n\n<reasoning>\nThe assistant did not use the todo list because this is a single, straightforward task confined to one location in the code. Adding a comment doesn't require tracking multiple steps or systematic organization.\n</reasoning>\n</example>\n\n<example>\nUser: Run npm install for me and tell me what happens.\nAssistant: I'll run the npm install command for you.\n\n*Executes: npm install*\n\nThe command completed successfully. Here's the output:\n[Output of npm install command]\n\nAll dependencies have been installed according to your package.json file.\n\n<reasoning>\nThe assistant did not use the todo list because this is a single command execution with immediate results. There are no multiple steps to track or organize, making the todo list unnecessary for this straightforward task.\n</reasoning>\n</example>\n\n## Task States and Management\n\n1. **Task States**: Use these states to track progress:\n   - pending: Task not yet started\n   - in_progress: Currently working on (limit to ONE task at a time)\n   - completed: Task finished successfully\n\n2. **Task Management**:\n   - Update task status in real-time as you work\n   - Mark tasks complete IMMEDIATELY after finishing (don't batch completions)\n   - Only have ONE task in_progress at any time\n   - Complete current tasks before starting new ones\n   - Remove tasks that are no longer relevant from the list entirely\n\n3. **Task Completion Requirements**:\n   - ONLY mark a task as completed when you have FULLY accomplished it\n   - If you encounter errors, blockers, or cannot finish, keep the task as in_progress\n   - When blocked, create a new task describing what needs to be resolved\n   - Never mark a task as completed if:\n     - Tests are failing\n     - Implementation is partial\n     - You encountered unresolved errors\n     - You couldn't find necessary files or dependencies\n\n4. **Task Breakdown**:\n   - Create specific, actionable items\n   - Break complex tasks into smaller, manageable steps\n   - Use clear, descriptive task names\n\nWhen in doubt, use this tool. Being proactive with task management demonstrates attentiveness and ensures you complete all requirements successfully.\n",
			InputSchema: ToolSchema{
				Type: "object",
				Properties: map[string]ToolProperty{
					"todos": {
						Type:        "array",
						Description: "The updated todo list",
						Items:       &ToolPropertyItems{Type: "object"},
					},
				},
				Required: []string{"todos"},
			},
		}
	case "webfetch":
		return &Tool{
			Name:        "WebFetch",
			Description: "\n- Fetches content from a specified URL and processes it using an AI model\n- Takes a URL and a prompt as input\n- Fetches the URL content, converts HTML to markdown\n- Processes the content with the prompt using a small, fast model\n- Returns the model's response about the content\n- Use this tool when you need to retrieve and analyze web content\n\nUsage notes:\n  - IMPORTANT: If an MCP-provided web fetch tool is available, prefer using that tool instead of this one, as it may have fewer restrictions. All MCP-provided tools start with \"mcp__\".\n  - The URL must be a fully-formed valid URL\n  - HTTP URLs will be automatically upgraded to HTTPS\n  - The prompt should describe what information you want to extract from the page\n  - This tool is read-only and does not modify any files\n  - Results may be summarized if the content is very large\n  - Includes a self-cleaning 15-minute cache for faster responses when repeatedly accessing the same URL\n  - When a URL redirects to a different host, the tool will inform you and provide the redirect URL in a special format. You should then make a new WebFetch request with the redirect URL to fetch the content.\n",
			InputSchema: ToolSchema{
				Type: "object",
				Properties: map[string]ToolProperty{
					"prompt": {
						Type:        "string",
						Description: "The prompt to run on the fetched content",
					},
					"url": {
						Type:        "string",
						Description: "The URL to fetch content from",
					},
				},
				Required: []string{"url", "prompt"},
			},
		}
	case "multiedit", "multi_edit":
		return &Tool{
			Name:        "MultiEdit",
			Description: "This is a tool for making multiple edits to a single file in one operation. It is built on top of the Edit tool and allows you to perform multiple find-and-replace operations efficiently. Prefer this tool over the Edit tool when you need to make multiple edits to the same file.\n\nBefore using this tool:\n\n1. Use the Read tool to understand the file's contents and context\n2. Verify the directory path is correct\n\nTo make multiple file edits, provide the following:\n1. file_path: The absolute path to the file to modify (must be absolute, not relative)\n2. edits: An array of edit operations to perform, where each edit contains:\n   - old_string: The text to replace (must match the file contents exactly, including all whitespace and indentation)\n   - new_string: The edited text to replace the old_string\n   - replace_all: Replace all occurences of old_string. This parameter is optional and defaults to false.\n\nIMPORTANT:\n- All edits are applied in sequence, in the order they are provided\n- Each edit operates on the result of the previous edit\n- All edits must be valid for the operation to succeed - if any edit fails, none will be applied\n- This tool is ideal when you need to make several changes to different parts of the same file\n- For Jupyter notebooks (.ipynb files), use the NotebookEdit instead\n\nCRITICAL REQUIREMENTS:\n1. All edits follow the same requirements as the single Edit tool\n2. The edits are atomic - either all succeed or none are applied\n3. Plan your edits carefully to avoid conflicts between sequential operations\n\nWARNING:\n- The tool will fail if edits.old_string doesn't match the file contents exactly (including whitespace)\n- The tool will fail if edits.old_string and edits.new_string are the same\n- Since edits are applied in sequence, ensure that earlier edits don't affect the text that later edits are trying to find\n\nWhen making edits:\n- Ensure all edits result in idiomatic, correct code\n- Do not leave the code in a broken state\n- Always use absolute file paths (starting with /)\n- Only use emojis if the user explicitly requests it. Avoid adding emojis to files unless asked.\n- Use replace_all for replacing and renaming strings across the file. This parameter is useful if you want to rename a variable for instance.\n\nIf you want to create a new file, use:\n- A new file path, including dir name if needed\n- First edit: empty old_string and the new file's contents as new_string\n- Subsequent edits: normal edit operations on the created content",
			InputSchema: ToolSchema{
				Type: "object",
				Properties: map[string]ToolProperty{
					"edits": {
						Type:        "array",
						Description: "Array of edit operations to perform sequentially on the file",
						Items:       &ToolPropertyItems{Type: "object"},
					},
					"file_path": {
						Type:        "string",
						Description: "The absolute path to the file to modify",
					},
				},
				Required: []string{"file_path", "edits"},
			},
		}
	case "exitplanmode", "exit_plan_mode":
		return &Tool{
			Name:        "ExitPlanMode",
			Description: "Use this tool when you are in plan mode and have finished presenting your plan and are ready to code. This will prompt the user to exit plan mode. \nIMPORTANT: Only use this tool when the task requires planning the implementation steps of a task that requires writing code. For research tasks where you're gathering information, searching files, reading files or in general trying to understand the codebase - do NOT use this tool.\n\nEg. \n1. Initial task: \"Search for and understand the implementation of vim mode in the codebase\" - Do not use the exit plan mode tool because you are not planning the implementation steps of a task.\n2. Initial task: \"Help me implement yank mode for vim\" - Use the exit plan mode tool after you have finished planning the implementation steps of the task.\n",
			InputSchema: ToolSchema{
				Type: "object",
				Properties: map[string]ToolProperty{
					"plan": {
						Type:        "string",
						Description: "The plan you came up with, that you want to run by the user for approval. Supports markdown. The plan should be pretty concise.",
					},
				},
				Required: []string{"plan"},
			},
		}
	default:
		return nil
	}
}
