# Flat Syntax Tree Parser Design

## Goals

- Parse Go source faster than the existing parser on steady-state workloads.
- Minimize allocations by storing syntax in flat arenas instead of pointer-rich trees.
- Preserve full trivia so source can round-trip exactly and comments can be reattached later.
- Match the existing parser's pass/fail behavior across the `go/` tree and the standard library.
- Support an optimization loop driven by parity checks, fuzzing, and benchmarks.

## Non-goals

- Reuse `go/ast` or the compiler's current syntax tree as the primary representation.
- Encode semantic comment attachment directly into storage.
- Depend on pretty-printing or formatting as the compatibility oracle.

## Core Representation

The parser stores lexical and structural data separately. Tokens exclude trivia.
Trivia is owned by gaps between tokens.

```
+------------+------------------------------------------+--------------------------------------+
| Arena      | Contents                                 | Notes                                |
+------------+------------------------------------------+--------------------------------------+
| Nodes      | node headers                             | Nodes[0] invalid; Nodes[1] is root   |
| Edges      | child node ids                           | Children are stored contiguously     |
| Tokens     | non-trivia tokens with byte spans        | Exact source spans, no decoded text  |
| Gaps       | trivia ranges before/between/after tokens| len(Gaps) == len(Tokens) + 1         |
| Trivia     | comments, whitespace, directives, etc.   | Full-fidelity lexical storage        |
| Directives | indexes into Trivia for `//go:`/`//line` | Semantic lookup side table           |
+------------+------------------------------------------+--------------------------------------+
```

Manual field layout matters because Go does not reorder struct fields. The hot
types should be packed deliberately and measured with `unsafe.Sizeof` in tests.

## Why Gap-Based Trivia

Gap-based ownership keeps lexical storage unambiguous:

- `Gaps[0]` stores trivia before the first token.
- `Gaps[i]` stores trivia between `Tokens[i-1]` and `Tokens[i]`.
- `Gaps[len(Tokens)]` stores trivia after the last token.

This avoids arbitrary "leading vs trailing trivia owner" decisions while still
supporting derived views such as:

- leading trivia for token `i`
- trailing trivia for token `i`
- gap trivia before or after a syntax node

Those derived views are convenience APIs. The stored truth is still the gap map.

## Parsing Pipeline

1. Scan source once into `Tokens`, `Trivia`, `Gaps`, and `LineStarts`.
2. Parse only the non-trivia token stream into `Nodes` and `Edges`.
3. Record token ranges on nodes so subtree text and trivia boundaries are cheap.
4. Build side indexes for directives and comment attachment.
5. Lower to the existing syntax representation only when compatibility checks need it.

Literal handling is source-faithful:

- raw strings keep backticks and embedded newlines in the token span
- interpreted strings and runes keep quotes and escapes in the token span
- invalid escapes or unterminated literals produce errors without losing source spans

The tree does not eagerly decode literal contents. Decoding is a separate helper.

## Semantic Comment Attachment

Lexical ownership and semantic ownership are different, especially in Go.

```go
type S struct {
	// doc for X
	X int // line comment for X
}
```

Both comments belong to field `X` semantically, but they live in different gaps
lexically. The design therefore keeps comment meaning in a separate pass:

- doc comments attach to the following declaration/spec/field
- end-of-line comments attach to the preceding declaration/spec/field
- detached groups stay detached
- directives are indexed explicitly because they affect semantics

This attachment pass is also the compatibility bridge to the current Go APIs.

## Compatibility Strategy

Compatibility is checked structurally, not by formatting text.

1. Parse each file with the old parser and the new parser.
2. Assert old error implies new error.
3. Assert old success implies new success.
4. Lower the flat syntax tree into the existing syntax representation.
5. Compare selected files structurally, including comment attachment expectations.

The lowering adapter is expected, not accidental. It gives us:

- direct parity checks against existing parser behavior
- stable diffs for debugging
- a path for incremental adoption before the flat tree has first-class consumers

## Benchmarks and Fuzzing

Optimization comes after parity.

- Add scanner-only benchmarks.
- Add full-parse benchmarks.
- Measure the compatibility adapter separately so parser wins are not hidden.
- Seed fuzzing from existing short parser tests plus a curated stdlib corpus.
- Keep a repro corpus for minimized failures.

The first benchmark bar is simple: the new parser must be faster than the old
parser on representative files without regressing correctness.

## Agent Loop Harness

The optimization loop should treat parser work as a hill-climbing problem:

- input: design doc, current repo state, and validation commands
- objective: parity tests, fuzzing, and benchmarks
- agents: Claude and Codex as optional workers
- scheduler: retry on rate limits, rotate agents, and keep iteration logs
- entrypoint: `meta agent-loop`
- sandbox: copy the repo into a Podman container, run with `--network none` by
  default, and only enable API egress through an allowlisting proxy when needed

Pure container networking is not sufficient to allowlist just OpenAI and
Anthropic domains. The loop should therefore support a proxy-based model for
"API-only" egress instead of claiming domain-level isolation it cannot enforce
by itself.

## Open Questions

- Final `Node` and `Token` field layout after size measurements.
- Whether `Gaps` should store `(offset,len)` pairs or a prefix-sum index into `Trivia`.
- Exact compatibility surface for comments and directives in the lowering adapter.
- How much semantic attachment should be precomputed versus derived lazily.
