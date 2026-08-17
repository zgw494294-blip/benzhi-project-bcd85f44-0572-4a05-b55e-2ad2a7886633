package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gridbond/internal/ledger"
	"gridbond/internal/trial"
)

const defaultLedgerPath = "gridbond.json"

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		printUsage(stdout)
		return 0
	}
	switch args[0] {
	case "begin":
		return runBegin(args[1:], stdout, stderr)
	case "record":
		return runRecord(args[1:], stdout, stderr)
	case "finalize":
		return runFinalize(args[1:], stdout, stderr)
	case "show":
		return runShow(args[1:], stdout, stderr)
	case "smoke":
		return runSmoke(stdout, stderr)
	default:
		return fail(stderr, "unknown command %q", args[0])
	}
}

func runBegin(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("begin", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	path := flags.String("ledger", defaultLedgerPath, "ledger file")
	coatingRun := flags.String("coating-run", "", "coating-run label")
	maximumLoss := floatOption{}
	flags.Var(&maximumLoss, "max-loss", "maximum acceptable loss percentage")
	panels := stringList{}
	flags.Var(&panels, "panel", "panel label; repeat for each panel")
	if err := flags.Parse(args); err != nil {
		return fail(stderr, "begin options: %v", err)
	}
	if !maximumLoss.set {
		return fail(stderr, "begin requires --max-loss")
	}
	current, err := trial.Begin(*coatingRun, maximumLoss.value, panels)
	if err != nil {
		return fail(stderr, "%v", err)
	}
	loaded, err := ledger.Load(*path)
	if err != nil {
		return fail(stderr, "%v", err)
	}
	if err := loaded.Add(current); err != nil {
		return fail(stderr, "%v", err)
	}
	if err := ledger.Save(*path, loaded); err != nil {
		return fail(stderr, "%v", err)
	}
	return writeJSON(stdout, current, stderr)
}

func runRecord(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("record", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	path := flags.String("ledger", defaultLedgerPath, "ledger file")
	trialID := flags.String("trial", "", "trial identifier")
	panel := flags.String("panel", "", "panel label")
	loss := floatOption{}
	flags.Var(&loss, "loss", "tape-pull loss percentage")
	note := optionalString{}
	flags.Var(&note, "note", "explanation for an over-limit result")
	if err := flags.Parse(args); err != nil {
		return fail(stderr, "record options: %v", err)
	}
	if !loss.set {
		return fail(stderr, "record requires --loss")
	}
	loaded, err := ledger.Load(*path)
	if err != nil {
		return fail(stderr, "%v", err)
	}
	current, ok := loaded.Find(strings.TrimSpace(*trialID))
	if !ok {
		return fail(stderr, "trial %q was not found", *trialID)
	}
	var suppliedNote *string
	if note.set {
		suppliedNote = &note.value
	}
	if err := current.Record(*panel, loss.value, suppliedNote); err != nil {
		return fail(stderr, "%v", err)
	}
	if err := ledger.Save(*path, loaded); err != nil {
		return fail(stderr, "%v", err)
	}
	return writeJSON(stdout, current, stderr)
}

func runFinalize(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("finalize", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	path := flags.String("ledger", defaultLedgerPath, "ledger file")
	trialID := flags.String("trial", "", "trial identifier")
	if err := flags.Parse(args); err != nil {
		return fail(stderr, "finalize options: %v", err)
	}
	loaded, err := ledger.Load(*path)
	if err != nil {
		return fail(stderr, "%v", err)
	}
	current, ok := loaded.Find(strings.TrimSpace(*trialID))
	if !ok {
		return fail(stderr, "trial %q was not found", *trialID)
	}
	report, err := current.Finalize()
	if err != nil {
		return fail(stderr, "%v", err)
	}
	if err := ledger.Save(*path, loaded); err != nil {
		return fail(stderr, "%v", err)
	}
	return writeJSON(stdout, report, stderr)
}

func runShow(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("show", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	path := flags.String("ledger", defaultLedgerPath, "ledger file")
	trialID := flags.String("trial", "", "trial identifier")
	if err := flags.Parse(args); err != nil {
		return fail(stderr, "show options: %v", err)
	}
	loaded, err := ledger.Load(*path)
	if err != nil {
		return fail(stderr, "%v", err)
	}
	current, ok := loaded.Find(strings.TrimSpace(*trialID))
	if !ok {
		return fail(stderr, "trial %q was not found", *trialID)
	}
	if current.Report != nil {
		return writeJSON(stdout, *current.Report, stderr)
	}
	return writeJSON(stdout, *current, stderr)
}

func runSmoke(stdout, stderr io.Writer) int {
	directory, err := os.MkdirTemp("", "gridbond-smoke-")
	if err != nil {
		return fail(stderr, "create smoke workspace: %v", err)
	}
	defer os.RemoveAll(directory)
	path := filepath.Join(directory, "ledger.json")

	beginOutput := bytes.Buffer{}
	if code := run([]string{
		"begin", "--ledger", path, "--coating-run", "smoke-run", "--max-loss", "5", "--panel", "panel-a", "--panel", "panel-b",
	}, &beginOutput, stderr); code != 0 {
		return code
	}
	var created trial.Trial
	if err := json.Unmarshal(beginOutput.Bytes(), &created); err != nil {
		return fail(stderr, "read smoke begin result: %v", err)
	}
	if created.ID == "" {
		return fail(stderr, "smoke begin result has no trial identifier")
	}
	if _, err := io.Copy(stdout, &beginOutput); err != nil {
		return fail(stderr, "write smoke begin result: %v", err)
	}

	if code := run([]string{"record", "--ledger", path, "--trial", created.ID, "--panel", "panel-a", "--loss", "1"}, stdout, stderr); code != 0 {
		return code
	}
	if code := run([]string{"record", "--ledger", path, "--trial", created.ID, "--panel", "panel-b", "--loss", "3"}, stdout, stderr); code != 0 {
		return code
	}
	if code := run([]string{"finalize", "--ledger", path, "--trial", created.ID}, stdout, stderr); code != 0 {
		return code
	}
	return run([]string{"show", "--ledger", path, "--trial", created.ID}, stdout, stderr)
}

func writeJSON(stdout io.Writer, value any, stderr io.Writer) int {
	if err := json.NewEncoder(stdout).Encode(value); err != nil {
		return fail(stderr, "write JSON result: %v", err)
	}
	return 0
}

func fail(stderr io.Writer, format string, args ...any) int {
	_, _ = fmt.Fprintf(stderr, "gridbond: "+format+"\n", args...)
	return 1
}

func printUsage(output io.Writer) {
	_, _ = fmt.Fprintln(output, "GridBond records cross-hatch tape-pull adhesion trials.")
	_, _ = fmt.Fprintln(output, "")
	_, _ = fmt.Fprintln(output, "Commands:")
	_, _ = fmt.Fprintln(output, "  begin     start a trial with one or more labeled panels")
	_, _ = fmt.Fprintln(output, "  record    add one tape-pull loss result to a trial")
	_, _ = fmt.Fprintln(output, "  finalize  create the accepted-or-recoat report")
	_, _ = fmt.Fprintln(output, "  show      inspect a trial or finalized report")
	_, _ = fmt.Fprintln(output, "  smoke     run a bounded complete workflow in a temporary ledger")
	_, _ = fmt.Fprintln(output, "")
	_, _ = fmt.Fprintln(output, "Use --ledger PATH on commands that read or write a ledger. The default is gridbond.json.")
}

type stringList []string

func (s *stringList) String() string {
	return strings.Join(*s, ",")
}

func (s *stringList) Set(value string) error {
	*s = append(*s, value)
	return nil
}

type floatOption struct {
	value float64
	set   bool
}

func (f *floatOption) String() string {
	return fmt.Sprintf("%v", f.value)
}

func (f *floatOption) Set(value string) error {
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return err
	}
	f.value = parsed
	f.set = true
	return nil
}

type optionalString struct {
	value string
	set   bool
}

func (s *optionalString) String() string {
	return s.value
}

func (s *optionalString) Set(value string) error {
	s.value = value
	s.set = true
	return nil
}
