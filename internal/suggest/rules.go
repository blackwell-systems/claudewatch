package suggest

import "fmt"

// MissingClaudeMD suggests creating a CLAUDE.md for projects that have
// sessions but no CLAUDE.md file.
func MissingClaudeMD(ctx *AnalysisContext) []Suggestion {
	var suggestions []Suggestion
	for _, p := range ctx.Projects {
		if p.SessionCount > 0 && !p.HasClaudeMD {
			suggestions = append(suggestions, Suggestion{
				Category: "configuration",
				Priority: PriorityHigh,
				Title:    fmt.Sprintf("Add CLAUDE.md to %s", p.Name),
				Description: fmt.Sprintf(
					"Project %q has %d sessions but no CLAUDE.md. "+
						"Adding a CLAUDE.md improves Claude's understanding of project context, "+
						"coding conventions, and reduces friction from wrong approaches.",
					p.Name, p.SessionCount,
				),
				ImpactScore: ComputeImpact(p.SessionCount, 1.0, 5.0, 15.0),
			})
		}
	}
	return suggestions
}

// RecurringFriction suggests interventions for friction types that appear
// in more than 30% of sessions.
func RecurringFriction(ctx *AnalysisContext) []Suggestion {
	var suggestions []Suggestion
	for _, frictionType := range ctx.RecurringFriction {
		frequency := 0.3 // minimum threshold to be in this list
		suggestions = append(suggestions, Suggestion{
			Category: "friction",
			Priority: PriorityHigh,
			Title:    fmt.Sprintf("Address recurring friction: %s", frictionType),
			Description: fmt.Sprintf(
				"Friction type %q appears in >30%% of sessions (%d total sessions). "+
					"Consider adding project-specific instructions to CLAUDE.md to prevent this pattern, "+
					"or configure hooks to catch it early.",
				frictionType, ctx.TotalSessions,
			),
			ImpactScore: ComputeImpact(ctx.TotalSessions, frequency, 3.0, 10.0),
		})
	}
	return suggestions
}

// HookGaps suggests adding hooks when few or no hooks are configured.
func HookGaps(ctx *AnalysisContext) []Suggestion {
	var suggestions []Suggestion

	if ctx.HookCount == 0 {
		suggestions = append(suggestions, Suggestion{
			Category: "configuration",
			Priority: PriorityMedium,
			Title:    "Configure Claude Code hooks",
			Description: "No hooks are configured. Hooks automate pre/post actions for Claude sessions. " +
				"Consider adding PreToolUse hooks for safety checks, PostToolUse hooks for formatting, " +
				"and SessionEnd hooks for automated metric logging.",
			ImpactScore: ComputeImpact(ctx.TotalSessions, 0.5, 2.0, 10.0),
		})
	} else if ctx.HookCount < 3 {
		suggestions = append(suggestions, Suggestion{
			Category: "configuration",
			Priority: PriorityLow,
			Title:    "Expand hook coverage",
			Description: fmt.Sprintf(
				"Only %d hook(s) configured. Consider adding hooks for: "+
					"PreToolUse (safety), PostToolUse (formatting), SessionEnd (metric logging).",
				ctx.HookCount,
			),
			ImpactScore: ComputeImpact(ctx.TotalSessions, 0.3, 1.0, 5.0),
		})
	}

	return suggestions
}

// UnusedSkills flags custom commands that appear to be unused based on
// having commands defined but low agent adoption.
func UnusedSkills(ctx *AnalysisContext) []Suggestion {
	var suggestions []Suggestion

	if ctx.CommandCount > 0 && ctx.TotalSessions > 5 {
		// If we have commands but very few sessions use task agents,
		// the commands may be underutilized.
		agentRatio := 0.0
		if ctx.TotalSessions > 0 {
			totalAgents := 0
			for _, p := range ctx.Projects {
				totalAgents += p.AgentCount
			}
			agentRatio = float64(totalAgents) / float64(ctx.TotalSessions)
		}

		if agentRatio < 0.1 {
			suggestions = append(suggestions, Suggestion{
				Category: "adoption",
				Priority: PriorityLow,
				Title:    "Custom commands may be underutilized",
				Description: fmt.Sprintf(
					"You have %d custom commands defined but agent/skill usage is low (%.0f%% of sessions). "+
						"Consider incorporating these commands into your workflow or removing unused ones.",
					ctx.CommandCount, agentRatio*100,
				),
				ImpactScore: ComputeImpact(ctx.TotalSessions, 0.2, 1.0, 5.0),
			})
		}
	}

	return suggestions
}

// HighErrorProjects flags projects with tool errors more than 2x the average.
func HighErrorProjects(ctx *AnalysisContext) []Suggestion {
	var suggestions []Suggestion

	if ctx.AvgToolErrors <= 0 {
		return suggestions
	}

	threshold := ctx.AvgToolErrors * 2.0

	for _, p := range ctx.Projects {
		if p.SessionCount == 0 {
			continue
		}
		projectAvgErrors := float64(p.ToolErrors) / float64(p.SessionCount)
		if projectAvgErrors > threshold {
			suggestions = append(suggestions, Suggestion{
				Category: "quality",
				Priority: PriorityHigh,
				Title:    fmt.Sprintf("High tool errors in %s", p.Name),
				Description: fmt.Sprintf(
					"Project %q averages %.1f tool errors per session, which is %.1fx the overall average (%.1f). "+
						"This often indicates missing permissions, incorrect file paths in CLAUDE.md, "+
						"or tools that need configuration.",
					p.Name, projectAvgErrors, projectAvgErrors/ctx.AvgToolErrors, ctx.AvgToolErrors,
				),
				ImpactScore: ComputeImpact(p.SessionCount, 0.8, 3.0, 10.0),
			})
		}
	}

	return suggestions
}

// AgentAdoption suggests using task agents if they are rarely used.
func AgentAdoption(ctx *AnalysisContext) []Suggestion {
	var suggestions []Suggestion

	if ctx.TotalSessions < 5 {
		return suggestions
	}

	totalAgents := 0
	for _, p := range ctx.Projects {
		totalAgents += p.AgentCount
	}

	agentSessionRatio := float64(totalAgents) / float64(ctx.TotalSessions)
	if agentSessionRatio < 0.1 {
		suggestions = append(suggestions, Suggestion{
			Category: "adoption",
			Priority: PriorityMedium,
			Title:    "Consider using task agents",
			Description: fmt.Sprintf(
				"Agent usage is low (%.0f%% of %d sessions). Task agents can parallelize work, "+
					"handle exploration tasks in the background, and reduce session duration. "+
					"Try delegating research, documentation, or test-writing tasks to agents.",
				agentSessionRatio*100, ctx.TotalSessions,
			),
			ImpactScore: ComputeImpact(ctx.TotalSessions, 0.5, 5.0, 5.0),
		})
	}

	return suggestions
}

// InterruptionPattern suggests CLAUDE.md improvements for projects with
// high user interruption rates.
func InterruptionPattern(ctx *AnalysisContext) []Suggestion {
	var suggestions []Suggestion

	for _, p := range ctx.Projects {
		if p.SessionCount == 0 {
			continue
		}
		avgInterruptions := float64(p.Interruptions) / float64(p.SessionCount)
		// Flag projects averaging more than 3 interruptions per session.
		if avgInterruptions > 3.0 {
			suggestions = append(suggestions, Suggestion{
				Category: "friction",
				Priority: PriorityMedium,
				Title:    fmt.Sprintf("High interruption rate in %s", p.Name),
				Description: fmt.Sprintf(
					"Project %q averages %.1f user interruptions per session across %d sessions. "+
						"High interruption rates suggest Claude's approach frequently diverges from "+
						"expectations. Improve CLAUDE.md with coding conventions, preferred patterns, "+
						"and explicit constraints to reduce course corrections.",
					p.Name, avgInterruptions, p.SessionCount,
				),
				ImpactScore: ComputeImpact(p.SessionCount, 0.6, 3.0, 15.0),
			})
		}
	}

	return suggestions
}

// AgentTypeEffectiveness flags agent types with success rates below 70%.
func AgentTypeEffectiveness(ctx *AnalysisContext) []Suggestion {
	var suggestions []Suggestion

	for agentType, successRate := range ctx.AgentTypeStats {
		if successRate < 0.70 {
			suggestions = append(suggestions, Suggestion{
				Category: "agents",
				Priority: PriorityMedium,
				Title:    fmt.Sprintf("Low success rate for %s agents", agentType),
				Description: fmt.Sprintf(
					"Your %s agents succeed only %.0f%% of the time. "+
						"Consider breaking complex %s tasks into smaller, more focused agents, "+
						"or providing more specific instructions in the agent prompt.",
					agentType, successRate*100, agentType,
				),
				ImpactScore: ComputeImpact(ctx.TotalSessions, 1.0-successRate, 5.0, 10.0),
			})
		}
	}

	return suggestions
}

// ParallelizationOpportunity flags projects running multiple sequential
// agents that could potentially run in parallel.
func ParallelizationOpportunity(ctx *AnalysisContext) []Suggestion {
	var suggestions []Suggestion

	for _, p := range ctx.Projects {
		if p.SequentialCount > 2 {
			estimatedMinutes := float64(p.SequentialCount) * 0.5 // rough estimate
			suggestions = append(suggestions, Suggestion{
				Category: "agents",
				Priority: PriorityLow,
				Title:    fmt.Sprintf("Parallelization opportunity in %s", p.Name),
				Description: fmt.Sprintf(
					"Project %q ran %d agents sequentially that could have been parallel, "+
						"costing an estimated %.0f extra minutes. "+
						"Use background agents for independent tasks like exploration, documentation, "+
						"and test writing.",
					p.Name, p.SequentialCount, estimatedMinutes,
				),
				ImpactScore: ComputeImpact(p.SessionCount, 0.4, estimatedMinutes, 5.0),
			})
		}
	}

	return suggestions
}

// CustomMetricRegression flags custom metrics trending in the wrong direction.
func CustomMetricRegression(ctx *AnalysisContext) []Suggestion {
	var suggestions []Suggestion

	for metricName, trend := range ctx.CustomMetricTrends {
		if trend == "regressing" {
			suggestions = append(suggestions, Suggestion{
				Category: "custom_metrics",
				Priority: PriorityMedium,
				Title:    fmt.Sprintf("Regression in custom metric: %s", metricName),
				Description: fmt.Sprintf(
					"Custom metric %q has been trending in the wrong direction. "+
						"Review recent sessions and configuration changes to identify "+
						"what may have caused this regression.",
					metricName,
				),
				ImpactScore: ComputeImpact(ctx.TotalSessions, 0.5, 3.0, 10.0),
			})
		}
	}

	return suggestions
}

// ClaudeMDSectionSuggestions recommends adding missing CLAUDE.md sections when
// section correlation data indicates that having certain sections reduces friction.
func ClaudeMDSectionSuggestions(ctx *AnalysisContext) []Suggestion {
	var suggestions []Suggestion

	for section, frictionReduction := range ctx.ClaudeMDSectionCorrelation {
		if frictionReduction <= 0 {
			continue
		}

		// Find projects missing this section.
		for _, p := range ctx.Projects {
			if !p.HasClaudeMD {
				continue
			}
			// Check if this project is missing the section.
			for _, missing := range p.ClaudeMDMissingSections {
				if missing == section {
					impact := ComputeImpact(p.SessionCount, frictionReduction/100.0, 5.0, 10.0)
					suggestions = append(suggestions, Suggestion{
						Category: "quality",
						Priority: PriorityMedium,
						Title:    fmt.Sprintf("Add %q section to %s CLAUDE.md", section, p.Name),
						Description: fmt.Sprintf(
							"Projects with a %q section show %.0f%% less friction. "+
								"Adding this section to %s (which has %d sessions) could reduce "+
								"recurring friction in this project.",
							section, frictionReduction, p.Name, p.SessionCount,
						),
						ImpactScore: impact,
					})
					break
				}
			}
		}
	}

	return suggestions
}

// ZeroCommitRateSuggestion flags workflows with a high zero-commit rate (>40%).
func ZeroCommitRateSuggestion(ctx *AnalysisContext) []Suggestion {
	var suggestions []Suggestion

	if ctx.ZeroCommitRate <= 0.40 || ctx.TotalSessions < 5 {
		return suggestions
	}

	suggestions = append(suggestions, Suggestion{
		Category: "quality",
		Priority: PriorityHigh,
		Title:    "High zero-commit rate in sessions",
		Description: fmt.Sprintf(
			"%.0f%% of %d sessions produced zero commits. This may indicate exploratory "+
				"sessions without deliverables, or incomplete workflows that stall before "+
				"committing. Consider breaking large tasks into smaller commit-sized chunks, "+
				"using the /commit skill, or reviewing whether these sessions achieve their goals.",
			ctx.ZeroCommitRate*100, ctx.TotalSessions,
		),
		ImpactScore: ComputeImpact(ctx.TotalSessions, ctx.ZeroCommitRate, 5.0, 10.0),
	})

	return suggestions
}

// CostOptimizationSuggestion flags low cache savings percentages and suggests
// enabling or improving prompt caching.
func CostOptimizationSuggestion(ctx *AnalysisContext) []Suggestion {
	var suggestions []Suggestion

	if ctx.CacheSavingsPercent >= 20 || ctx.TotalCost <= 0 {
		return suggestions
	}

	suggestions = append(suggestions, Suggestion{
		Category: "configuration",
		Priority: PriorityMedium,
		Title:    "Low prompt cache savings",
		Description: fmt.Sprintf(
			"Cache savings are only %.0f%% of total cost ($%.2f). "+
				"Improving prompt caching can significantly reduce costs. "+
				"Ensure CLAUDE.md files are stable (frequent changes invalidate cache), "+
				"use consistent system prompts, and consider structuring prompts with "+
				"static context first followed by dynamic content.",
			ctx.CacheSavingsPercent, ctx.TotalCost,
		),
		ImpactScore: ComputeImpact(ctx.TotalSessions, 0.5, 5.0, 10.0),
	})

	return suggestions
}

