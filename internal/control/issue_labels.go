package control

import (
	"encoding/json"
	"slices"
)

// Issue labels carry operational outcomes that are not workflow states. The
// workflow status is the five-state business projection (todo, in_progress,
// in_review, done, cancelled); blocked / failed / budget_exceeded outcomes
// stay visible as labels so boards never lose operational signal.
const (
	issueLabelBlocked        = "blocked"
	issueLabelFailed         = "failed"
	issueLabelBudgetExceeded = "budget_exceeded"
)

func hasIssueLabel(issue Issue, labels ...string) bool {
	for _, label := range labels {
		if slices.Contains(issue.Labels, label) {
			return true
		}
	}
	return false
}

func withIssueLabels(issue Issue, labels ...string) []string {
	next := slices.Clone(issue.Labels)
	for _, label := range labels {
		if label != "" && !slices.Contains(next, label) {
			next = append(next, label)
		}
	}
	slices.Sort(next)
	return next
}

func withoutIssueLabels(issue Issue, labels ...string) []string {
	next := make([]string, 0, len(issue.Labels))
	for _, label := range issue.Labels {
		if !slices.Contains(labels, label) {
			next = append(next, label)
		}
	}
	slices.Sort(next)
	return next
}

// issueLabelsColumn pre-serializes labels exactly like gorm's json serializer
// so map-based Updates can carry the JSON column value in a single statement.
func issueLabelsColumn(labels []string) string {
	data, err := json.Marshal(labels)
	if err != nil {
		return "[]"
	}
	return string(data)
}
