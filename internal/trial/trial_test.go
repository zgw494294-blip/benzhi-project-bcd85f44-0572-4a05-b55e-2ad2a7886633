package trial

import (
	"errors"
	"testing"
)

func TestBeginRejectsEmptyAndDuplicatePanels(t *testing.T) {
	for _, panels := range [][]string{{}, {"A", " "}, {"A", "A"}, {"A", " A "}} {
		if _, err := Begin("run-1", 5, panels); !errors.Is(err, ErrInvalidTrial) {
			t.Fatalf("Begin(%v) error = %v, want invalid trial", panels, err)
		}
	}
}

func TestRecordRequiresNoteForOverLimitResult(t *testing.T) {
	trial, err := Begin("run-1", 5, []string{"panel-a", "panel-b"})
	if err != nil {
		t.Fatal(err)
	}
	if err := trial.Record("panel-a", 6, nil); !errors.Is(err, ErrInvalidTrial) {
		t.Fatalf("over-limit record error = %v, want invalid trial", err)
	}
	if len(trial.Results) != 0 {
		t.Fatalf("failed record changed results: %#v", trial.Results)
	}
	note := "visible coating loss"
	if err := trial.Record("panel-a", 6, &note); err != nil {
		t.Fatal(err)
	}
	if trial.Results["panel-a"].Note == nil || *trial.Results["panel-a"].Note != note {
		t.Fatalf("stored note = %#v, want %q", trial.Results["panel-a"].Note, note)
	}
}

func TestFinalizeAcceptedReportIsImmutableAndOnlyHappensOnce(t *testing.T) {
	trial, err := Begin("run-1", 5, []string{"panel-a", "panel-b"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := trial.Finalize(); !errors.Is(err, ErrIncompleteTrial) {
		t.Fatalf("incomplete finalize error = %v, want incomplete trial", err)
	}
	if trial.Status != StatusActive || trial.Report != nil {
		t.Fatalf("incomplete finalize changed trial: %#v", trial)
	}
	if err := trial.Record("panel-a", 5, nil); err != nil {
		t.Fatal(err)
	}
	if err := trial.Record("panel-b", 2, nil); err != nil {
		t.Fatal(err)
	}
	report, err := trial.Finalize()
	if err != nil {
		t.Fatal(err)
	}
	if !report.Accepted || report.Outcome != OutcomeAccepted {
		t.Fatalf("report = %#v, want accepted", report)
	}
	trial.Results["panel-a"] = PanelResult{LossPercent: 99}
	if report.Results["panel-a"].LossPercent != 5 {
		t.Fatalf("report changed after trial mutation: %#v", report.Results)
	}
	if _, err := trial.Finalize(); !errors.Is(err, ErrAlreadyFinalized) {
		t.Fatalf("repeated finalize error = %v, want already finalized", err)
	}
}

func TestFinalizeProducesRecoatReport(t *testing.T) {
	trial, err := Begin("run-1", 5, []string{"panel-a"})
	if err != nil {
		t.Fatal(err)
	}
	note := "loss at cut intersections"
	if err := trial.Record("panel-a", 6, &note); err != nil {
		t.Fatal(err)
	}
	report, err := trial.Finalize()
	if err != nil {
		t.Fatal(err)
	}
	if report.Accepted || report.Outcome != OutcomeRecoat {
		t.Fatalf("report = %#v, want recoat", report)
	}
}
