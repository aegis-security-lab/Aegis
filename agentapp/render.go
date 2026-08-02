package agentapp

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"
)

func FinalizePage(page Page, canBack bool) Page {
	page.CanBack = canBack
	page.CanHome = true
	page.GeneratedAt = time.Now().UTC()
	page.Refs = append([]Ref(nil), page.Refs...)
	if canBack {
		page.Refs = append(page.Refs, Ref{Ref: "@back", Kind: "system", Label: "Back", Actions: []string{ActionBack, ActionSwipe}})
	}
	page.Refs = append(page.Refs,
		Ref{Ref: "@home", Kind: "system", Label: "Home", Actions: []string{ActionHome, ActionSwipe}},
		Ref{Ref: "@refresh", Kind: "system", Label: "Refresh", Actions: []string{ActionRefresh}},
	)
	page.Revision = pageRevision(page)
	page.Text = RenderText(page)
	return page
}

func pageRevision(page Page) string {
	var source strings.Builder
	source.WriteString(page.AppID)
	source.WriteByte('|')
	source.WriteString(page.PageID)
	source.WriteByte('|')
	source.WriteString(page.Title)
	source.WriteByte('|')
	source.WriteString(page.Summary)
	for _, section := range page.Sections {
		source.WriteByte('|')
		source.WriteString(section.Title)
		for _, line := range section.Lines {
			source.WriteByte('|')
			source.WriteString(line.Ref)
			source.WriteByte(':')
			source.WriteString(line.Label)
			source.WriteByte(':')
			source.WriteString(line.Detail)
		}
	}
	for _, ref := range page.Refs {
		source.WriteByte('|')
		source.WriteString(ref.Ref)
		source.WriteByte(':')
		source.WriteString(ref.Target)
	}
	sum := sha256.Sum256([]byte(source.String()))
	return "rev_" + hex.EncodeToString(sum[:8])
}

func RenderText(page Page) string {
	var out strings.Builder
	refs := make(map[string]Ref, len(page.Refs))
	for _, ref := range page.Refs {
		refs[ref.Ref] = ref
	}
	fmt.Fprintf(&out, "[APP] %s (%s)\n", safeText(page.TitleAppName()), safeText(page.AppID))
	fmt.Fprintf(&out, "[PAGE] %s\n", safeText(page.Title))
	fmt.Fprintf(&out, "[REVISION] %s\n", page.Revision)
	if page.Summary != "" {
		fmt.Fprintf(&out, "[SUMMARY]\n%s\n", safeText(page.Summary))
	}
	for _, section := range page.Sections {
		fmt.Fprintf(&out, "\n[SECTION] %s\n", safeText(section.Title))
		for _, line := range section.Lines {
			if line.Ref != "" {
				fmt.Fprintf(&out, "%s ", line.Ref)
			}
			if line.Kind != "" {
				fmt.Fprintf(&out, "[%s] ", line.Kind)
			}
			out.WriteString(safeText(line.Label))
			if line.Detail != "" {
				fmt.Fprintf(&out, "\n    %s", safeText(line.Detail))
			}
			if ref, ok := refs[line.Ref]; ok && len(ref.Actions) > 0 {
				fmt.Fprintf(&out, "\n    Actions: %s", strings.Join(ref.Actions, ", "))
			}
			out.WriteByte('\n')
		}
	}
	out.WriteString("\n[NAV]\n")
	for _, ref := range page.Refs {
		if !strings.HasPrefix(ref.Ref, "@") {
			continue
		}
		if ref.Kind == "system" {
			fmt.Fprintf(&out, "%s [system] %s · Actions: %s\n", ref.Ref, ref.Label, strings.Join(ref.Actions, ", "))
		}
	}
	return strings.TrimSpace(out.String()) + "\n"
}

// safeText keeps data supplied by users or other agents on one logical line.
// Only the renderer may create control records such as [SECTION] and @refs.
func safeText(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

func (p Page) TitleAppName() string {
	if p.Metadata != nil && p.Metadata["appName"] != "" {
		return p.Metadata["appName"]
	}
	return p.AppID
}

func HasAction(ref Ref, action string) bool {
	for _, allowed := range ref.Actions {
		if allowed == action {
			return true
		}
	}
	return false
}

func SortedMetadata(metadata map[string]string) string {
	keys := make([]string, 0, len(metadata))
	for key := range metadata {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+": "+metadata[key])
	}
	return strings.Join(parts, " · ")
}
