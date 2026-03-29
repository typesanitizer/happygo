---
name: luci-test-history
description: Query Go’s LUCI Analysis API for test history and variant/builder breakdowns.
---

# LUCI Test History

Query upstream Go’s LUCI CI for test history and variant information.

## API Endpoint

Base URL: `https://analysis.api.luci.app/prpc/luci.analysis.v1.TestHistory/`

## Available Methods

### Query - Get test verdicts
```bash
curl -s "https://analysis.api.luci.app/prpc/luci.analysis.v1.TestHistory/Query" \
  -H "Content-Type: application/json" \
  -H "Accept: application/json" \
  -d '{"project":"golang","testId":"<TEST_ID>","predicate":{},"pageSize":100}'
```

Response contains `verdicts[]` with:
- `testId`, `variantHash`, `invocationId`
- `statusV2`: PASSED, FAILED, SKIPPED, etc.
- `partitionTime`: timestamp

### QueryVariants - Get test variants (builders/platforms)
```bash
curl -s "https://analysis.api.luci.app/prpc/luci.analysis.v1.TestHistory/QueryVariants" \
  -H "Content-Type: application/json" \
  -H "Accept: application/json" \
  -d '{"project":"golang","testId":"<TEST_ID>"}'
```

Response contains `variants[]` with:
- `variantHash`
- `variant.def`: object with `builder`, `goos`, `goarch`, `go_branch`, etc.

### QueryTests - Search for test IDs
```bash
curl -s "https://analysis.api.luci.app/prpc/luci.analysis.v1.TestHistory/QueryTests" \
  -H "Content-Type: application/json" \
  -H "Accept: application/json" \
  -d '{"project":"golang","testIdSubstring":"<SUBSTRING>"}'
```

## Example: Check if test runs on Windows

```bash
# 1. Get Windows variant hashes
curl -s ".../QueryVariants" -d '{"project":"golang","testId":"cmd/go.TestScript/list_swigcxx"}' | \
  jq -r '.variants[] | select(.variant.def.goos == "windows") | .variantHash'

# 2. Check status for those variants
curl -s ".../Query" -d '{"project":"golang","testId":"cmd/go.TestScript/list_swigcxx","pageSize":100}' | \
  jq -r '.verdicts[] | select(.variantHash == "<HASH>") | .statusV2'
```

## Notes

- Response is prefixed with `)]}'` - strip with `sed 's/^)]}'\''//'`
- Project for Go is always `"golang"`
- Test IDs use format like `cmd/go.TestScript/test_name` or `pkg/path.TestName`
