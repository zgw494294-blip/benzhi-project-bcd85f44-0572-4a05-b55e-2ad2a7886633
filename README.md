# GridBond

GridBond is a small Go command-line tool for recording cross-hatch tape-pull adhesion trials on coated panels. It keeps one versioned JSON ledger on disk and turns each completed trial into an accepted-or-recoat report.

## Requirements

- Go 1.22.0 or newer

## Commands

Start a trial with one or more distinct panel labels:

```text
go run ./cmd/gridbond begin --ledger gridbond.json --coating-run batch-42 --max-loss 5 --panel left --panel right
```

Record one result per panel. A result above the configured maximum requires a note:

```text
go run ./cmd/gridbond record --ledger gridbond.json --trial TRIAL_ID --panel left --loss 2
go run ./cmd/gridbond record --ledger gridbond.json --trial TRIAL_ID --panel right --loss 7 --note "loss at the cut intersections"
```

Finalize after every declared panel has one result, then inspect the immutable report:

```text
go run ./cmd/gridbond finalize --ledger gridbond.json --trial TRIAL_ID
go run ./cmd/gridbond show --ledger gridbond.json --trial TRIAL_ID
```

The `smoke` command runs a complete bounded workflow against a temporary ledger:

```text
go run ./cmd/gridbond smoke
```

The ledger is replaced atomically after each successful write. A missing ledger starts empty; malformed or unsupported existing data is reported as an operational error.

## Verification

```text
go test ./...
```
