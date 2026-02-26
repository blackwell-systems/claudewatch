package suggest

// Engine runs all registered rules against an AnalysisContext and collects
// the resulting suggestions.
type Engine struct {
	rules []Rule
}

// NewEngine creates a new suggest engine with all built-in rules registered.
func NewEngine() *Engine {
	return &Engine{
		rules: []Rule{
			MissingClaudeMD,
			RecurringFriction,
			HookGaps,
			UnusedSkills,
			HighErrorProjects,
			AgentAdoption,
			InterruptionPattern,
			AgentTypeEffectiveness,
			ParallelizationOpportunity,
			CustomMetricRegression,
		},
	}
}

// Run executes all registered rules against the given context and returns
// the collected suggestions sorted by impact score (highest first).
func (e *Engine) Run(ctx *AnalysisContext) []Suggestion {
	var all []Suggestion
	for _, rule := range e.rules {
		results := rule(ctx)
		all = append(all, results...)
	}
	return RankSuggestions(all)
}
