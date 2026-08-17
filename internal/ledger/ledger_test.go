package ledger

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"gridbond/internal/trial"
)

func TestLoadMissingLedgerReturnsEmptyVersionedLedger(t *testing.T) {
	loaded, err := Load(filepath.Join(t.TempDir(), "ledger.json"))
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Version != CurrentVersion || len(loaded.Trials) != 0 {
		t.Fatalf("loaded = %#v, want empty current ledger", loaded)
	}
}

func TestLoadPropagatesMalformedAndUnsupportedFiles(t *testing.T) {
	directory := t.TempDir()
	malformed := filepath.Join(directory, "malformed.json")
	if err := os.WriteFile(malformed, []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(malformed); err == nil {
		t.Fatal("Load malformed file returned nil error")
	}
	unsupported := filepath.Join(directory, "unsupported.json")
	if err := os.WriteFile(unsupported, []byte(`{"version":99,"trials":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(unsupported); !errors.Is(err, ErrUnsupportedVersion) {
		t.Fatalf("Load unsupported file error = %v, want unsupported version", err)
	}
}

func TestSaveAndLoadRoundTrip(t *testing.T) {
	current, err := trial.Begin("run-1", 5, []string{"panel-a"})
	if err != nil {
		t.Fatal(err)
	}
	if err := current.Record("panel-a", 1, nil); err != nil {
		t.Fatal(err)
	}
	loaded := New()
	if err := loaded.Add(current); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "nested", "ledger.json")
	if err := Save(path, loaded); err == nil {
		t.Fatal("Save to missing parent returned nil error")
	}
	path = filepath.Join(t.TempDir(), "ledger.json")
	if err := Save(path, loaded); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Trials) != 1 || got.Trials[0].ID != current.ID || got.Trials[0].Results["panel-a"].LossPercent != 1 {
		t.Fatalf("round trip = %#v", got)
	}
}
