package ledger

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gridbond/internal/trial"
)

const CurrentVersion = 1

var ErrUnsupportedVersion = errors.New("unsupported ledger version")

type Ledger struct {
	Version int           `json:"version"`
	Trials  []trial.Trial `json:"trials"`
}

func New() Ledger {
	return Ledger{Version: CurrentVersion, Trials: []trial.Trial{}}
}

func Load(path string) (Ledger, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return New(), nil
	}
	if err != nil {
		return Ledger{}, fmt.Errorf("read ledger: %w", err)
	}
	var loaded Ledger
	if err := json.Unmarshal(data, &loaded); err != nil {
		return Ledger{}, fmt.Errorf("decode ledger: %w", err)
	}
	if loaded.Version != CurrentVersion {
		return Ledger{}, fmt.Errorf("%w: %d", ErrUnsupportedVersion, loaded.Version)
	}
	if loaded.Trials == nil {
		loaded.Trials = []trial.Trial{}
	}
	if err := loaded.Validate(); err != nil {
		return Ledger{}, fmt.Errorf("validate ledger: %w", err)
	}
	return loaded, nil
}

func Save(path string, loaded Ledger) error {
	if err := loaded.Validate(); err != nil {
		return fmt.Errorf("validate ledger: %w", err)
	}
	directory := filepath.Dir(path)
	base := filepath.Base(path)
	temporary, err := os.CreateTemp(directory, "."+base+".tmp-")
	if err != nil {
		return fmt.Errorf("create ledger temporary file: %w", err)
	}
	temporaryName := temporary.Name()
	keepTemporary := true
	defer func() {
		if keepTemporary {
			_ = os.Remove(temporaryName)
		}
	}()

	encoder := json.NewEncoder(temporary)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(loaded); err != nil {
		return fmt.Errorf("write ledger: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		return fmt.Errorf("sync ledger: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close ledger: %w", err)
	}
	if err := os.Rename(temporaryName, path); err != nil {
		return fmt.Errorf("replace ledger: %w", err)
	}
	keepTemporary = false
	return nil
}

func (l Ledger) Validate() error {
	if l.Version != CurrentVersion {
		return fmt.Errorf("%w: %d", ErrUnsupportedVersion, l.Version)
	}
	seen := make(map[string]struct{}, len(l.Trials))
	for _, current := range l.Trials {
		if err := current.Validate(); err != nil {
			return err
		}
		if _, exists := seen[current.ID]; exists {
			return fmt.Errorf("duplicate trial identifier %q", current.ID)
		}
		seen[current.ID] = struct{}{}
	}
	return nil
}

func (l *Ledger) Add(current trial.Trial) error {
	if l == nil {
		return errors.New("nil ledger")
	}
	if err := current.Validate(); err != nil {
		return err
	}
	if _, ok := l.Find(current.ID); ok {
		return fmt.Errorf("trial %q already exists", current.ID)
	}
	l.Trials = append(l.Trials, current)
	return nil
}

func (l *Ledger) Find(id string) (*trial.Trial, bool) {
	if l == nil {
		return nil, false
	}
	for index := range l.Trials {
		if l.Trials[index].ID == id {
			return &l.Trials[index], true
		}
	}
	return nil, false
}
