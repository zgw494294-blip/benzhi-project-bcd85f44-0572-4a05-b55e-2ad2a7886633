package trial

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"strings"
)

const (
	StatusActive    = "active"
	StatusFinalized = "finalized"
	OutcomeAccepted = "accepted"
	OutcomeRecoat   = "recoat"
)

var (
	ErrInvalidTrial       = errors.New("invalid trial")
	ErrTrialNotActive     = errors.New("trial is not active")
	ErrUnknownPanel       = errors.New("panel is not declared")
	ErrPanelAlreadyRecord = errors.New("panel already has a result")
	ErrIncompleteTrial    = errors.New("every declared panel needs one result")
	ErrAlreadyFinalized   = errors.New("trial is already finalized")
)

type PanelResult struct {
	LossPercent float64 `json:"loss_percent"`
	Note        *string `json:"note,omitempty"`
}

type Report struct {
	TrialID            string                 `json:"trial_id"`
	CoatingRun         string                 `json:"coating_run"`
	MaximumLossPercent float64                `json:"maximum_loss_percent"`
	Results            map[string]PanelResult `json:"results"`
	Accepted           bool                   `json:"accepted"`
	Outcome            string                 `json:"outcome"`
}

type Trial struct {
	ID                 string                 `json:"id"`
	CoatingRun         string                 `json:"coating_run"`
	MaximumLossPercent float64                `json:"maximum_loss_percent"`
	PanelLabels        []string               `json:"panel_labels"`
	Results            map[string]PanelResult `json:"results"`
	Status             string                 `json:"status"`
	Report             *Report                `json:"report,omitempty"`
}

func Begin(coatingRun string, maximumLossPercent float64, panelLabels []string) (Trial, error) {
	coatingRun = strings.TrimSpace(coatingRun)
	if coatingRun == "" {
		return Trial{}, fmt.Errorf("%w: coating run is required", ErrInvalidTrial)
	}
	if err := validatePercent(maximumLossPercent, "maximum loss percentage"); err != nil {
		return Trial{}, err
	}

	labels, err := normalizePanelLabels(panelLabels)
	if err != nil {
		return Trial{}, err
	}
	id, err := newID()
	if err != nil {
		return Trial{}, fmt.Errorf("create trial identifier: %w", err)
	}
	return Trial{
		ID:                 id,
		CoatingRun:         coatingRun,
		MaximumLossPercent: maximumLossPercent,
		PanelLabels:        labels,
		Results:            make(map[string]PanelResult),
		Status:             StatusActive,
	}, nil
}

func (t Trial) Validate() error {
	if strings.TrimSpace(t.ID) == "" {
		return fmt.Errorf("%w: trial identifier is required", ErrInvalidTrial)
	}
	if strings.TrimSpace(t.CoatingRun) == "" {
		return fmt.Errorf("%w: coating run is required", ErrInvalidTrial)
	}
	if err := validatePercent(t.MaximumLossPercent, "maximum loss percentage"); err != nil {
		return err
	}
	if _, err := normalizePanelLabels(t.PanelLabels); err != nil {
		return err
	}
	if t.Results == nil {
		if t.Status != StatusActive {
			return fmt.Errorf("%w: finalized trial has no results", ErrInvalidTrial)
		}
	} else {
		for panel, result := range t.Results {
			if !contains(t.PanelLabels, panel) {
				return fmt.Errorf("%w: result for %q", ErrInvalidTrial, panel)
			}
			if err := validatePercent(result.LossPercent, "loss percentage"); err != nil {
				return err
			}
		}
	}
	if t.Status == StatusActive {
		if t.Report != nil {
			return fmt.Errorf("%w: active trial has a report", ErrInvalidTrial)
		}
		return nil
	}
	if t.Status != StatusFinalized {
		return fmt.Errorf("%w: unsupported status %q", ErrInvalidTrial, t.Status)
	}
	if t.Report == nil {
		return fmt.Errorf("%w: finalized trial has no report", ErrInvalidTrial)
	}
	if len(t.Results) != len(t.PanelLabels) {
		return fmt.Errorf("%w: finalized trial is incomplete", ErrInvalidTrial)
	}
	for _, panel := range t.PanelLabels {
		if _, ok := t.Results[panel]; !ok {
			return fmt.Errorf("%w: finalized trial is incomplete", ErrInvalidTrial)
		}
	}
	if err := t.Report.validateFor(t); err != nil {
		return err
	}
	return nil
}

func (t *Trial) Record(panel string, lossPercent float64, note *string) error {
	if t == nil {
		return ErrInvalidTrial
	}
	if err := t.Validate(); err != nil {
		return err
	}
	if t.Status != StatusActive {
		return ErrTrialNotActive
	}
	panel = strings.TrimSpace(panel)
	if !contains(t.PanelLabels, panel) {
		return fmt.Errorf("%w: %q", ErrUnknownPanel, panel)
	}
	if _, exists := t.Results[panel]; exists {
		return fmt.Errorf("%w: %q", ErrPanelAlreadyRecord, panel)
	}
	if err := validatePercent(lossPercent, "loss percentage"); err != nil {
		return err
	}
	cleanNote := cleanNoteValue(note)
	if lossPercent > t.MaximumLossPercent && (cleanNote == nil || strings.TrimSpace(*cleanNote) == "") {
		return fmt.Errorf("%w: note is required for an over-limit result", ErrInvalidTrial)
	}
	if t.Results == nil {
		t.Results = make(map[string]PanelResult)
	}
	t.Results[panel] = PanelResult{LossPercent: lossPercent, Note: cleanNote}
	return nil
}

func (t *Trial) Finalize() (Report, error) {
	if t == nil {
		return Report{}, ErrInvalidTrial
	}
	if t.Status != StatusActive {
		return Report{}, ErrAlreadyFinalized
	}
	if err := t.Validate(); err != nil {
		return Report{}, err
	}
	for _, panel := range t.PanelLabels {
		if _, exists := t.Results[panel]; !exists {
			return Report{}, ErrIncompleteTrial
		}
	}
	accepted := true
	results := cloneResults(t.Results)
	for _, result := range results {
		if result.LossPercent > t.MaximumLossPercent {
			accepted = false
			break
		}
	}
	outcome := OutcomeRecoat
	if accepted {
		outcome = OutcomeAccepted
	}
	report := Report{
		TrialID:            t.ID,
		CoatingRun:         t.CoatingRun,
		MaximumLossPercent: t.MaximumLossPercent,
		Results:            results,
		Accepted:           accepted,
		Outcome:            outcome,
	}
	t.Status = StatusFinalized
	t.Report = &report
	return report, nil
}

func (r Report) validateFor(t Trial) error {
	if r.TrialID != t.ID || r.CoatingRun != t.CoatingRun || r.MaximumLossPercent != t.MaximumLossPercent {
		return fmt.Errorf("%w: report does not match trial", ErrInvalidTrial)
	}
	if r.Outcome != OutcomeAccepted && r.Outcome != OutcomeRecoat {
		return fmt.Errorf("%w: unsupported report outcome %q", ErrInvalidTrial, r.Outcome)
	}
	if r.Accepted != (r.Outcome == OutcomeAccepted) {
		return fmt.Errorf("%w: report outcome does not match acceptance", ErrInvalidTrial)
	}
	if len(r.Results) != len(t.Results) {
		return fmt.Errorf("%w: report results do not match trial", ErrInvalidTrial)
	}
	for panel, result := range t.Results {
		reported, ok := r.Results[panel]
		if !ok || reported.LossPercent != result.LossPercent || !sameNote(reported.Note, result.Note) {
			return fmt.Errorf("%w: report results do not match trial", ErrInvalidTrial)
		}
	}
	if r.Accepted {
		for _, result := range r.Results {
			if result.LossPercent > r.MaximumLossPercent {
				return fmt.Errorf("%w: accepted report has an over-limit result", ErrInvalidTrial)
			}
		}
	}
	return nil
}

func validatePercent(value float64, label string) error {
	if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > 100 {
		return fmt.Errorf("%w: %s must be between 0 and 100", ErrInvalidTrial, label)
	}
	return nil
}

func normalizePanelLabels(panelLabels []string) ([]string, error) {
	if len(panelLabels) == 0 {
		return nil, fmt.Errorf("%w: at least one panel is required", ErrInvalidTrial)
	}
	labels := make([]string, 0, len(panelLabels))
	seen := make(map[string]struct{}, len(panelLabels))
	for _, raw := range panelLabels {
		label := strings.TrimSpace(raw)
		if label == "" {
			return nil, fmt.Errorf("%w: panel labels must be nonempty", ErrInvalidTrial)
		}
		if _, exists := seen[label]; exists {
			return nil, fmt.Errorf("%w: duplicate panel %q", ErrInvalidTrial, label)
		}
		seen[label] = struct{}{}
		labels = append(labels, label)
	}
	return labels, nil
}

func cleanNoteValue(note *string) *string {
	if note == nil {
		return nil
	}
	clean := strings.TrimSpace(*note)
	if clean == "" {
		return nil
	}
	return &clean
}

func cloneResults(results map[string]PanelResult) map[string]PanelResult {
	clone := make(map[string]PanelResult, len(results))
	for panel, result := range results {
		clone[panel] = PanelResult{LossPercent: result.LossPercent, Note: cloneNote(result.Note)}
	}
	return clone
}

func cloneNote(note *string) *string {
	if note == nil {
		return nil
	}
	clone := *note
	return &clone
}

func sameNote(left, right *string) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func contains(labels []string, wanted string) bool {
	for _, label := range labels {
		if label == wanted {
			return true
		}
	}
	return false
}

func newID() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return "trial-" + hex.EncodeToString(bytes), nil
}
