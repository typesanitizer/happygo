# Go style guide

This guide applies to non-forked folders only (for example, `common/`, `meta/`, and `.github/`-adjacent helper code).

## Imports

- Group order: stdlib, third-party, monorepo (`github.com/typesanitizer/happygo/...`).
- `github.com/typesanitizer/happygo/common/core` should be imported with `.`.

## Naming

### Avoid negation in names

```go
type ABC struct {
    xyzDisabled bool // ❌
}

func HasNoPathSeparators(p string) bool // ❌
```

Use positive naming, letting the caller handle negation.

```go
type ABC struct {
    xyzEnabled bool // ✔️
}

func HasPathSeparators(p string) bool // ✔️
```

**Q**: Won't this lead to potential bugs with default initialization
or higher verbosity when the more common desired setting is `true`?

**A**: If that kind of default initialization is desired, expose a helper
function. Higher verbosity is not a reason to reduce readability.
Overall the risk of confusing code due to double negations is higher.

## 'Type-unique' arguments bundled as first parameter

Examples:

- `context.Context`.
- `*logx.Logger`.
- `logx.LogCtx` (bundles `Logger` and `context.Context` together).

If you need to create more bundles, define a dedicated type by
embedding the relevant dependencies and pass that around, instead
of repeating several arguments at multiple sites in a call chain.

## Enum-like constants use `Type_Value` naming

```go
type ListProvenance int

const (
	ListProvenance_All        ListProvenance = iota + 1
	ListProvenance_FirstParty
	ListProvenance_Forked
)
```

Start at `iota + 1` so that the zero value is distinct from all valid cases.

## Optional customization points should go in a dedicated Options struct

```go
func RunThing(ctx logx.LogCtx, target string, options RunThingOptions) error
```

Bundling customization points allows passing the value through multiple
functions with less repetition, and provides a natural documentation point
(field definition) for the semantics of each options, instead of having
to repeat that at every function.

Related: <https://matklad.github.io/2026/02/11/programming-aphorisms.html>

Generally, the fields of an `Options` type will fall into one of 3 cases:
- The zero value for the field is a sensible default.
- Have type `common/core.Option`,
- The field is initialized to a sensible default by the matching
  `func New*Options` constructor.
