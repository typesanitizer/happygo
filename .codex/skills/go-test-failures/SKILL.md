---
name: go-test-failures
description: Run Go tests with -json and jq to show only failing tests or failing-test output, with cache disabled (-count=1) and support for any Go package pattern.
---

# Go Test Failures

## Use the list-only command

Run Go tests with JSON output and print only failing test names.

```bash
go test -json -count=1 -timeout=5m <pkgs> \
  | jq -r 'select(.Action=="fail" and .Test!=null) | .Test' \
  | sort -u
```

## Use the verbose command

Print each failing test name followed by its output.

```bash
go test -json -count=1 -timeout=5m <pkgs> \
  | jq -rs '
      [ .[] | select(.Action=="fail" and .Test!=null) | .Test ]
      | unique as $fails
      | $fails[] as $t
      | "FAIL: \($t)\n" + (
          [ .[] | select(.Test==$t and .Action=="output") | .Output ]
          | join("")
        )
    '
```

## Optional: list failing packages (no test name)

Some failures are package-level. List those packages separately.

```bash
go test -json -count=1 -timeout=5m <pkgs> \
  | jq -r 'select(.Action=="fail" and .Test==null) | .Package' \
  | sort -u
```

## Notes

- Replace `<pkgs>` with any Go package pattern: `./...`, `./pkg/foo`, `std`, `cmd/...`, or module paths.
- Add extra `go test` flags before `<pkgs>` (for example: `-run`, `-race`, `-short`, `-vet=off`).
- These pipelines require `jq`.
- The verbose command uses `jq -s` to slurp all JSON events; this is simplest but can use more memory on very large test runs.
