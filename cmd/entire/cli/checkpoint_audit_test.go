package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/entireio/cli/cmd/entire/cli/review/intentlens"
)

type checkpointAuditFakeTransport struct {
	response []byte
	err      error
	called   bool
	prompt   string
}

func (t *checkpointAuditFakeTransport) Generate(_ context.Context, _ string, prompt string, _ json.RawMessage) ([]byte, error) {
	t.called = true
	t.prompt = prompt
	return t.response, t.err
}

func TestCheckpointAuditCommandWiresEvaluatorAndRendersDashboard(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "set")
	report := checkpointAuditReportFixture()
	transport := &checkpointAuditFakeTransport{response: checkpointAuditResponseJSON()}
	cmd := checkpointAuditTestCommand(report, transport)
	var output bytes.Buffer
	cmd.SetOut(&output)
	cmd.SetErr(&output)
	cmd.SetArgs([]string{"checkpoint-123"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v\noutput: %s", err, output.String())
	}
	if !transport.called {
		t.Fatal("fake transport was not called")
	}
	for _, want := range []string{
		"IntentLens Audit",
		"checkpoint    checkpoint-123",
		"context       COMPLETE",
		"R1   IMPLEMENTED",
		"R2   INCOMPLETE",
		"R3   UNCERTAIN",
		"Recommendations",
		"R2: Connect rate limiting to the login route.",
	} {
		if !strings.Contains(output.String(), want) {
			t.Errorf("dashboard missing %q\n%s", want, output.String())
		}
	}
	assertDashboardLine(t, output.String(), "summary", "Login works, lockout is incomplete, and session behavior is uncertain.")
	assertDashboardLine(t, output.String(), "findings", "IMPLEMENTED 1 | INCOMPLETE 1 | UNCERTAIN 1")
	for _, forbidden := range []string{"diff --git", "super-secret-value", "BEGIN TRANSCRIPT", "SECRET_TRANSCRIPT_9f1c2d"} {
		if strings.Contains(transport.prompt, forbidden) {
			t.Fatalf("Gemini prompt leaked %q: %q", forbidden, transport.prompt)
		}
		if strings.Contains(output.String(), forbidden) {
			t.Fatalf("dashboard leaked %q: %q", forbidden, output.String())
		}
	}
}

func TestCheckpointAuditCommandJSONOutputsValidatedAudit(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "set")
	transport := &checkpointAuditFakeTransport{response: checkpointAuditResponseJSON()}
	cmd := checkpointAuditTestCommand(checkpointAuditReportFixture(), transport)
	var output bytes.Buffer
	cmd.SetOut(&output)
	cmd.SetErr(&output)
	cmd.SetArgs([]string{"checkpoint-123", "--json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v\noutput: %s", err, output.String())
	}
	var audit intentlens.Audit
	if err := json.Unmarshal(output.Bytes(), &audit); err != nil {
		t.Fatalf("decode audit JSON: %v\n%s", err, output.String())
	}
	if _, err := intentlens.ParseAuditJSON(output.Bytes()); err != nil {
		t.Fatalf("JSON output is not validated audit JSON: %v", err)
	}
	if audit.Summary != "Login works, lockout is incomplete, and session behavior is uncertain." {
		t.Fatalf("summary = %q", audit.Summary)
	}
	for _, forbidden := range []string{`"intent"`, `"implementation"`, `"checkpoint_id"`, "diff --git"} {
		if strings.Contains(output.String(), forbidden) {
			t.Fatalf("JSON output included collector evidence %q: %s", forbidden, output.String())
		}
	}
}

func TestCheckpointAuditCommandRequirementFilterShowsOnlySelectedDetails(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "set")
	transport := &checkpointAuditFakeTransport{response: checkpointAuditResponseJSON()}
	cmd := checkpointAuditTestCommand(checkpointAuditReportFixture(), transport)
	var output bytes.Buffer
	cmd.SetOut(&output)
	cmd.SetErr(&output)
	cmd.SetArgs([]string{"checkpoint-123", "--requirement", "R2"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v\noutput: %s", err, output.String())
	}
	for _, want := range []string{"Requirement Detail", "R2   INCOMPLETE", "Lockout middleware is not connected.", "Recommendation: Connect rate limiting to the login route."} {
		if !strings.Contains(output.String(), want) {
			t.Errorf("filtered output missing %q\n%s", want, output.String())
		}
	}
	for _, forbidden := range []string{"Credential verification is connected.", "Session preservation was not verified."} {
		if strings.Contains(output.String(), forbidden) {
			t.Fatalf("filtered output showed unselected evidence %q:\n%s", forbidden, output.String())
		}
	}
}

func TestCheckpointAuditCommandIncompleteContextIsConservative(t *testing.T) {
	report := checkpointAuditReportFixture()
	report.Intent.Prompts = nil
	transport := &checkpointAuditFakeTransport{err: errors.New("must not be called")}
	cmd := checkpointAuditTestCommand(report, transport)
	var output bytes.Buffer
	cmd.SetOut(&output)
	cmd.SetErr(&output)
	cmd.SetArgs([]string{"checkpoint-123"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v\noutput: %s", err, output.String())
	}
	if transport.called {
		t.Fatal("transport called for incomplete context")
	}
	for _, want := range []string{"context       INCOMPLETE", "R1   UNCERTAIN", "checkpoint intent was missing or redacted"} {
		if !strings.Contains(output.String(), want) {
			t.Errorf("incomplete-context output missing %q\n%s", want, output.String())
		}
	}
	assertDashboardLine(t, output.String(), "findings", "IMPLEMENTED 0 | INCOMPLETE 0 | UNCERTAIN 1")
}

func TestCheckpointAuditCommandMissingKeyError(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "")
	transport := &checkpointAuditFakeTransport{response: checkpointAuditResponseJSON()}
	cmd := checkpointAuditTestCommand(checkpointAuditReportFixture(), transport)
	var output bytes.Buffer
	cmd.SetOut(&output)
	cmd.SetErr(&output)
	cmd.SetArgs([]string{"checkpoint-123"})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "GEMINI_API_KEY") {
		t.Fatalf("error = %v, want clear missing key error", err)
	}
	if transport.called {
		t.Fatal("transport called without runtime key")
	}
}

func checkpointAuditTestCommand(report AuditReport, transport *checkpointAuditFakeTransport) *cobra.Command {
	return newCheckpointAuditCmdWithDeps(checkpointAuditDeps{
		collect: func(_ *cobra.Command, target string, _ int, _ string) (AuditReport, error) {
			report.Intent.CheckpointID = target
			return report, nil
		},
		evaluator: intentlens.NewGeminiEvaluator(transport),
	})
}

func checkpointAuditReportFixture() AuditReport {
	return AuditReport{
		Intent: IntentPacket{
			CheckpointID:         "checkpoint-123",
			Prompts:              []string{"Users can log in, failed login attempts lock after five tries, and sessions survive password changes."},
			DeclaredFilesTouched: []string{"auth/login.go", "auth/login_test.go"},
		},
		Implementation: ImplementationEvidence{
			LinkedCommits:      []string{"abc123"},
			ActualFilesTouched: []string{"auth/login.go", "auth/routes.go", "auth/login_test.go"},
			Diffs: map[string]string{
				"abc123": "diff --git a/auth/login.go b/auth/login.go\n+ SECRET_TRANSCRIPT_9f1c2d=super-secret-value",
			},
			FocusedTests:  []string{"auth/login_test.go"},
			GraphEvidence: json.RawMessage(`{"output":"BEGIN TRANSCRIPT SECRET_TRANSCRIPT_9f1c2d raw checkpoint content"}`),
		},
		Findings: []AuditFinding{},
	}
}

func assertDashboardLine(t *testing.T, output string, label string, value string) {
	t.Helper()
	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), label) && strings.Contains(line, value) {
			return
		}
	}
	t.Fatalf("dashboard missing %s value %q\n%s", label, value, output)
}

func checkpointAuditResponseJSON() []byte {
	return []byte(`{
		"summary": "Login works, lockout is incomplete, and session behavior is uncertain.",
		"requirements": [
			{
				"id": "R1",
				"requirement": "Users can log in.",
				"status": "IMPLEMENTED",
				"confidence": 0.92,
				"evidence": [
					{"type": "code", "file": "auth/login.go", "symbol": "Login", "explanation": "Credential verification is connected."},
					{"type": "test", "test_name": "TestLogin", "result": "passed", "explanation": "Login behavior is verified."}
				],
				"recommendation": ""
			},
			{
				"id": "R2",
				"requirement": "Failed login attempts lock after five tries.",
				"status": "INCOMPLETE",
				"confidence": 0.81,
				"evidence": [
					{"type": "graph", "reference": "route:/login -> Login", "explanation": "Lockout middleware is not connected."}
				],
				"recommendation": "Connect rate limiting to the login route."
			},
			{
				"id": "R3",
				"requirement": "Sessions survive password changes.",
				"status": "UNCERTAIN",
				"confidence": 0.2,
				"evidence": [
					{"type": "checkpoint", "reference": "sanitized evidence package", "explanation": "Session preservation was not verified."}
				],
				"recommendation": "Add verification for existing sessions after password changes."
			}
		]
	}`)
}
