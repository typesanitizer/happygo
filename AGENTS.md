# Notes for AI Agents

## Code Style

Follow `docs/style-guides/go.md` for Go code conventions (import ordering, etc.).

## Commit Hygiene for PRs

When making fixes to code that was introduced in a PR:

1. Use fixup commits: Create commits with `git commit --fixup=<target-sha>`
   to associate the fix with the original commit that introduced the issue.
   If [git-absorb](github.com/tummychow/git-absorb) is installed, using that
   can be faster for making fixup commits.

2. Autosquash before pushing: Run `GIT_SEQUENCE_EDITOR=: git rebase -i --autosquash <base>`
   to squash fixup commits into their targets.

3. Keep history clean: The goal is a clean, logical commit history where
   each commit represents a coherent change, not a sequence of "fix typo" or
   "oops forgot this" commits.

This approach maintains bisectability and makes code review easier by keeping
related changes together.

## Temporary Files

Store temporary files (downloads, logs, caches) in `.cache/tmp/`.
Do not put files in `/tmp`.

## Investigating Issues

When investigating CI failures or bugs:

1. Analyze before proposing fixes: Carefully analyze the issue first, grounding
   observations in logs and code. Do not jump to fixes without understanding the
   root cause.

2. Record logs for search: Instead of repeatedly querying the API, download
   the log to a file under `.cache/<date>-<topic>/` and run commands against it.

3. Document findings: Document hypotheses and observations in 
   `.cache/<date>-<topic>/NOTES.md`.

4. Ask for approval: Before implementing a fix, present your analysis and
   proposed solution for approval.

## SYNC Comments

When a diff touches code near a `SYNC(id: ...)` comment, check all matching
`SYNC(id: ...)` sites and flag if they are out of sync.

## CI Links

Never add links to GitHub Actions runs in notes or comments; the logs are only
retained for 90 days. Instead, inline the relevant context concisely, if needed.
