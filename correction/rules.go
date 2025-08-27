package correction

import (
	"claude-proxy/logger"
	"claude-proxy/types"
	"strings"
)

// Rule represents a single rule that can be evaluated for tool necessity
type Rule interface {
	IsSatisfiedBy(pairs []ActionPair, messages []types.OpenAIMessage) (bool, RuleDecision)
	Priority() int // Higher priority rules are evaluated first
	Name() string  // For debugging and logging
}

// RuleEngine evaluates rules in priority order and returns the first confident decision
type RuleEngine struct {
	rules []Rule
}

// NewRuleEngine creates a new rule engine with default rules
func NewRuleEngine() *RuleEngine {
	rules := []Rule{
		// Highest priority: Contextual negation (explanation/hypothetical requests)
		&ContextualNegationRule{},

		// High priority: Clear implementation patterns
		&StrongVerbWithFileRule{},
		&ImplementationVerbWithFileRule{},
		&ResearchCompletionRule{},

		// Medium priority: Less confident patterns
		&StrongVerbWithoutArtifactRule{},

		// Low priority: Exclusion patterns
		&PureResearchRule{},

		// Fallback: Ambiguous cases
		&AmbiguousRequestRule{},
	}

	// Sort rules by priority (higher priority first)
	for i := 0; i < len(rules)-1; i++ {
		for j := i + 1; j < len(rules); j++ {
			if rules[i].Priority() < rules[j].Priority() {
				rules[i], rules[j] = rules[j], rules[i]
			}
		}
	}

	return &RuleEngine{rules: rules}
}

// Evaluate runs rules in priority order with observability by default and returns the first confident decision
func (re *RuleEngine) Evaluate(pairs []ActionPair, messages []types.OpenAIMessage, logFunc func(component, category, requestID, message string, fields map[string]interface{}), requestID string) RuleDecision {
	// Handle nil logger gracefully
	if logFunc == nil {
		logFunc = func(component, category, requestID, message string, fields map[string]interface{}) {
			// No-op when logger is nil
		}
	}

	logFunc(logger.ComponentHybridClassifier, logger.CategoryClassification, requestID, "Stage B: Starting rule engine evaluation", map[string]interface{}{
		"stage":      "B_rule_engine",
		"rules_count": len(re.rules),
		"pairs_input": pairs,
	})

	for i, rule := range re.rules {
		logFunc(logger.ComponentHybridClassifier, logger.CategoryClassification, requestID, "Stage B: Evaluating rule", map[string]interface{}{
			"stage":        "B_rule_engine",
			"rule_name":    rule.Name(),
			"rule_priority": rule.Priority(),
			"rule_index":   i,
		})

		if satisfied, decision := rule.IsSatisfiedBy(pairs, messages); satisfied {
			logFunc(logger.ComponentHybridClassifier, logger.CategoryClassification, requestID, "Stage B: Rule matched", map[string]interface{}{
				"stage":         "B_rule_engine",
				"rule_name":     rule.Name(),
				"rule_priority": rule.Priority(),
				"matched":       true,
				"decision":      boolToDecisionString(decision.RequireTools),
				"confident":     decision.Confident,
				"reason":        decision.Reason,
			})
			
			logFunc(logger.ComponentHybridClassifier, logger.CategoryClassification, requestID, "Stage B: Rule engine evaluation complete", map[string]interface{}{
				"stage":           "B_rule_engine",
				"final_rule":      rule.Name(),
				"final_decision":  boolToDecisionString(decision.RequireTools),
				"final_confident": decision.Confident,
				"final_reason":    decision.Reason,
			})
			
			return decision
		} else {
			logFunc(logger.ComponentHybridClassifier, logger.CategoryClassification, requestID, "Stage B: Rule not matched", map[string]interface{}{
				"stage":        "B_rule_engine",
				"rule_name":    rule.Name(),
				"rule_priority": rule.Priority(),
				"matched":      false,
			})
		}
	}

	// Should never reach here due to AmbiguousRequestRule fallback
	fallbackDecision := RuleDecision{
		RequireTools: false,
		Confident:    false,
		Reason:       "No rules matched (unexpected)",
	}
	
	logFunc(logger.ComponentHybridClassifier, logger.CategoryClassification, requestID, "Stage B: Unexpected fallback reached", map[string]interface{}{
		"stage":    "B_rule_engine",
		"decision": boolToDecisionString(fallbackDecision.RequireTools),
		"confident": fallbackDecision.Confident,
		"reason":   fallbackDecision.Reason,
	})
	
	return fallbackDecision
}


// boolToDecisionString converts a boolean to "require_tools" or "no_tools" for rule logging
func boolToDecisionString(b bool) string {
	if b {
		return "require_tools"
	}
	return "no_tools"
}

// AddRule adds a custom rule to the engine
func (re *RuleEngine) AddRule(rule Rule) {
	re.rules = append(re.rules, rule)

	// Re-sort by priority
	for i := 0; i < len(re.rules)-1; i++ {
		for j := i + 1; j < len(re.rules); j++ {
			if re.rules[i].Priority() < re.rules[j].Priority() {
				re.rules[i], re.rules[j] = re.rules[j], re.rules[i]
			}
		}
	}
}

// Helper functions for rule implementations
func hasStrongVerb(pairs []ActionPair, strongVerbs map[string]bool) bool {
	for _, pair := range pairs {
		if strongVerbs[pair.Verb] {
			return true
		}
	}
	return false
}

func hasImplementationVerb(pairs []ActionPair, implVerbs map[string]bool) bool {
	for _, pair := range pairs {
		if implVerbs[pair.Verb] {
			return true
		}
	}
	return false
}

func hasResearchVerb(pairs []ActionPair, researchVerbs map[string]bool) bool {
	for _, pair := range pairs {
		if researchVerbs[pair.Verb] {
			return true
		}
	}
	return false
}

func hasFileArtifact(pairs []ActionPair) bool {
	for _, pair := range pairs {
		if pair.Artifact != "" && looksLikeFile(pair.Artifact) {
			return true
		}
	}
	return false
}

func hasResearchCompletion(pairs []ActionPair) bool {
	for _, pair := range pairs {
		if pair.Verb == "research_done" {
			return true
		}
	}
	return false
}

func looksLikeFile(artifact string) bool {
	if artifact == "" {
		return false
	}

	// Check for file extensions
	fileExtensions := []string{
		".md", ".go", ".py", ".js", ".ts", ".json", ".yaml", ".yml",
		".txt", ".cfg", ".conf", ".ini", ".toml", ".xml", ".html",
		".css", ".sql", ".sh", ".bat", ".dockerfile", ".makefile",
		".java", ".swift", ".cpp",
	}

	lower := strings.ToLower(artifact)
	for _, ext := range fileExtensions {
		if strings.HasSuffix(lower, ext) {
			return true
		}
	}

	// Check for common file-related words
	fileWords := []string{"file", "config", "script", "document", "readme"}
	for _, word := range fileWords {
		if strings.Contains(lower, word) {
			return true
		}
	}

	return false
}

// StrongVerbWithFileRule: Strong implementation verbs + file artifacts = YES (confident)
type StrongVerbWithFileRule struct{}

func (r *StrongVerbWithFileRule) Priority() int { return 100 }
func (r *StrongVerbWithFileRule) Name() string  { return "StrongVerbWithFile" }

func (r *StrongVerbWithFileRule) IsSatisfiedBy(pairs []ActionPair, messages []types.OpenAIMessage) (bool, RuleDecision) {
	strongVerbs := map[string]bool{
		"create": true, "write": true, "edit": true, "update": true,
		"fix": true, "implement": true, "build": true, "run": true, "debug": true,
		"creating": true, "writing": true, "editing": true, "updating": true,
		"fixing": true, "implementing": true, "building": true, "running": true, "debugging": true,
	}

	for _, pair := range pairs {
		if strongVerbs[pair.Verb] && pair.Artifact != "" {
			// General rule for strong verbs + files
			if looksLikeFile(pair.Artifact) {
				return true, RuleDecision{
					RequireTools: true,
					Confident:    true,
					Reason:       "Strong implementation verb '" + pair.Verb + "' with file '" + pair.Artifact + "'",
				}
			}
		}
	}

	return false, RuleDecision{}
}

// ImplementationVerbWithFileRule: Any implementation verb + clear file pattern = YES (confident)
type ImplementationVerbWithFileRule struct{}

func (r *ImplementationVerbWithFileRule) Priority() int { return 90 }
func (r *ImplementationVerbWithFileRule) Name() string  { return "ImplementationVerbWithFile" }

func (r *ImplementationVerbWithFileRule) IsSatisfiedBy(pairs []ActionPair, messages []types.OpenAIMessage) (bool, RuleDecision) {
	implVerbs := map[string]bool{
		"create": true, "make": true, "build": true, "write": true, "add": true,
		"implement": true, "install": true, "setup": true, "configure": true,
		"edit": true, "modify": true, "update": true, "change": true,
		"fix": true, "correct": true, "repair": true, "patch": true,
		"run": true, "execute": true, "launch": true, "start": true,
		"delete": true, "remove": true, "clean": true, "clear": true,
		// Include -ing forms
		"creating": true, "making": true, "building": true, "writing": true, "adding": true,
		"implementing": true, "installing": true, "setting": true, "configuring": true,
		"editing": true, "modifying": true, "updating": true, "changing": true,
		"fixing": true, "correcting": true, "repairing": true, "patching": true,
		"running": true, "executing": true, "launching": true, "starting": true,
		"deleting": true, "removing": true, "cleaning": true, "clearing": true,
	}

	for _, pair := range pairs {
		if implVerbs[pair.Verb] && looksLikeFile(pair.Artifact) {
			return true, RuleDecision{
				RequireTools: true,
				Confident:    true,
				Reason:       "Implementation verb '" + pair.Verb + "' with file '" + pair.Artifact + "'",
			}
		}
	}

	return false, RuleDecision{}
}

// ResearchCompletionRule: Context-aware continuation after research = YES (confident)
type ResearchCompletionRule struct{}

func (r *ResearchCompletionRule) Priority() int { return 80 }
func (r *ResearchCompletionRule) Name() string  { return "ResearchCompletion" }

func (r *ResearchCompletionRule) IsSatisfiedBy(pairs []ActionPair, messages []types.OpenAIMessage) (bool, RuleDecision) {
	implVerbs := map[string]bool{
		"create": true, "make": true, "build": true, "write": true, "add": true,
		"implement": true, "install": true, "setup": true, "configure": true,
		"edit": true, "modify": true, "update": true, "change": true,
		"fix": true, "correct": true, "repair": true, "patch": true,
		"run": true, "execute": true, "launch": true, "start": true,
		"delete": true, "remove": true, "clean": true, "clear": true,
		// Include -ing forms
		"creating": true, "making": true, "building": true, "writing": true, "adding": true,
		"implementing": true, "installing": true, "setting": true, "configuring": true,
		"editing": true, "modifying": true, "updating": true, "changing": true,
		"fixing": true, "correcting": true, "repairing": true, "patching": true,
		"running": true, "executing": true, "launching": true, "starting": true,
		"deleting": true, "removing": true, "cleaning": true, "clearing": true,
	}

	hasResearchDone := false
	hasImplVerb := false

	for _, pair := range pairs {
		if pair.Verb == "research_done" {
			hasResearchDone = true
		}
		if implVerbs[pair.Verb] {
			hasImplVerb = true
		}
	}

	if hasResearchDone && hasImplVerb {
		return true, RuleDecision{
			RequireTools: true,
			Confident:    true,
			Reason:       "Research phase complete, now implementation requested",
		}
	}

	return false, RuleDecision{}
}

// StrongVerbWithoutArtifactRule: Strong implementation verbs without clear artifacts = YES (less confident)
type StrongVerbWithoutArtifactRule struct{}

func (r *StrongVerbWithoutArtifactRule) Priority() int { return 70 }
func (r *StrongVerbWithoutArtifactRule) Name() string  { return "StrongVerbWithoutArtifact" }

func (r *StrongVerbWithoutArtifactRule) IsSatisfiedBy(pairs []ActionPair, messages []types.OpenAIMessage) (bool, RuleDecision) {
	strongVerbs := map[string]bool{
		"create": true, "write": true, "edit": true, "update": true,
		"fix": true, "implement": true, "build": true, "run": true, "debug": true,
		"creating": true, "writing": true, "editing": true, "updating": true,
		"fixing": true, "implementing": true, "building": true, "running": true, "debugging": true,
	}

	for _, pair := range pairs {
		if strongVerbs[pair.Verb] {
			// Be confident for certain very strong verbs even without artifacts
			confident := pair.Verb == "run" || pair.Verb == "running" || 
						pair.Verb == "build" || pair.Verb == "building" ||
						pair.Verb == "fix" || pair.Verb == "fixing" ||
						pair.Verb == "create" || pair.Verb == "creating" ||
						pair.Verb == "implement" || pair.Verb == "implementing" ||
						pair.Verb == "update" || pair.Verb == "updating" ||
						pair.Verb == "debug" || pair.Verb == "debugging"
			return true, RuleDecision{
				RequireTools: true,
				Confident:    confident,
				Reason:       "Strong implementation verb '" + pair.Verb + "' detected",
			}
		}
	}

	return false, RuleDecision{}
}

// PureResearchRule: Pure research verbs without implementation = NO (confident)
type PureResearchRule struct{}

func (r *PureResearchRule) Priority() int { return 60 }
func (r *PureResearchRule) Name() string  { return "PureResearch" }

func (r *PureResearchRule) IsSatisfiedBy(pairs []ActionPair, messages []types.OpenAIMessage) (bool, RuleDecision) {
	researchVerbs := map[string]bool{
		"read": true, "analyze": true, "examine": true, "check": true, "review": true,
		"explain": true, "describe": true, "tell": true, "show": true, "list": true,
		"find": true, "search": true, "look": true, "investigate": true, "explore": true,
		"understand": true, "learn": true, "study": true, "research": true,
	}

	implVerbs := map[string]bool{
		"create": true, "make": true, "build": true, "write": true, "add": true,
		"implement": true, "install": true, "setup": true, "configure": true,
		"edit": true, "modify": true, "update": true, "change": true,
		"fix": true, "correct": true, "repair": true, "patch": true,
		"run": true, "execute": true, "launch": true, "start": true,
		"delete": true, "remove": true, "clean": true, "clear": true,
		// Include -ing forms
		"creating": true, "making": true, "building": true, "writing": true, "adding": true,
		"implementing": true, "installing": true, "setting": true, "configuring": true,
		"editing": true, "modifying": true, "updating": true, "changing": true,
		"fixing": true, "correcting": true, "repairing": true, "patching": true,
		"running": true, "executing": true, "launching": true, "starting": true,
		"deleting": true, "removing": true, "cleaning": true, "clearing": true,
	}

	hasOnlyResearch := false
	hasImplementation := false

	for _, pair := range pairs {
		if researchVerbs[pair.Verb] {
			hasOnlyResearch = true
		}
		if implVerbs[pair.Verb] {
			hasImplementation = true
		}
	}

	if hasOnlyResearch && !hasImplementation {
		return true, RuleDecision{
			RequireTools: false,
			Confident:    true,
			Reason:       "Only research/analysis verbs detected, no implementation",
		}
	}

	return false, RuleDecision{}
}

// ContextualNegationRule: Highest priority rule for explanation/hypothetical requests = NO (confident)
type ContextualNegationRule struct{}

func (r *ContextualNegationRule) Priority() int { return 110 }
func (r *ContextualNegationRule) Name() string  { return "ContextualNegation" }

func (r *ContextualNegationRule) IsSatisfiedBy(pairs []ActionPair, messages []types.OpenAIMessage) (bool, RuleDecision) {
	// Check for our special contextual negation marker
	for _, pair := range pairs {
		if pair.Verb == "explanation_only" && pair.Artifact == "contextual_negation" {
			return true, RuleDecision{
				RequireTools: false,
				Confident:    true,
				Reason:       "Explanation/hypothetical request detected (contextual negation)",
			}
		}
	}

	return false, RuleDecision{}
}

// AmbiguousRequestRule: Fallback rule for ambiguous cases = LLM analysis needed (not confident)
type AmbiguousRequestRule struct{}

func (r *AmbiguousRequestRule) Priority() int { return 10 }
func (r *AmbiguousRequestRule) Name() string  { return "AmbiguousRequest" }

func (r *AmbiguousRequestRule) IsSatisfiedBy(pairs []ActionPair, messages []types.OpenAIMessage) (bool, RuleDecision) {
	// This rule always matches as a fallback
	return true, RuleDecision{
		RequireTools: false,
		Confident:    false,
		Reason:       "Ambiguous request, requires LLM analysis",
	}
}
