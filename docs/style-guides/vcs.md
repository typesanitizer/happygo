# Version control style guide

## Commit hygiene

Keep history clean. Generally, a PR should only have 1 commit.
The goal is a clean, logical commit history where each commit
represents a coherent change.

This approach maintains bisectability and makes code review easier
by keeping related changes together.

## Commit messages

We use a loose form of [conventional commits](https://www.conventionalcommits.org/en/v1.0.0/).

```text
<type>[optional scope]: <title>

<description>

[optional footer(s)]
```

Reasonable values for `<type>` include `feat`, `fix`, `ci`, `docs`,
`test`, `perf`, and `chore`.

Avoid introducing extra types such as `style` or `refactor`.
Use `chore` for maintenance work and as a fallback.

Write the commit title in sentence case, and do not end the title with a period.

For the description, keep it brief but do not omit it entirely.

There is generally no need to mark breaking changes outside public APIs
in the root `go/` folder and public tools such as `gopls`.
If you do need to mark one, use footer syntax:

```text
BREAKING CHANGE: <description>
```

Hard-wrap commit messages to a reasonable width (for example, 80-100 chars).
URLs, SHAs, and auto-generated messages may exceed that width.

## PR descriptions

Keep PR descriptions concise and focused on what changed and why.

Use extra structure only when needed for longer descriptions.

Avoid exhaustive change lists in PR descriptions.
Reviewers can inspect the diff and commit history for details.

## Test descriptions

Avoid superfluous statements such as "Added unit tests" or
"Covered by existing tests."

## Flake fixes

When fixing a flaky test, temporarily adjust CI to:

- Run only jobs needed for the flaky test
- Run the flaky test at least 100 times

After confirming stability, undo temporary pipeline adjustments in a separate commit.
