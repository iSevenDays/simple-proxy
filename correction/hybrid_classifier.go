package correction

import (
	"claude-proxy/logger"
	"claude-proxy/types"
	"regexp"
	"strings"
)

// ActionPair represents an extracted verb-artifact pair for tool necessity analysis
type ActionPair struct {
	Verb      string // The action verb (create, update, edit, etc.)
	Artifact  string // The target artifact (filename, command, etc.)
	Confident bool   // Whether the extraction is confident about this pair
}

// RuleDecision represents the result of deterministic rule evaluation
type RuleDecision struct {
	RequireTools bool   // Whether tools should be required
	Confident    bool   // Whether the rule engine is confident about this decision
	Reason       string // Human-readable reason for the decision
}

// HybridClassifier implements the two-stage approach for detecting tool necessity
type HybridClassifier struct {
	implVerbs     map[string]bool
	researchVerbs map[string]bool
	strongVerbs   map[string]bool
	filePattern   *regexp.Regexp
	ruleEngine    *RuleEngine // New rule-based evaluation system
}

// NewHybridClassifier creates a new two-stage hybrid classifier
func NewHybridClassifier() *HybridClassifier {
	// Implementation verbs that indicate tool usage
	implVerbs := map[string]bool{
		"create": true, "make": true, "build": true, "write": true, "add": true,
		"implement": true, "install": true, "setup": true, "configure": true,
		"edit": true, "modify": true, "update": true, "change": true,
		"fix": true, "correct": true, "repair": true, "patch": true, "debug": true,
		"run": true, "execute": true, "launch": true, "start": true,
		"delete": true, "remove": true, "clean": true, "clear": true,
		"document": true, "doc": true, "readme": true, // Documentation verbs
		// Include -ing forms
		"creating": true, "making": true, "building": true, "writing": true, "adding": true,
		"implementing": true, "installing": true, "setting": true, "configuring": true,
		"editing": true, "modifying": true, "updating": true, "changing": true,
		"fixing": true, "correcting": true, "repairing": true, "patching": true, "debugging": true,
		"running": true, "executing": true, "launching": true, "starting": true,
		"deleting": true, "removing": true, "cleaning": true, "clearing": true,
		"documenting": true, "docs": true, // Documentation -ing forms
	}

	// Research verbs that indicate investigation/analysis
	researchVerbs := map[string]bool{
		"read": true, "analyze": true, "examine": true, "check": true, "review": true,
		"explain": true, "describe": true, "tell": true, "show": true, "list": true,
		"find": true, "search": true, "look": true, "investigate": true, "explore": true,
		"understand": true, "learn": true, "study": true, "research": true,
	}

	// Strong implementation verbs that almost always require tools
	strongVerbs := map[string]bool{
		"create": true, "write": true, "edit": true, "update": true,
		"fix": true, "implement": true, "build": true, "run": true, "debug": true,
		"creating": true, "writing": true, "editing": true, "updating": true,
		"fixing": true, "implementing": true, "building": true, "running": true, "debugging": true,
	}

	// File pattern regex to detect file references
	filePattern := regexp.MustCompile(`\b[\w\-/\.]+\.(?:md|go|py|js|ts|json|yaml|yml|txt|cfg|conf|ini|toml|xml|html|css|sql|sh|bat|dockerfile|makefile|readme)\b`)

	return &HybridClassifier{
		implVerbs:     implVerbs,
		researchVerbs: researchVerbs,
		strongVerbs:   strongVerbs,
		filePattern:   filePattern,
		ruleEngine:    NewRuleEngine(), // Initialize the rule-based system
	}
}

// DetectToolNecessity implements the core two-stage hybrid logic with observability
// NOTE: This is a component-level method. For production use, call Service.DetectToolNecessity
// which provides the complete API including context handling and LLM fallback
func (h *HybridClassifier) DetectToolNecessity(messages []types.OpenAIMessage, logFunc func(component, category, requestID, message string, fields map[string]interface{}), requestID string) RuleDecision {
	// Handle nil logger gracefully
	if logFunc == nil {
		logFunc = func(component, category, requestID, message string, fields map[string]interface{}) {
			// No-op when logger is nil
		}
	}
	
	// Stage A: Extract verb-artifact pairs
	pairs := h.extractActionPairs(messages, logFunc, requestID)

	// Stage B: Apply rule-based evaluation using the rule engine
	decision := h.ruleEngine.Evaluate(pairs, messages, logFunc, requestID)

	return decision
}

// extractActionPairs analyzes conversation messages to extract verb-artifact pairs with observability by default
// Stage A of the two-stage hybrid classifier
func (h *HybridClassifier) extractActionPairs(messages []types.OpenAIMessage, logFunc func(component, category, requestID, message string, fields map[string]interface{}), requestID string) []ActionPair {
	// Handle nil logger for backward compatibility
	if logFunc == nil {
		logFunc = func(component, category, requestID, message string, fields map[string]interface{}) {
			// No-op when logger is nil
		}
	}

	var pairs []ActionPair

	logFunc(logger.ComponentHybridClassifier, logger.CategoryClassification, requestID, "Stage A: Starting action pair extraction", map[string]interface{}{
		"stage":           "A_extract_pairs",
		"messages_count":  len(messages),
	})

	// Find the most recent user message - this is the primary intent
	var mostRecentUserMsg *types.OpenAIMessage
	var mostRecentUserIdx int
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" {
			mostRecentUserMsg = &messages[i]
			mostRecentUserIdx = i
			break
		}
	}

	if mostRecentUserMsg == nil {
		logFunc(logger.ComponentHybridClassifier, logger.CategoryClassification, requestID, "Stage A: No user messages found", map[string]interface{}{
			"stage": "A_extract_pairs",
			"pairs_extracted": 0,
		})
		return pairs // No user messages found
	}

	logFunc(logger.ComponentHybridClassifier, logger.CategoryClassification, requestID, "Stage A: Found most recent user message", map[string]interface{}{
		"stage":             "A_extract_pairs",
		"user_message_idx":  mostRecentUserIdx,
		"user_prompt":       mostRecentUserMsg.Content,
	})

	// PHASE 1: Analyze the most recent user message (primary intent)
	content := strings.ToLower(mostRecentUserMsg.Content)
	words := strings.Fields(content)
	
	// CONTEXTUAL NEGATION DETECTION - Check for patterns that negate implementation intent
	if h.detectContextualNegation(content) {
		logFunc(logger.ComponentHybridClassifier, logger.CategoryClassification, requestID, "Stage A: Contextual negation detected", map[string]interface{}{
			"stage":                "A_extract_pairs",
			"negation_detected":    true,
			"content_snippet":      content[:min(100, len(content))],
		})
		
		// Add a special marker to indicate this is explanation/hypothetical only
		pairs = append(pairs, ActionPair{
			Verb:      "explanation_only",
			Artifact:  "contextual_negation",
			Confident: true,
		})
		
		logFunc(logger.ComponentHybridClassifier, logger.CategoryClassification, requestID, "Stage A: Action pairs extracted", map[string]interface{}{
			"stage":           "A_extract_pairs",
			"pairs_count":     len(pairs),
			"pairs":           pairs,
			"negation_result": true,
		})
		
		return pairs
	} else {
		logFunc(logger.ComponentHybridClassifier, logger.CategoryClassification, requestID, "Stage A: No contextual negation detected", map[string]interface{}{
			"stage":             "A_extract_pairs",
			"negation_detected": false,
		})
	}

	// Extract verb-artifact pairs from current message
	currentPairs := []ActionPair{}
	for i, word := range words {
		cleanWord := strings.Trim(word, ".,!?:;\"'()[]")

		// Check if this is an implementation verb in the current request
		if h.implVerbs[cleanWord] {
			artifact := h.findNearbyArtifact(words, i, mostRecentUserMsg.Content)
			confident := artifact != "" || h.strongVerbs[cleanWord]

			pair := ActionPair{
				Verb:      cleanWord,
				Artifact:  artifact,
				Confident: confident,
			}
			pairs = append(pairs, pair)
			currentPairs = append(currentPairs, pair)
			
			logFunc(logger.ComponentHybridClassifier, logger.CategoryClassification, requestID, "Stage A: Implementation verb detected", map[string]interface{}{
				"stage":      "A_extract_pairs",
				"verb":       cleanWord,
				"artifact":   artifact,
				"confident":  confident,
				"is_strong":  h.strongVerbs[cleanWord],
			})
		}

		// Check if this is a research verb
		if h.researchVerbs[cleanWord] && !h.implVerbs[cleanWord] {
			pair := ActionPair{
				Verb:      cleanWord,
				Artifact:  "",
				Confident: true,
			}
			pairs = append(pairs, pair)
			currentPairs = append(currentPairs, pair)
			
			logFunc(logger.ComponentHybridClassifier, logger.CategoryClassification, requestID, "Stage A: Research verb detected", map[string]interface{}{
				"stage":     "A_extract_pairs",
				"verb":      cleanWord,
				"artifact":  "",
				"confident": true,
			})
		}
	}

	logFunc(logger.ComponentHybridClassifier, logger.CategoryClassification, requestID, "Stage A: Current message analysis complete", map[string]interface{}{
		"stage":                   "A_extract_pairs",
		"current_pairs_count":     len(currentPairs),
		"current_pairs":           currentPairs,
	})

	// PHASE 2: Check previous user messages for compound request context
	// Only if we don't have strong implementation verbs in the current message
	hasStrongCurrentImplementation := false
	for _, pair := range pairs {
		if h.strongVerbs[pair.Verb] && pair.Artifact != "" {
			hasStrongCurrentImplementation = true
			break
		}
	}

	logFunc(logger.ComponentHybridClassifier, logger.CategoryClassification, requestID, "Stage A: Strong implementation check", map[string]interface{}{
		"stage":                          "A_extract_pairs",
		"has_strong_current_impl":        hasStrongCurrentImplementation,
	})

	// If current message doesn't have strong implementation signals, check if this might be 
	// a compound request continuation that needs historical context
	if !hasStrongCurrentImplementation {
		// First, check if implementation has been completed recently
		implementationCompleted := h.detectRecentImplementationCompletion(messages, mostRecentUserIdx, logFunc, requestID)
		
		// Only check historical context if implementation hasn't been completed
		if !implementationCompleted {
			logFunc(logger.ComponentHybridClassifier, logger.CategoryClassification, requestID, "Stage A: Checking historical context", map[string]interface{}{
				"stage":                      "A_extract_pairs",
				"implementation_completed":   false,
			})
			
			// Look back through previous user messages for compound request context
			historicalPairs := []ActionPair{}
			for i := mostRecentUserIdx - 1; i >= 0; i-- {
				if messages[i].Role == "user" {
					historicalContent := strings.ToLower(messages[i].Content)
					historicalWords := strings.Fields(historicalContent)

					for j, word := range historicalWords {
						cleanWord := strings.Trim(word, ".,!?:;\"'()[]")

						// Only add historical implementation verbs with lower priority
						if h.implVerbs[cleanWord] {
							artifact := h.findNearbyArtifact(historicalWords, j, messages[i].Content)
							confident := artifact != "" || h.strongVerbs[cleanWord]

							// Mark as historical context (less confident)
							pair := ActionPair{
								Verb:      cleanWord,
								Artifact:  artifact,
								Confident: confident && len(pairs) == 0, // Only confident if no current pairs
							}
							pairs = append(pairs, pair)
							historicalPairs = append(historicalPairs, pair)
							
							logFunc(logger.ComponentHybridClassifier, logger.CategoryClassification, requestID, "Stage A: Historical verb detected", map[string]interface{}{
								"stage":              "A_extract_pairs",
								"verb":               cleanWord,
								"artifact":           artifact,
								"confident":          pair.Confident,
								"historical_msg_idx": i,
							})
						}
					}
					
					// Only look at the previous user message for compound context
					break
				}
			}
			
			logFunc(logger.ComponentHybridClassifier, logger.CategoryClassification, requestID, "Stage A: Historical context analysis complete", map[string]interface{}{
				"stage":                  "A_extract_pairs",
				"historical_pairs_count": len(historicalPairs),
				"historical_pairs":       historicalPairs,
			})
		} else {
			logFunc(logger.ComponentHybridClassifier, logger.CategoryClassification, requestID, "Stage A: Implementation recently completed, skipping historical context", map[string]interface{}{
				"stage":                    "A_extract_pairs",
				"implementation_completed": true,
			})
		}
	}

	// PHASE 3: Analyze recent assistant messages for research context
	startIdx := 0
	if len(messages) > 6 {
		startIdx = len(messages) - 6
	}

	researchPairs := []ActionPair{}
	for msgIdx, msg := range messages[startIdx:] {
		if msg.Role == "assistant" && len(msg.ToolCalls) > 0 {
			for _, toolCall := range msg.ToolCalls {
				toolName := strings.ToLower(toolCall.Function.Name)
				if toolName == "task" || toolName == "read" || toolName == "grep" || toolName == "glob" {
					pair := ActionPair{
						Verb:      "research_done",
						Artifact:  toolName,
						Confident: true,
					}
					pairs = append(pairs, pair)
					researchPairs = append(researchPairs, pair)
					
					logFunc(logger.ComponentHybridClassifier, logger.CategoryClassification, requestID, "Stage A: Research tool detected", map[string]interface{}{
						"stage":              "A_extract_pairs",
						"tool_name":          toolName,
						"assistant_msg_idx":  startIdx + msgIdx,
					})
				}
			}
		}
	}

	logFunc(logger.ComponentHybridClassifier, logger.CategoryClassification, requestID, "Stage A: Research context analysis complete", map[string]interface{}{
		"stage":                "A_extract_pairs",
		"research_pairs_count": len(researchPairs),
		"research_pairs":       researchPairs,
	})

	logFunc(logger.ComponentHybridClassifier, logger.CategoryClassification, requestID, "Stage A: Action pair extraction complete", map[string]interface{}{
		"stage":         "A_extract_pairs",
		"total_pairs":   len(pairs),
		"pairs":         pairs,
	})

	return pairs
}


// detectRecentImplementationCompletion checks if implementation work was recently completed with observability by default
func (h *HybridClassifier) detectRecentImplementationCompletion(messages []types.OpenAIMessage, mostRecentUserIdx int, logFunc func(component, category, requestID, message string, fields map[string]interface{}), requestID string) bool {
	logFunc(logger.ComponentHybridClassifier, logger.CategoryClassification, requestID, "Stage A: Checking recent implementation completion", map[string]interface{}{
		"stage":               "A_extract_pairs",
		"most_recent_user_idx": mostRecentUserIdx,
	})
	
	// Look at messages between the most recent user message and the previous user message
	if mostRecentUserIdx < 1 {
		logFunc(logger.ComponentHybridClassifier, logger.CategoryClassification, requestID, "Stage A: No previous context for completion check", map[string]interface{}{
			"stage": "A_extract_pairs",
		})
		return false // No previous context
	}
	
	// Find the previous user message
	prevUserIdx := -1
	for i := mostRecentUserIdx - 1; i >= 0; i-- {
		if messages[i].Role == "user" {
			prevUserIdx = i
			break
		}
	}
	
	if prevUserIdx == -1 {
		logFunc(logger.ComponentHybridClassifier, logger.CategoryClassification, requestID, "Stage A: No previous user message found", map[string]interface{}{
			"stage": "A_extract_pairs",
		})
		return false // No previous user message
	}
	
	logFunc(logger.ComponentHybridClassifier, logger.CategoryClassification, requestID, "Stage A: Analyzing completion indicators", map[string]interface{}{
		"stage":         "A_extract_pairs",
		"prev_user_idx": prevUserIdx,
		"check_range":   []int{prevUserIdx + 1, mostRecentUserIdx},
	})
	
	// Check assistant messages between previous user message and current user message
	// for completion indicators
	for i := prevUserIdx + 1; i < mostRecentUserIdx; i++ {
		if messages[i].Role == "assistant" {
			content := strings.ToLower(messages[i].Content)
			
			// Look for completion indicators
			completionPhrases := []string{
				"updated", "completed", "finished", "successfully", 
				"created", "implemented", "generated", "added",
				"comprehensive", "analysis", "based on",
				"the file now contains", "detailed information",
			}
			
			completionCount := 0
			foundPhrases := []string{}
			for _, phrase := range completionPhrases {
				if strings.Contains(content, phrase) {
					completionCount++
					foundPhrases = append(foundPhrases, phrase)
				}
			}
			
			logFunc(logger.ComponentHybridClassifier, logger.CategoryClassification, requestID, "Stage A: Completion phrases analysis", map[string]interface{}{
				"stage":             "A_extract_pairs",
				"assistant_msg_idx": i,
				"completion_count":  completionCount,
				"found_phrases":     foundPhrases,
			})
			
			// If we see multiple completion indicators in a single message,
			// and there were tool calls in this conversation, it's likely completion
			if completionCount >= 3 {
				// Also check that there were actual tool calls in this sequence
				for j := prevUserIdx + 1; j < mostRecentUserIdx; j++ {
					if messages[j].Role == "assistant" && len(messages[j].ToolCalls) > 0 {
						logFunc(logger.ComponentHybridClassifier, logger.CategoryClassification, requestID, "Stage A: Implementation completion detected", map[string]interface{}{
							"stage":            "A_extract_pairs",
							"completion_count": completionCount,
							"tool_calls_found": true,
						})
						return true // Implementation was completed
					}
				}
			}
		}
	}
	
	logFunc(logger.ComponentHybridClassifier, logger.CategoryClassification, requestID, "Stage A: No recent implementation completion found", map[string]interface{}{
		"stage": "A_extract_pairs",
	})
	
	return false
}

// min helper function
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}


// AddCustomRule allows adding custom rules to the classifier
func (h *HybridClassifier) AddCustomRule(rule Rule) {
	h.ruleEngine.AddRule(rule)
}

// findNearbyArtifact looks for file patterns or artifacts near a verb
func (h *HybridClassifier) findNearbyArtifact(words []string, verbIndex int, originalContent string) string {
	// Check words around the verb (within 3 positions)
	start := verbIndex - 3
	if start < 0 {
		start = 0
	}
	end := verbIndex + 4
	if end > len(words) {
		end = len(words)
	}

	// Look for file patterns first
	for i := start; i < end; i++ {
		if matches := h.filePattern.FindStringSubmatch(words[i]); matches != nil {
			return matches[0]
		}
	}

	// Check the original content for file patterns
	if matches := h.filePattern.FindStringSubmatch(originalContent); matches != nil {
		return matches[0]
	}

	// Look for common non-file artifacts
	artifactPatterns := []string{
		"file", "config", "settings", "test", "tests", "command", "script",
		"function", "method", "class", "module", "package", "dependency",
	}

	for i := start; i < end; i++ {
		word := strings.ToLower(strings.Trim(words[i], ".,!?:;\"'()[]"))
		for _, pattern := range artifactPatterns {
			if word == pattern || strings.Contains(word, pattern) {
				return word
			}
		}
	}

	return ""
}

// detectContextualNegation identifies patterns that negate implementation intent
// Returns true if the request is for explanation/hypothetical scenarios rather than implementation
func (h *HybridClassifier) detectContextualNegation(content string) bool {
	// Patterns that indicate explanation/teaching rather than implementation
	teachingPatterns := []string{
		"show me how to",
		"explain how to",
		"tell me how to",
		"demonstrate how to",
		"walk me through",
		"guide me through",
		"how do i",
		"how should i",
		"how can i",
		"what's the best way to",
		"what is the best way to",
	}
	
	// Patterns that indicate hypothetical scenarios
	hypotheticalPatterns := []string{
		"what would happen if",
		"what if i",
		"what if we",
		"suppose i",
		"suppose we",
		"imagine if",
		"if i were to",
		"if we were to",
		"hypothetically",
		"theoretically",
	}
	
	// Patterns that indicate analysis without action
	analysisPatterns := []string{
		"without fixing",
		"without implementing",
		"without creating",
		"without updating",
		"without changing",
		"without modifying",
		"analyze what",
		"explain what",
		"describe what",
		"tell me what",
		"show me what happens",
		"show me what would",
		"show me what this",
		"show me what the",
		"without actually",
		"but don't",
		"don't actually",
		"just analyze",
		"only analyze",
		"just explain",
		"only explain",
	}
	
	// Patterns that indicate meta-conversations about tools (not using tools)
	metaToolPatterns := []string{
		"how does the",
		"how do you use",
		"what does the",
		"explain the",
		"describe the",
		"tell me about the",
		"what parameters",
		"what options",
		"how to use",
		"tool work",
		"tool do",
		"command work",
		"command do",
		"function work",
		"function do",
	}
	
	// Check for teaching patterns
	for _, pattern := range teachingPatterns {
		if strings.Contains(content, pattern) {
			return true
		}
	}
	
	// Check for hypothetical patterns
	for _, pattern := range hypotheticalPatterns {
		if strings.Contains(content, pattern) {
			return true
		}
	}
	
	// Check for analysis-only patterns
	for _, pattern := range analysisPatterns {
		if strings.Contains(content, pattern) {
			return true
		}
	}
	
	// Check for meta-tool conversation patterns
	for _, pattern := range metaToolPatterns {
		if strings.Contains(content, pattern) {
			return true
		}
	}
	
	return false
}

// looksLikeFile checks if an artifact appears to be a file (uses the helper function from rules.go)
func (h *HybridClassifier) looksLikeFile(artifact string) bool {
	return looksLikeFile(artifact)
}