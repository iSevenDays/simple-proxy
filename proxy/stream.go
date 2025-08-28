package proxy

import (
	"bufio"
	"claude-proxy/types"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// ProcessStreamingResponse handles streaming OpenAI responses properly
// Reads all chunks until finish_reason != null (solving the core streaming issue)
func (h *Handler) ProcessStreamingResponse(ctx context.Context, resp *http.Response) (*types.OpenAIResponse, error) {
	requestID := GetRequestID(ctx)
	if h.obsLogger != nil {
		h.obsLogger.Info("proxy_core", "request", requestID, "Processing streaming response", map[string]interface{}{})
	}

	scanner := bufio.NewScanner(resp.Body)
	// Increase buffer size to handle large streaming chunks (tool calls, long content)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024) // 64KB initial, 1MB max
	var chunks []types.OpenAIStreamChunk
	var finalChunk *types.OpenAIStreamChunk

	for scanner.Scan() {
		line := scanner.Text()

		// Skip empty lines and non-data lines
		if line == "" || !strings.HasPrefix(line, "data: ") {
			continue
		}

		// Extract JSON from "data: " prefix
		jsonStr := strings.TrimPrefix(line, "data: ")

		// Skip [DONE] marker
		if jsonStr == "[DONE]" {
			break
		}

		// Parse chunk
		var chunk types.OpenAIStreamChunk
		if err := json.Unmarshal([]byte(jsonStr), &chunk); err != nil {
			if h.obsLogger != nil {
				h.obsLogger.Warn("proxy_core", "warning", requestID, "Failed to parse streaming chunk", map[string]interface{}{
					"error": err.Error(),
				})
			}
			continue
		}

		chunks = append(chunks, chunk)

		// Check if this is the final chunk with finish_reason
		if len(chunk.Choices) > 0 && chunk.Choices[0].FinishReason != nil {
			finalChunk = &chunk
			if h.obsLogger != nil {
				h.obsLogger.Info("proxy_core", "request", requestID, "Found final chunk with finish_reason", map[string]interface{}{
					"finish_reason": *chunk.Choices[0].FinishReason,
				})
			}
			break
		}
	}

	if err := scanner.Err(); err != nil {
		if h.obsLogger != nil {
			h.obsLogger.Error("proxy_core", "error", requestID, "Streaming error", map[string]interface{}{
				"error": err.Error(),
			})
		}
		return nil, fmt.Errorf("error reading stream: %v", err)
	}

	if h.obsLogger != nil {
		h.obsLogger.Info("proxy_core", "request", requestID, "Processed streaming chunks", map[string]interface{}{
			"chunk_count": len(chunks),
		})
	}

	// Reconstruct complete response from chunks
	return h.ReconstructResponseFromChunks(ctx, chunks, finalChunk)
}

// ReconstructResponseFromChunks builds a complete OpenAI response from streaming chunks
func (h *Handler) ReconstructResponseFromChunks(ctx context.Context, chunks []types.OpenAIStreamChunk, finalChunk *types.OpenAIStreamChunk) (*types.OpenAIResponse, error) {
	if len(chunks) == 0 {
		return nil, fmt.Errorf("no chunks received")
	}

	firstChunk := chunks[0]

	// Initialize response structure
	response := &types.OpenAIResponse{
		ID:      firstChunk.ID,
		Object:  "chat.completion",
		Created: firstChunk.Created,
		Model:   firstChunk.Model,
		Choices: []types.OpenAIChoice{},
		Usage: types.OpenAIUsage{
			PromptTokens:     0,
			CompletionTokens: 0,
			TotalTokens:      0,
		},
	}

	// Reconstruct message content and tool calls
	var contentParts []string
	var toolCalls []types.OpenAIToolCall

	for _, chunk := range chunks {
		if len(chunk.Choices) == 0 {
			continue
		}

		delta := chunk.Choices[0].Delta

		// Accumulate content
		if delta.Content != "" {
			contentParts = append(contentParts, delta.Content)
		}

		// Accumulate tool calls by index (streaming chunks can have partial data)
		for _, toolCall := range delta.ToolCalls {
			index := toolCall.Index

			// Ensure we have enough tool calls for this index
			for len(toolCalls) <= index {
				toolCalls = append(toolCalls, types.OpenAIToolCall{
					Type:     "function",
					Function: types.OpenAIToolCallFunction{},
				})
			}

			// Accumulate fields for this tool call index
			if toolCall.ID != "" {
				toolCalls[index].ID = toolCall.ID
			}
			if toolCall.Type != "" {
				toolCalls[index].Type = toolCall.Type
			}
			if toolCall.Function.Name != "" {
				toolCalls[index].Function.Name = toolCall.Function.Name
			}
			// Always accumulate arguments (can be spread across multiple chunks)
			toolCalls[index].Function.Arguments += toolCall.Function.Arguments
		}
	}

	// Build final message
	message := types.OpenAIMessage{
		Role:    "assistant",
		Content: strings.Join(contentParts, ""),
	}

	requestID := GetRequestID(ctx)
	if len(toolCalls) > 0 {
		message.ToolCalls = toolCalls
		if h.obsLogger != nil {
			h.obsLogger.Info("proxy_core", "transformation", requestID, "Reconstructed tool calls", map[string]interface{}{
				"tool_call_count": len(toolCalls),
			})
		}
	}

	// Set finish reason
	var finishReason *string
	if finalChunk != nil && len(finalChunk.Choices) > 0 {
		finishReason = finalChunk.Choices[0].FinishReason
	}

	// Add choice to response
	response.Choices = append(response.Choices, types.OpenAIChoice{
		Index:        0,
		Message:      message,
		FinishReason: finishReason,
	})

	finishReasonStr := "null"
	if finishReason != nil {
		finishReasonStr = *finishReason
	}
	// Use structured logging for response reconstruction summary
	if h.obsLogger != nil {
		h.obsLogger.Info("proxy_core", "success", requestID, "Reconstructed complete response", map[string]interface{}{
			"content_length": len(message.Content),
			"tool_calls":     len(toolCalls),
			"finish_reason":  finishReasonStr,
		})
	}

	return response, nil
}
