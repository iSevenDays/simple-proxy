package logger

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// ConversationLogger handles full conversation logging to files
type ConversationLogger struct {
	sessionID     string
	logFile       *os.File
	mu            sync.Mutex
	enabled       bool
	logLevel      Level
	maskSensitive bool
	logFullTools  bool
	truncation    int
}

// ConversationConfig holds configuration for conversation logging
type ConversationConfig struct {
	Enabled       bool
	LogLevel      Level
	MaskSensitive bool
	LogFullTools  bool
	Truncation    int
	LogDir        string
}

// conversationLoggerInstance holds the global conversation logger
var conversationLoggerInstance *ConversationLogger
var conversationLoggerOnce sync.Once

// NewConversationLogger creates a new conversation logger
func NewConversationLogger(logDir string, logLevel Level, maskSensitive bool, logFullTools bool, truncation int) (*ConversationLogger, error) {
	sessionID := generateSessionID()
	timestamp := time.Now().Format("20060102-150405")
	filename := fmt.Sprintf("conversation-%s-%s.log", sessionID, timestamp)
	
	// Ensure log directory exists
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create log directory: %v", err)
	}
	
	// Create log file
	filePath := filepath.Join(logDir, filename)
	logFile, err := os.Create(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to create log file: %v", err)
	}
	
	cl := &ConversationLogger{
		sessionID:     sessionID,
		logFile:       logFile,
		enabled:       true,
		logLevel:      logLevel,
		maskSensitive: maskSensitive,
		logFullTools:  logFullTools,
		truncation:    truncation,
	}
	
	// Log session start
	cl.logSessionStart(filePath)
	
	return cl, nil
}

// GetSessionID returns the session ID
func (cl *ConversationLogger) GetSessionID() string {
	return cl.sessionID
}

// ParseLevel parses a string level to Level enum
func ParseLevel(levelStr string) Level {
	switch levelStr {
	case "DEBUG":
		return DEBUG
	case "INFO":
		return INFO
	case "WARN":
		return WARN
	case "ERROR":
		return ERROR
	default:
		return INFO
	}
}

// InitConversationLogger initializes the global conversation logger
func InitConversationLogger(config ConversationConfig) error {
	var err error
	conversationLoggerOnce.Do(func() {
		if !config.Enabled {
			conversationLoggerInstance = &ConversationLogger{enabled: false}
			return
		}

		sessionID := generateSessionID()
		timestamp := time.Now().Format("20060102-150405")
		filename := fmt.Sprintf("conversation-%s-%s.log", sessionID, timestamp)
		
		// Ensure log directory exists
		if err = os.MkdirAll(config.LogDir, 0755); err != nil {
			log.Printf("❌ Failed to create log directory %s: %v", config.LogDir, err)
			return
		}
		
		logPath := filepath.Join(config.LogDir, filename)
		file, fileErr := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if fileErr != nil {
			err = fmt.Errorf("failed to create conversation log file %s: %v", logPath, fileErr)
			return
		}

		conversationLoggerInstance = &ConversationLogger{
			sessionID:     sessionID,
			logFile:       file,
			enabled:       true,
			logLevel:      config.LogLevel,
			maskSensitive: config.MaskSensitive,
			logFullTools:  config.LogFullTools,
			truncation:    config.Truncation,
		}

		// Log session start
		conversationLoggerInstance.logSessionStart(logPath)
		log.Printf("📋 Conversation logging enabled: %s", logPath)
	})
	return err
}

// GetConversationLogger returns the global conversation logger instance
func GetConversationLogger() *ConversationLogger {
	if conversationLoggerInstance == nil {
		return &ConversationLogger{enabled: false}
	}
	return conversationLoggerInstance
}

// generateSessionID creates a unique session identifier
func generateSessionID() string {
	return fmt.Sprintf("session_%d", time.Now().UnixNano()%100000)
}

// logSessionStart logs the session initialization
func (cl *ConversationLogger) logSessionStart(logPath string) {
	if !cl.enabled {
		return
	}
	
	sessionInfo := map[string]interface{}{
		"event":      "session_start",
		"session_id": cl.sessionID,
		"timestamp":  time.Now().UTC().Format(time.RFC3339),
		"log_file":   logPath,
	}
	
	cl.writeLogEntry("SESSION", sessionInfo)
}

// LogConversationStart logs the beginning of a new conversation
func (cl *ConversationLogger) LogConversationStart(ctx context.Context, requestID string) {
	if !cl.enabled {
		return
	}
	
	conversationInfo := map[string]interface{}{
		"event":        "conversation_start",
		"session_id":   cl.sessionID,
		"request_id":   requestID,
		"timestamp":    time.Now().UTC().Format(time.RFC3339),
	}
	
	cl.writeLogEntry("CONVERSATION", conversationInfo)
}

// LogRequest logs a complete incoming request
func (cl *ConversationLogger) LogRequest(ctx context.Context, requestID string, request interface{}) {
	if !cl.enabled || cl.logLevel > INFO {
		return
	}
	
	// Mask sensitive data if configured
	requestData := request
	if cl.maskSensitive {
		requestData = cl.maskSensitiveData(request)
	}
	
	// Process tool data based on LOG_FULL_TOOLS setting
	requestData = cl.processToolData(requestData)
	
	// Apply truncation if enabled
	if cl.truncation > 0 {
		requestData = cl.truncateMessages(requestData)
	}
	
	logEntry := map[string]interface{}{
		"event":      "request",
		"session_id": cl.sessionID,
		"request_id": requestID,
		"timestamp":  time.Now().UTC().Format(time.RFC3339),
		"data":       requestData,
	}
	
	cl.writeLogEntry("REQUEST", logEntry)
}

// LogResponse logs a complete outgoing response
func (cl *ConversationLogger) LogResponse(ctx context.Context, requestID string, response interface{}) {
	if !cl.enabled || cl.logLevel > INFO {
		return
	}
	
	// Mask sensitive data if configured
	responseData := response
	if cl.maskSensitive {
		responseData = cl.maskSensitiveData(response)
	}
	
	logEntry := map[string]interface{}{
		"event":      "response",
		"session_id": cl.sessionID,
		"request_id": requestID,
		"timestamp":  time.Now().UTC().Format(time.RFC3339),
		"data":       responseData,
	}
	
	cl.writeLogEntry("RESPONSE", logEntry)
}

// LogToolCall logs a tool call and its result
func (cl *ConversationLogger) LogToolCall(ctx context.Context, requestID string, toolName string, params interface{}, result interface{}) {
	if !cl.enabled || cl.logLevel > INFO {
		return
	}
	
	logEntry := map[string]interface{}{
		"event":      "tool_call",
		"session_id": cl.sessionID,
		"request_id": requestID,
		"timestamp":  time.Now().UTC().Format(time.RFC3339),
		"tool_name":  toolName,
		"params":     params,
		"result":     result,
	}
	
	cl.writeLogEntry("TOOL", logEntry)
}

// LogCorrection logs a tool correction attempt
func (cl *ConversationLogger) LogCorrection(ctx context.Context, requestID string, original interface{}, corrected interface{}, method string) {
	if !cl.enabled || cl.logLevel > INFO {
		return
	}
	
	logEntry := map[string]interface{}{
		"event":      "correction",
		"session_id": cl.sessionID,
		"request_id": requestID,
		"timestamp":  time.Now().UTC().Format(time.RFC3339),
		"method":     method,
		"original":   original,
		"corrected":  corrected,
	}
	
	cl.writeLogEntry("CORRECTION", logEntry)
}

// LogConversationEnd logs the end of a conversation
func (cl *ConversationLogger) LogConversationEnd(ctx context.Context, requestID string, stats map[string]interface{}) {
	if !cl.enabled {
		return
	}
	
	conversationInfo := map[string]interface{}{
		"event":      "conversation_end",
		"session_id": cl.sessionID,
		"request_id": requestID,
		"timestamp":  time.Now().UTC().Format(time.RFC3339),
		"stats":      stats,
	}
	
	cl.writeLogEntry("CONVERSATION", conversationInfo)
}

// writeLogEntry writes a structured log entry to the file
func (cl *ConversationLogger) writeLogEntry(category string, data map[string]interface{}) {
	if !cl.enabled {
		return
	}
	
	cl.mu.Lock()
	defer cl.mu.Unlock()
	
	if cl.logFile == nil {
		return
	}
	
	// Create structured log line
	logLine := map[string]interface{}{
		"category": category,
		"data":     data,
	}
	
	jsonData, err := json.MarshalIndent(logLine, "", "  ")
	if err != nil {
		log.Printf("❌ Failed to marshal conversation log entry: %v", err)
		return
	}
	
	// Write to file with newline
	if _, err := cl.logFile.Write(append(jsonData, '\n')); err != nil {
		log.Printf("❌ Failed to write conversation log entry: %v", err)
		return
	}
	
	// Ensure immediate write
	cl.logFile.Sync()
}

// maskSensitiveData removes or masks sensitive information
func (cl *ConversationLogger) maskSensitiveData(data interface{}) interface{} {
	// Convert to JSON and back to create a deep copy
	jsonData, err := json.Marshal(data)
	if err != nil {
		return data
	}
	
	var result interface{}
	if err := json.Unmarshal(jsonData, &result); err != nil {
		return data
	}
	
	// Recursively mask sensitive fields
	cl.maskSensitiveFields(result)
	return result
}

// maskSensitiveFields recursively masks sensitive fields in data structures
func (cl *ConversationLogger) maskSensitiveFields(data interface{}) {
	switch v := data.(type) {
	case map[string]interface{}:
		for key, value := range v {
			// Mask known sensitive fields
			if cl.isSensitiveField(key) {
				v[key] = "***"
			} else {
				cl.maskSensitiveFields(value)
			}
		}
	case []interface{}:
		for _, item := range v {
			cl.maskSensitiveFields(item)
		}
	}
}

// isSensitiveField checks if a field name contains sensitive information
func (cl *ConversationLogger) isSensitiveField(fieldName string) bool {
	sensitiveFields := []string{
		"api_key", "apikey", "key", "token", "secret", "password", "auth",
		"authorization", "bearer", "x-api-key",
	}
	
	fieldLower := fmt.Sprintf("%s", fieldName)
	for _, sensitive := range sensitiveFields {
		if fieldLower == sensitive {
			return true
		}
	}
	return false
}

// processToolData processes tool information based on LOG_FULL_TOOLS setting
func (cl *ConversationLogger) processToolData(data interface{}) interface{} {
	// If LOG_FULL_TOOLS is true, return data as-is
	if cl.logFullTools {
		return data
	}
	
	// Convert to JSON and back to create a deep copy
	jsonData, err := json.Marshal(data)
	if err != nil {
		return data
	}
	
	var result interface{}
	if err := json.Unmarshal(jsonData, &result); err != nil {
		return data
	}
	
	// Recursively process tools
	cl.processToolsInData(result)
	return result
}

// truncateMessages recursively truncates message content based on CONVERSATION_TRUNCATION setting
func (cl *ConversationLogger) truncateMessages(data interface{}) interface{} {
	// Convert to JSON and back to create a deep copy
	jsonData, err := json.Marshal(data)
	if err != nil {
		return data
	}
	
	var result interface{}
	if err := json.Unmarshal(jsonData, &result); err != nil {
		return data
	}
	
	// Recursively process messages
	cl.truncateMessagesInData(result)
	return result
}

// truncateMessagesInData recursively finds and truncates message content
func (cl *ConversationLogger) truncateMessagesInData(data interface{}) {
	switch v := data.(type) {
	case map[string]interface{}:
		for key, value := range v {
			if key == "content" {
				// Handle content as string
				if contentStr, ok := value.(string); ok {
					v[key] = cl.truncateString(contentStr)
				} else {
					// Always recurse into content even if it's not a string (could be array)
					cl.truncateMessagesInData(value)
				}
			} else if key == "text" {
				// Handle text fields anywhere in the structure
				if textStr, ok := value.(string); ok {
					v[key] = cl.truncateString(textStr)
				}
			} else {
				// Recurse into all other fields
				cl.truncateMessagesInData(value)
			}
		}
	case []interface{}:
		for _, item := range v {
			cl.truncateMessagesInData(item)
		}
	}
}

// truncateString truncates a string to the specified length, keeping beginning and end
func (cl *ConversationLogger) truncateString(s string) string {
	if cl.truncation <= 0 || len(s) <= cl.truncation {
		return s
	}
	
	// Handle very small truncation cases
	if cl.truncation < 5 {
		// Just return first few characters if we can't fit ellipsis
		if cl.truncation <= len(s) {
			return s[:cl.truncation]
		}
		return s
	}
	
	// Calculate how much to keep from each end
	halfLength := (cl.truncation - 5) / 2 // Reserve 5 chars for " ... "
	if halfLength < 1 {
		halfLength = 1
	}
	
	beginning := s[:halfLength]
	end := s[len(s)-halfLength:]
	
	return beginning + " ... " + end
}

// processToolsInData recursively processes tools array to show only tool names
func (cl *ConversationLogger) processToolsInData(data interface{}) {
	switch v := data.(type) {
	case map[string]interface{}:
		for key, value := range v {
			if key == "tools" {
				// Replace tools array with tool names only
				if toolsArray, ok := value.([]interface{}); ok {
					toolNames := make([]string, 0, len(toolsArray))
					for _, tool := range toolsArray {
						if toolMap, ok := tool.(map[string]interface{}); ok {
							if name, ok := toolMap["name"].(string); ok {
								toolNames = append(toolNames, name)
							}
						}
					}
					v[key] = toolNames
				}
			} else {
				cl.processToolsInData(value)
			}
		}
	case []interface{}:
		for _, item := range v {
			cl.processToolsInData(item)
		}
	}
}

// Close safely closes the conversation logger
func (cl *ConversationLogger) Close() error {
	if !cl.enabled || cl.logFile == nil {
		return nil
	}
	
	cl.mu.Lock()
	defer cl.mu.Unlock()
	
	// Log session end
	sessionInfo := map[string]interface{}{
		"event":     "session_end",
		"session_id": cl.sessionID,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}
	
	logLine := map[string]interface{}{
		"category": "SESSION",
		"data":     sessionInfo,
	}
	
	if jsonData, err := json.MarshalIndent(logLine, "", "  "); err == nil {
		cl.logFile.Write(append(jsonData, '\n'))
		cl.logFile.Sync()
	}
	
	return cl.logFile.Close()
}