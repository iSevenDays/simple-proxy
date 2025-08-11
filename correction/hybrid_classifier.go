package correction

import (
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
		"fix": true, "correct": true, "repair": true, "patch": true,
		"run": true, "execute": true, "launch": true, "start": true,
		"delete": true, "remove": true, "clean": true, "clear": true,
		"document": true, "doc": true, "readme": true, // Documentation verbs
		// Include -ing forms
		"creating": true, "making": true, "building": true, "writing": true, "adding": true,
		"implementing": true, "installing": true, "setting": true, "configuring": true,
		"editing": true, "modifying": true, "updating": true, "changing": true,
		"fixing": true, "correcting": true, "repairing": true, "patching": true,
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
		"fix": true, "implement": true, "build": true, "run": true,
		"creating": true, "writing": true, "editing": true, "updating": true,
		"fixing": true, "implementing": true, "building": true, "running": true,
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

// DetectToolNecessity implements the two-stage hybrid approach
func (h *HybridClassifier) DetectToolNecessity(messages []types.OpenAIMessage) RuleDecision {
	// Stage A: Extract verb-artifact pairs
	pairs := h.extractActionPairs(messages)

	// Stage B: Apply rule-based evaluation using the rule engine
	decision := h.ruleEngine.Evaluate(pairs, messages)

	return decision
}

// extractActionPairs analyzes conversation messages to extract verb-artifact pairs
// Stage A of the two-stage hybrid classifier
func (h *HybridClassifier) extractActionPairs(messages []types.OpenAIMessage) []ActionPair {
	var pairs []ActionPair

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
		return pairs // No user messages found
	}

	// PHASE 1: Analyze the most recent user message (primary intent)
	content := strings.ToLower(mostRecentUserMsg.Content)
	words := strings.Fields(content)
	
	// CONTEXTUAL NEGATION DETECTION - Check for patterns that negate implementation intent
	if h.detectContextualNegation(content) {
		// Add a special marker to indicate this is explanation/hypothetical only
		pairs = append(pairs, ActionPair{
			Verb:      "explanation_only",
			Artifact:  "contextual_negation",
			Confident: true,
		})
		return pairs
	}

	for i, word := range words {
		cleanWord := strings.Trim(word, ".,!?:;\"'()[]")

		// Check if this is an implementation verb in the current request
		if h.implVerbs[cleanWord] {
			artifact := h.findNearbyArtifact(words, i, mostRecentUserMsg.Content)
			confident := artifact != "" || h.strongVerbs[cleanWord]

			pairs = append(pairs, ActionPair{
				Verb:      cleanWord,
				Artifact:  artifact,
				Confident: confident,
			})
		}

		// Check if this is a research verb
		if h.researchVerbs[cleanWord] && !h.implVerbs[cleanWord] {
			pairs = append(pairs, ActionPair{
				Verb:      cleanWord,
				Artifact:  "",
				Confident: true,
			})
		}
	}

	// PHASE 2: Check previous user messages for compound request context
	// Only if we don't have strong implementation verbs in the current message
	hasStrongCurrentImplementation := false
	for _, pair := range pairs {
		if h.strongVerbs[pair.Verb] && pair.Artifact != "" {
			hasStrongCurrentImplementation = true
			break
		}
	}

	// If current message doesn't have strong implementation signals, check if this might be 
	// a compound request continuation that needs historical context
	if !hasStrongCurrentImplementation {
		// First, check if implementation has been completed recently
		implementationCompleted := h.detectRecentImplementationCompletion(messages, mostRecentUserIdx)
		
		// Only check historical context if implementation hasn't been completed
		if !implementationCompleted {
			// Look back through previous user messages for compound request context
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
							pairs = append(pairs, ActionPair{
								Verb:      cleanWord,
								Artifact:  artifact,
								Confident: confident && len(pairs) == 0, // Only confident if no current pairs
							})
						}
					}
					
					// Only look at the previous user message for compound context
					break
				}
			}
		}
	}

	// PHASE 3: Analyze recent assistant messages for research context
	startIdx := 0
	if len(messages) > 6 {
		startIdx = len(messages) - 6
	}

	for _, msg := range messages[startIdx:] {
		if msg.Role == "assistant" && len(msg.ToolCalls) > 0 {
			for _, toolCall := range msg.ToolCalls {
				toolName := strings.ToLower(toolCall.Function.Name)
				if toolName == "task" || toolName == "read" || toolName == "grep" || toolName == "glob" {
					pairs = append(pairs, ActionPair{
						Verb:      "research_done",
						Artifact:  toolName,
						Confident: true,
					})
				}
			}
		}
	}

	return pairs
}

// detectRecentImplementationCompletion checks if implementation work was recently completed
// by looking for assistant messages indicating completion after tool usage
func (h *HybridClassifier) detectRecentImplementationCompletion(messages []types.OpenAIMessage, mostRecentUserIdx int) bool {
	// Look at messages between the most recent user message and the previous user message
	if mostRecentUserIdx < 1 {
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
		return false // No previous user message
	}
	
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
			for _, phrase := range completionPhrases {
				if strings.Contains(content, phrase) {
					completionCount++
				}
			}
			
			// If we see multiple completion indicators in a single message,
			// and there were tool calls in this conversation, it's likely completion
			if completionCount >= 3 {
				// Also check that there were actual tool calls in this sequence
				for j := prevUserIdx + 1; j < mostRecentUserIdx; j++ {
					if messages[j].Role == "assistant" && len(messages[j].ToolCalls) > 0 {
						return true // Implementation was completed
					}
				}
			}
		}
	}
	
	return false
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
		"show me what",
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