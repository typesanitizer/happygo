# Notes for AI Agents

This project is a monorepo with several sub-projects,
such as forks of golang/go, golang/tools, go-delve/delve
as well as first-party code.

See [development docs](docs/DEVELOPMENT.md) for general guidance.

## Personal workflows

See docs/agent-guidance/github_username.md for information
about developers' preferred workflows.

## Version control

If `.jj/` exists, prefer using jj for version control; avoid git.

Follow `docs/style-guides/vcs.md` for version control conventions.

## Code Style

Follow `docs/style-guides/go.md` for Go code conventions.

## Temporary Files

Store temporary files (downloads, logs, caches) in `.cache/tmp/`
or `.cache/<date>-<topic>/`. Do not put files in `/tmp`.

## Investigating Issues

When investigating CI failures or bugs:

1. Analyze before proposing fixes: Carefully analyze the issue first, grounding
   observations in logs and code. Do not jump to fixes without understanding the
   root cause.

2. Record logs for search: Instead of repeatedly querying the API, download
   the log to a file under `.cache/<date>-<topic>/` and run commands against it.

3. Ask for approval: Before implementing a fix, present your analysis and
   proposed solution for approval.
