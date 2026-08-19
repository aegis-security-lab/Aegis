package control

import (
	"fmt"
	"path"
	"strings"
	"time"

	"gorm.io/gorm"
)

func planningPrompt(i Issue, maxDepth, maxPerRequest, maxDirect int, budget IssueBudgetConfig) string {
	return fmt.Sprintf(`Decompose this real software task into executable child Issues.
Issue %s: %s
Description: %s
Objective: %s
Workspace: %s
Use the system-provided Agent type roster to select the best role for each child Issue. Reusing one Agent type is allowed: every child receives a distinct task-local identity, Session and Phone.
	Create only the next small, useful wave by calling the Phone Board shortcut phone_board_delegate exactly once with 2-%d independently verifiable Issues. Do not dispatch the entire project up front. The configured hierarchy permits depth %d and at most %d direct children per Issue. Every child scope must be realistically completable and verifiable within one Execution budget of %d model turns and %d active minutes. Split large repositories, modules, or audit surfaces into smaller outcome-based slices instead of assigning one Agent an exhaustive review of tens of thousands of lines. Every objective must state the concrete outcome and acceptance evidence. Continue useful parent work after dispatch; use phone_board_sleep only when no valuable action remains. Board heartbeats wake released waiting loops and do not interrupt active work; Phone messages may still steer when useful.`, i.Identifier, i.Title, i.Description, i.Objective, i.Workspace, maxPerRequest, maxDepth, maxDirect, budget.MaxTurns, budget.ActiveTimeMinutes)
}

func workerPrompt(i Issue, maxDepth, maxPerRequest, maxDirect int, budget IssueBudgetConfig) string {
	completionInstruction := "Before ending the turn, publish a complete standalone final report and every required supporting deliverable with aegis_publish_attachment, then call aegis_submit_final_result. The user-facing package contains only the attachments from this latest submission; neither your submission body nor your final chat prose is included. The report must let the user understand, verify, and reproduce the completed work without reading comments or earlier submissions. That explicit Agent action immediately starts acceptance."
	if strings.TrimSpace(i.Objective) == "" {
		completionInstruction = "Before ending the turn, publish a complete standalone final report and every required supporting deliverable with aegis_publish_attachment, then call aegis_submit_final_result with a concise evidence-based result. Only the latest submission's attachments are user-facing, so they must be independently usable without your final prose or earlier submissions."
	}
	return fmt.Sprintf(`Complete this Issue in the real workspace.
Issue %s: %s
Description: %s
Objective: %s
Workspace: %s
Use tools to inspect and modify the project, run relevant validation, fix in-scope failures, and finish with a concise evidence-based report.
For every user-facing deliverable file you generate (reports, archives, images, documents, or datasets), call aegis_publish_attachment before ending the turn so the tool uploads it directly to Aegis. Treat each final submission as a full replacement, never an incremental patch: if validation previously failed, publish a newly consolidated report/package containing all still-valid material and every correction. Source-code edits are collected separately, but the user-facing report must still document the completed work and reproducible verification evidence.

This child Execution has a work budget of %d model turns and %d active minutes, followed only by a restricted summary window. If this Issue cannot be completed and verified inside that budget, call the Phone Board shortcut phone_board_delegate once with only the next small wave of 2-%d independently verifiable child Issues before doing broad exploration. Large modules, repositories, and audit surfaces must be split into smaller outcome-based slices; do not accept an exhaustive tens-of-thousands-of-lines scope as one Execution. Reusing the same Agent type is allowed because each Issue receives a unique task-local identity, Session and Phone. Continue useful work after dispatch. Use phone_board_sleep only when there is no valuable action left; heartbeats wake released waiting loops rather than interrupting active work, while child completion, comments or Phone Relay may still steer or wake this session.

%s`, i.Identifier, i.Title, i.Description, i.Objective, i.Workspace, budget.MaxTurns, budget.ActiveTimeMinutes, maxPerRequest, completionInstruction)
}

func continuationPrompt(parent Issue, children []Issue) string {
	var summaries strings.Builder
	for _, child := range children {
		fmt.Fprintf(&summaries, "- %s [%s/%s] %s\n  id: %s\n  Agent: %s\n  Result: %s\n  Error: %s\n", child.Identifier, child.Status, child.ExecutionPhase, child.Title, child.ID, fallback(child.AssigneeAgentID, "unassigned"), fallback(truncate(strings.TrimSpace(child.Result), 1800), "No result recorded."), fallback(child.Error, "none"))
	}
	completionInstruction := "Only after every material parent-objective requirement passes your evidence review may you call aegis_submit_final_result. The final response must map each requirement to concrete evidence because it will be independently evaluated by the acceptance Agent."
	if strings.TrimSpace(parent.Objective) == "" {
		completionInstruction = "This parent Issue has no acceptance objective, so no acceptance Agent will run; finish with a concise evidence-based integration report."
	}
	return fmt.Sprintf(`Resume the parent Issue because one child completed or the requested coordination check became due. This is one coordination wakeup; other children may still be running.

Parent %s: %s
Description: %s
Objective: %s
Workspace: %s

Complete direct child status list (completed and unfinished):
%s
You are the acceptance owner for the parent objective. Call aegis_list_child_issues to refresh the complete direct-child state, then build an explicit parent-level acceptance checklist from every material requirement in the Objective and Description. For each requirement, classify it as PASS, FAIL, or UNPROVEN and cite concrete child or workspace evidence. A child being done, accepted, or well-written proves only that child's objective; it never proves the parent objective by itself. Summaries, effort, broad coverage, partial success, and adjacent findings are not substitutes for the exact requested outcome.

If any parent requirement is FAIL or UNPROVEN, the parent is not complete and you MUST continue implementation instead of producing a final integration report or calling aegis_submit_final_result. When correction or additional evidence belongs to an existing child, post a concrete Board comment describing the failed acceptance criterion and notify that owner to continue. When the unmet criterion requires genuinely distinct work, dispatch the next small dependency-free wave with aegis_create_subissues. You may also perform only bounded parent-level integration or verification work that is not appropriate to delegate. After continued or new child work, estimate the next check interval, call aegis_wait_for_child_issues with estimatedWaitMinutes, and end the turn.

Treat hard goals as binary: unless the exact required capability, artifact, access, behavior, or measurable outcome is demonstrated with reproducible evidence, acceptance fails and exploration or implementation continues. Do not reinterpret, weaken, or replace the parent objective merely to close the Issue. An impossibility conclusion is not success and is allowed only through the configured validation policy with concrete proof; do not substitute a report for an unmet objective. Repeat the acceptance-and-implementation loop until every material requirement is PASS. Only then run parent-level verification, publish requested user-facing deliverables with aegis_publish_attachment, and submit the evidence-based final result. %s`, parent.Identifier, parent.Title, parent.Description, parent.Objective, parent.Workspace, summaries.String(), completionInstruction)
}

const childCommentContextMaxLines = 2000
const childCommentContextMaxBytes = 50 * 1024

func (m *Manager) continuationPromptWithChildComments(parent Issue, children []Issue) string {
	prompt := continuationPrompt(parent, children)
	if len(children) == 0 {
		return prompt
	}
	childIDs := make([]string, len(children))
	labels := make(map[string]string, len(children))
	for index, child := range children {
		childIDs[index] = child.ID
		labels[child.ID] = child.Identifier + " · " + child.Title
	}
	var comments []IssueComment
	if err := m.store.db.Where("issue_id IN ?", childIDs).Order("created_at asc").Find(&comments).Error; err != nil || len(comments) == 0 {
		return prompt + "\n\n## Direct child Issue comments\n\nNo child comments were recorded."
	}
	var full strings.Builder
	full.WriteString("# Direct child Issue comment history\n\n")
	full.WriteString("Parent: " + parent.Identifier + " · " + parent.Title + "\n\n")
	for _, comment := range comments {
		fmt.Fprintf(&full, "## %s\n\n- Time: %s\n- Author: %s/%s\n- Type: %s\n\n%s\n\n", labels[comment.IssueID], comment.CreatedAt.Format(time.RFC3339), fallback(comment.AuthorType, "unknown"), fallback(comment.AuthorID, "unknown"), fallback(comment.Type, "normal"), fallback(strings.TrimSpace(comment.Body), "(empty comment)"))
	}
	complete := full.String()
	tail, truncated := truncateTextTail(complete, childCommentContextMaxLines, childCommentContextMaxBytes)
	if !truncated {
		return prompt + "\n\n## Complete direct child Issue comments\n\n" + complete
	}
	path, err := m.writeTaskRuntimeFile(parent, path.Join(".aegis", "context", "child-comments-"+parent.ID+".md"), []byte(complete))
	if err != nil {
		return prompt + "\n\n## Recent direct child Issue comments (truncated from the beginning)\n\nThe complete comment history could not be persisted: " + err.Error() + "\n\n" + tail
	}
	return prompt + fmt.Sprintf("\n\n## Recent direct child Issue comments (latest %d lines / %d bytes)\n\nThe full child comment history exceeded the prompt limit. Aegis preserved the newest content below and saved the complete history to `%s`. You MUST read that file with the read tool using offset/limit before making completion, coverage, cancellation, or new-delegation decisions; do not assume the visible tail is the whole history.\n\n%s", childCommentContextMaxLines, childCommentContextMaxBytes, path, tail)
}

func truncateTextTail(value string, maxLines, maxBytes int) (string, bool) {
	if len(value) <= maxBytes && strings.Count(value, "\n")+1 <= maxLines {
		return value, false
	}
	lines := strings.Split(value, "\n")
	start := len(lines)
	bytes := 0
	for start > 0 && len(lines)-start < maxLines {
		lineBytes := len([]byte(lines[start-1]))
		if start < len(lines) {
			lineBytes++
		}
		if bytes+lineBytes > maxBytes {
			break
		}
		bytes += lineBytes
		start--
	}
	if start == len(lines) {
		runes := []rune(value)
		for len(runes) > 0 && len([]byte(string(runes))) > maxBytes {
			runes = runes[1:]
		}
		return string(runes), true
	}
	return strings.Join(lines[start:], "\n"), true
}

func wakeupPrompt(i Issue, w AgentWakeup, db *gorm.DB) (string, error) {
	var comment IssueComment
	if err := db.First(&comment, "id = ? AND issue_id = ?", w.CommentID, i.ID).Error; err != nil {
		return "", fmt.Errorf("load wakeup comment: %w", err)
	}
	trigger := "The operator added a comment to an Issue assigned to you."
	if w.Reason == "issue_comment_mentioned" {
		trigger = "You were explicitly mentioned in an Issue comment."
	} else if w.Reason == "issue_comment_resume" {
		trigger = "The operator added a comment while this Issue was not running. This permanently reopens the Issue as active work. Continue execution from its durable history and current objective; do not treat this as a temporary question-and-answer turn."
	} else if w.Reason == "validation_feedback" {
		trigger = "The acceptance Agent posted structured validation feedback. Continue the same Issue and submit a complete replacement delivery when the work is ready for validation again. The next user-facing package contains only that latest submission's attachments: consolidate the full final report and all required support files instead of publishing an addendum or delta."
	}
	validationInstruction := "If you perform new work without splitting, the Issue's objective and validation settings remain authoritative."
	if strings.TrimSpace(i.Objective) == "" {
		validationInstruction = "This Issue has no acceptance objective. Your response or scoped work will not start an acceptance-validation flow."
	}
	return fmt.Sprintf(`%s

Issue %s: %s
Description: %s
Objective: %s
Workspace: %s

Comment from %s:
<comment>
%s
</comment>

	Respond to the operator's comment concretely in this same Issue. You MUST explicitly call aegis_board with action=comment so your answer is visible as a reply on this Issue; a plain assistant response is not sufficient. If this wakeup reopens inactive work, continue until the current objective is complete and call aegis_submit_final_result with the complete replacement delivery. If the comment requests a final delivery, call aegis_submit_final_result; it publishes immediately to Board. If independently executable child work is required, create and assign it through Board or the decomposition tool using the system-provided organization delegation boundary. %s`, trigger, i.Identifier, i.Title, i.Description, i.Objective, i.Workspace, comment.AuthorID, comment.Body, validationInstruction), nil
}
