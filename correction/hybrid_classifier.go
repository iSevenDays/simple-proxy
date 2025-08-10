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

	// Analyze recent messages (focus on last few turns)
	startIdx := 0
	if len(messages) > 6 { // Look at last 6 messages for context
		startIdx = len(messages) - 6
	}

	for _, msg := range messages[startIdx:] {
		if msg.Role == "user" || msg.Role == "assistant" {
			content := strings.ToLower(msg.Content)
			words := strings.Fields(content)

			// Look for verb-artifact patterns
			for i, word := range words {
				cleanWord := strings.Trim(word, ".,!?:;\"'()[]")

				// Check if this is an implementation verb
				if h.implVerbs[cleanWord] {
					// Look for artifacts in nearby words
					artifact := h.findNearbyArtifact(words, i, msg.Content)
					confident := artifact != "" || h.strongVerbs[cleanWord]

					pairs = append(pairs, ActionPair{
						Verb:      cleanWord,
						Artifact:  artifact,
						Confident: confident,
					})
				}

				// Check if this is a research verb (for contrast)
				if h.researchVerbs[cleanWord] && !h.implVerbs[cleanWord] {
					pairs = append(pairs, ActionPair{
						Verb:      cleanWord,
						Artifact:  "",
						Confident: true,
					})
				}
			}
		}

		// Also check if assistant used research tools (indicates research phase done)
		if msg.Role == "assistant" && len(msg.ToolCalls) > 0 {
			for _, toolCall := range msg.ToolCalls {
				toolName := strings.ToLower(toolCall.Function.Name)
				if toolName == "task" || toolName == "read" || toolName == "grep" || toolName == "glob" {
					// Previous research tools used - context indicator
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

// looksLikeFile checks if an artifact appears to be a file (uses the helper function from rules.go)
func (h *HybridClassifier) looksLikeFile(artifact string) bool {
	return looksLikeFile(artifact)
}