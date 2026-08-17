package ledger

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"syscall"

	"gridbond/internal/trial"
)

const CurrentVersion = 1

var ErrUnsupportedVersion = errors.New("unsupported ledger version")

type Ledger struct {
	Version  int           `json:"version"`
	Trials   []trial.Trial `json:"trials"`
	baseline []byte
}

func New() Ledger {
	return Ledger{
		Version: CurrentVersion,
		Trials:  []trial.Trial{},
	}
}

func Load(path string) (Ledger, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		empty := New()
		empty.baseline = []byte(`{"version":1,"trials":[]}`)
		return empty, nil
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
	loaded.baseline = append([]byte(nil), data...)
	return loaded, nil
}

func Save(path string, loaded Ledger) error {
	if err := loaded.Validate(); err != nil {
		return fmt.Errorf("validate ledger: %w", err)
	}
	directory := filepath.Dir(path)
	base := filepath.Base(path)
	lock, err := os.OpenFile(filepath.Join(directory, "."+base+".lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return fmt.Errorf("open ledger lock: %w", err)
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("lock ledger: %w", err)
	}

	if loaded.baseline != nil {
		current, err := Load(path)
		if err != nil {
			return fmt.Errorf("load current ledger: %w", err)
		}
		loaded, err = mergeChanges(loaded, current)
		if err != nil {
			return err
		}
	}

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

func mergeChanges(loaded, current Ledger) (Ledger, error) {
	var baseline Ledger
	if err := json.Unmarshal(loaded.baseline, &baseline); err != nil {
		return Ledger{}, fmt.Errorf("decode ledger baseline: %w", err)
	}

	baseTrials := indexTrials(baseline.Trials)
	loadedTrials := indexTrials(loaded.Trials)
	currentTrials := indexTrials(current.Trials)
	merged := Ledger{Version: CurrentVersion, Trials: make([]trial.Trial, 0, len(current.Trials)+len(loaded.Trials))}

	for _, persisted := range current.Trials {
		original, existed := baseTrials[persisted.ID]
		desired, retained := loadedTrials[persisted.ID]
		if !existed {
			if retained && !reflect.DeepEqual(desired, persisted) {
				return Ledger{}, fmt.Errorf("trial %q was added concurrently", persisted.ID)
			}
			merged.Trials = append(merged.Trials, persisted)
			continue
		}
		if !retained {
			if !reflect.DeepEqual(persisted, original) {
				return Ledger{}, fmt.Errorf("trial %q changed concurrently", persisted.ID)
			}
			continue
		}
		desiredChanged := !reflect.DeepEqual(desired, original)
		persistedChanged := !reflect.DeepEqual(persisted, original)
		if desiredChanged && persistedChanged && !reflect.DeepEqual(desired, persisted) {
			return Ledger{}, fmt.Errorf("trial %q changed concurrently", persisted.ID)
		}
		if desiredChanged {
			merged.Trials = append(merged.Trials, desired)
		} else {
			merged.Trials = append(merged.Trials, persisted)
		}
	}

	for _, desired := range loaded.Trials {
		if _, exists := currentTrials[desired.ID]; exists {
			continue
		}
		if original, existed := baseTrials[desired.ID]; existed {
			if !reflect.DeepEqual(desired, original) {
				return Ledger{}, fmt.Errorf("trial %q was removed concurrently", desired.ID)
			}
			continue
		}
		merged.Trials = append(merged.Trials, desired)
	}

	if err := merged.Validate(); err != nil {
		return Ledger{}, fmt.Errorf("validate merged ledger: %w", err)
	}
	return merged, nil
}

func indexTrials(trials []trial.Trial) map[string]trial.Trial {
	indexed := make(map[string]trial.Trial, len(trials))
	for _, current := range trials {
		indexed[current.ID] = current
	}
	return indexed
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
