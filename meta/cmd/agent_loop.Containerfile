FROM golang:1.25-bookworm

ARG CLAUDE_CODE_VERSION=2.1.76
ARG CODEX_VERSION=0.114.0

RUN apt-get update \
    && apt-get install -y --no-install-recommends git ca-certificates nodejs npm \
    && npm install -g "@anthropic-ai/claude-code@${CLAUDE_CODE_VERSION}" "@openai/codex@${CODEX_VERSION}" \
    && npm cache clean --force \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /src

COPY go.work go.work
COPY go.work.sum go.work.sum
COPY common common
COPY meta meta

RUN go build -o /usr/local/bin/meta ./meta/cmd
RUN claude --version \
    && codex --version \
    && /usr/local/bin/meta --help >/dev/null

ENTRYPOINT ["/usr/local/bin/meta", "agent-loop-inner"]
