package intentlens

import (
	"fmt"
	"io"
	"strings"
)

const DemoNotice = "Demo evidence fixture — backend not connected"

type ViewState struct {
	Audit   *Audit
	Loading bool
	Demo    bool
	Err     error
}

type DashboardState struct {
	Audit         *Audit
	CheckpointID  string
	ContextStatus ContextStatus
	ContextNote   string
	RequirementID string
}

func Render(w io.Writer, state ViewState) {
	if state.Loading {
		fmt.Fprintln(w, "Loading audit result...")
		return
	}
	if state.Err != nil {
		fmt.Fprintf(w, "Could not display audit result: %v\n", state.Err)
		return
	}
	if state.Audit == nil || len(state.Audit.Requirements) == 0 {
		fmt.Fprintln(w, "No audit result was provided.")
		return
	}
	if state.Demo {
		fmt.Fprintln(w, DemoNotice)
		fmt.Fprintln(w)
	}
	fmt.Fprintln(w, "IntentLens Audit")
	fmt.Fprintln(w)
	fmt.Fprintln(w, state.Audit.Summary)
	for _, requirement := range state.Audit.Requirements {
		fmt.Fprintln(w)
		fmt.Fprintf(w, "%s  %s  %.0f%% confidence\n", requirement.ID, requirement.Status, requirement.Confidence*100)
		fmt.Fprintln(w, requirement.Requirement)
		fmt.Fprintln(w, "Evidence:")
		for _, evidence := range requirement.Evidence {
			fmt.Fprintf(w, "  - [%s] %s\n", evidence.Type, evidence.Explanation)
			var details []string
			for _, detail := range []struct{ label, value string }{
				{"file", evidence.File}, {"symbol", evidence.Symbol}, {"test", evidence.TestName},
				{"reference", evidence.Reference}, {"result", evidence.Result},
			} {
				if strings.TrimSpace(detail.value) != "" {
					details = append(details, detail.label+": "+detail.value)
				}
			}
			if len(details) > 0 {
				fmt.Fprintf(w, "    %s\n", strings.Join(details, " | "))
			}
		}
		if strings.TrimSpace(requirement.Recommendation) == "" {
			fmt.Fprintln(w, "Recommendation: none")
		} else {
			fmt.Fprintf(w, "Recommendation: %s\n", requirement.Recommendation)
		}
	}
}

func RenderDashboard(w io.Writer, state DashboardState) error {
	if state.Audit == nil || len(state.Audit.Requirements) == 0 {
		fmt.Fprintln(w, "No audit result was provided.")
		return nil
	}
	var selected *Requirement
	if strings.TrimSpace(state.RequirementID) != "" {
		for i := range state.Audit.Requirements {
			if state.Audit.Requirements[i].ID == state.RequirementID {
				selected = &state.Audit.Requirements[i]
				break
			}
		}
		if selected == nil {
			return fmt.Errorf("requirement %s not found in audit result", state.RequirementID)
		}
	}

	counts := countStatuses(state.Audit.Requirements)
	fmt.Fprintln(w, "IntentLens Audit")
	fmt.Fprintln(w, "----------------")
	printDashboardValue(w, "checkpoint", state.CheckpointID)
	if state.ContextStatus != "" {
		printDashboardValue(w, "context", string(state.ContextStatus))
	}
	printDashboardValue(w, "note", state.ContextNote)
	printDashboardValue(w, "summary", state.Audit.Summary)
	fmt.Fprintf(w, "%-13s IMPLEMENTED %d | INCOMPLETE %d | UNCERTAIN %d\n", "findings", counts[StatusImplemented], counts[StatusIncomplete], counts[StatusUncertain])

	fmt.Fprintln(w)
	fmt.Fprintln(w, "Requirements")
	for _, requirement := range state.Audit.Requirements {
		fmt.Fprintf(w, "  %-4s %-10s %3.0f%%  %s\n", requirement.ID, requirement.Status, requirement.Confidence*100, shortDashboardText(requirement.Requirement, 92))
	}

	if selected != nil {
		fmt.Fprintln(w)
		renderRequirementDetail(w, *selected)
		return nil
	}

	recommendations := topRecommendations(state.Audit.Requirements, 3)
	if len(recommendations) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "Recommendations")
		for _, recommendation := range recommendations {
			fmt.Fprintf(w, "  %s\n", recommendation)
		}
	}
	return nil
}

func renderRequirementDetail(w io.Writer, requirement Requirement) {
	fmt.Fprintln(w, "Requirement Detail")
	fmt.Fprintf(w, "  %-4s %-10s %3.0f%%  %s\n", requirement.ID, requirement.Status, requirement.Confidence*100, requirement.Requirement)
	if len(requirement.Evidence) > 0 {
		fmt.Fprintln(w, "  Evidence")
		for _, evidence := range requirement.Evidence {
			fmt.Fprintf(w, "    - [%s] %s\n", evidence.Type, evidence.Explanation)
			for _, detail := range evidenceDetails(evidence) {
				fmt.Fprintf(w, "      %s\n", detail)
			}
		}
	}
	if strings.TrimSpace(requirement.Recommendation) != "" {
		fmt.Fprintf(w, "  Recommendation: %s\n", requirement.Recommendation)
	}
}

func printDashboardValue(w io.Writer, label string, value string) {
	if strings.TrimSpace(value) == "" {
		return
	}
	fmt.Fprintf(w, "%-13s %s\n", label, value)
}

func countStatuses(requirements []Requirement) map[Status]int {
	counts := map[Status]int{
		StatusImplemented: 0,
		StatusIncomplete:  0,
		StatusUncertain:   0,
	}
	for _, requirement := range requirements {
		counts[requirement.Status]++
	}
	return counts
}

func topRecommendations(requirements []Requirement, limit int) []string {
	var recommendations []string
	for _, requirement := range requirements {
		recommendation := strings.TrimSpace(requirement.Recommendation)
		if recommendation == "" {
			continue
		}
		recommendations = append(recommendations, requirement.ID+": "+shortDashboardText(recommendation, 120))
		if len(recommendations) == limit {
			break
		}
	}
	return recommendations
}

func evidenceDetails(evidence Evidence) []string {
	var details []string
	for _, detail := range []struct{ label, value string }{
		{"file", evidence.File},
		{"symbol", evidence.Symbol},
		{"test", evidence.TestName},
		{"reference", evidence.Reference},
		{"result", evidence.Result},
	} {
		if strings.TrimSpace(detail.value) != "" {
			details = append(details, detail.label+": "+detail.value)
		}
	}
	return details
}

func shortDashboardText(value string, limit int) string {
	value = strings.TrimSpace(whitespacePattern.ReplaceAllString(value, " "))
	if value == "" {
		return ""
	}
	return truncateRunes(value, limit)
}
