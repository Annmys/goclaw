package agent

import (
	"fmt"
	"strings"
	"time"

	"github.com/nextlevelbuilder/goclaw/internal/skills"
)

// GenerateSkillDraft creates a SKILL.md template from repeated tool usage data.
// Produces a skeleton that admin can edit before activation via evolution approval.
func GenerateSkillDraft(toolName string, callCount int, successRate float64) string {
	return fmt.Sprintf(`---
name: %s-patterns
description: Skill auto-generated from repeated %s tool usage (%d calls/week, %.0f%% success)
family: %s
display_name: %s Patterns
canonical: true
---

# %s Usage Patterns

Auto-generated from tool metrics. Edit before activating.

## When to Use

Describe scenarios where this tool pattern should be applied automatically.

## Instructions

Provide specific instructions for using %s effectively based on observed patterns.

## Constraints

List any constraints or guardrails for this tool usage.
`, toolName, toolName, callCount, successRate*100, skills.Slugify(toolName), toolName, toolName, toolName)
}

// FeedbackInsight captures one user feedback item for an auto-evolving skill draft.
type FeedbackInsight struct {
	CreatedAt      time.Time
	FeedbackType   string
	SessionKey     string
	MessageRef     string
	MessageContent string
	Correction     string
	UserID         string
}

// GenerateFeedbackSkillDraft creates an auto-evolving SKILL.md draft from user feedback history.
// The generated skill is meant to sit alongside the agent's normal skills and give it a durable
// memory of repeated corrections.
func GenerateFeedbackSkillDraft(agentKey, displayName, slug string, items []FeedbackInsight) string {
	title := displayName
	if strings.TrimSpace(title) == "" {
		title = agentKey
	}
	var b strings.Builder
	fmt.Fprintf(&b, `---
name: %s 反馈记忆
description: Auto-generated feedback memory skill for %s (%s)
slug: %s
family: %s
display_name: %s 反馈记忆
canonical: true
---

# %s 反馈记忆

This skill is maintained automatically from user feedback. Use the latest corrections as durable guidance.

## Current Rules

- Prefer the most recent user correction over older assumptions.
- If a previous response was marked not useful, do not repeat that pattern.
- When the feedback points to a concrete mistake, treat the correction as the next-response rule.

## Recent Feedback

`, title, title, agentKey, slug, skills.Slugify(slug), title, title)

	if len(items) == 0 {
		b.WriteString("- No feedback has been recorded yet.\n")
		return b.String()
	}

	for _, item := range items {
		when := item.CreatedAt.UTC().Format(time.RFC3339)
		fmt.Fprintf(&b, "### %s - %s\n", when, item.FeedbackType)
		if item.SessionKey != "" {
			fmt.Fprintf(&b, "- Session: %s\n", item.SessionKey)
		}
		if item.MessageRef != "" {
			fmt.Fprintf(&b, "- Message Ref: %s\n", item.MessageRef)
		}
		if item.UserID != "" {
			fmt.Fprintf(&b, "- User: %s\n", item.UserID)
		}
		if item.MessageContent != "" {
			fmt.Fprintf(&b, "- Message: %s\n", normalizeFeedbackLine(item.MessageContent))
		}
		if item.Correction != "" {
			fmt.Fprintf(&b, "- Correction: %s\n", normalizeFeedbackLine(item.Correction))
		}
		b.WriteString("\n")
	}

	return b.String()
}

func normalizeFeedbackLine(s string) string {
	s = strings.ReplaceAll(s, "\r\n", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.TrimSpace(s)
	if len(s) > 400 {
		s = s[:400] + "..."
	}
	return s
}
