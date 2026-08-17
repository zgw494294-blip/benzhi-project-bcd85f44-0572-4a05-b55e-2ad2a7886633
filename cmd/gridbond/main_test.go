package main

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"gridbond/internal/trial"
)

func TestCLIWorkflowAndStatePreservingFailures(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ledger.json")
	var stdout, stderr bytes.Buffer
	if code := run([]string{
		"begin", "--ledger", path, "--coating-run", "batch-42", "--max-loss", "4", "--panel", "left", "--panel", "right",
	}, &stdout, &stderr); code != 0 {
		t.Fatalf("begin exit code = %d, stderr = %s", code, stderr.String())
	}
	var created trial.Trial
	if err := json.Unmarshal(stdout.Bytes(), &created); err != nil {
		t.Fatalf("decode begin output: %v", err)
	}
	if created.ID == "" {
		t.Fatal("begin output has no identifier")
	}

	stdout.Reset()
	stderr.Reset()
	if code := run([]string{
		"record", "--ledger", path, "--trial", created.ID, "--panel", "left", "--loss", "5",
	}, &stdout, &stderr); code == 0 {
		t.Fatal("over-limit record without note succeeded")
	}
	if !strings.Contains(stderr.String(), "note is required") {
		t.Fatalf("stderr = %q, want note error", stderr.String())
	}

	note := "small pull-through at the intersection"
	stdout.Reset()
	stderr.Reset()
	if code := run([]string{
		"record", "--ledger", path, "--trial", created.ID, "--panel", "left", "--loss", "5", "--note", note,
	}, &stdout, &stderr); code != 0 {
		t.Fatalf("record exit code = %d, stderr = %s", code, stderr.String())
	}
	stdout.Reset()
	if code := run([]string{
		"record", "--ledger", path, "--trial", created.ID, "--panel", "right", "--loss", "2",
	}, &stdout, &stderr); code != 0 {
		t.Fatalf("second record exit code = %d, stderr = %s", code, stderr.String())
	}

	stdout.Reset()
	if code := run([]string{"finalize", "--ledger", path, "--trial", created.ID}, &stdout, &stderr); code != 0 {
		t.Fatalf("finalize exit code = %d, stderr = %s", code, stderr.String())
	}
	var report trial.Report
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("decode finalize output: %v", err)
	}
	if report.Accepted || report.Outcome != trial.OutcomeRecoat {
		t.Fatalf("report = %#v, want recoat", report)
	}
	stdout.Reset()
	if code := run([]string{"show", "--ledger", path, "--trial", created.ID}, &stdout, &stderr); code != 0 {
		t.Fatalf("show exit code = %d, stderr = %s", code, stderr.String())
	}
	var shown trial.Report
	if err := json.Unmarshal(stdout.Bytes(), &shown); err != nil {
		t.Fatalf("decode show output: %v", err)
	}
	if shown.TrialID != created.ID || shown.Results["left"].Note == nil {
		t.Fatalf("show report = %#v", shown)
	}
}

func TestSmokeUsesTemporaryLedger(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"smoke"}, &stdout, &stderr); code != 0 {
		t.Fatalf("smoke exit code = %d, stderr = %s", code, stderr.String())
	}
	if lines := strings.Count(stdout.String(), "\n"); lines != 5 {
		t.Fatalf("smoke output lines = %d, want five workflow results", lines)
	}
}
