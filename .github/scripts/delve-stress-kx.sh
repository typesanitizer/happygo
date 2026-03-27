#!/usr/bin/env bash
set -euo pipefail

artifact_dir=${1:?artifact directory is required}
mkdir -p "$artifact_dir"
summary_file="$artifact_dir/summary.txt"
overall_status=0

log() {
  printf '[happygo-kx][ci][%s] %s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$*" | tee -a "$summary_file"
}

dump_state() {
  local phase=$1
  local out="$artifact_dir/${phase}.diag.txt"

  {
    echo "# phase: $phase"
    date -u
    echo
    echo "## go version"
    go version
    echo
    echo "## go env"
    go env GOOS GOARCH GOROOT GOCACHE GOPATH GOMOD
    echo
    echo "## xcode-select"
    xcode-select -p || true
    echo
    echo "## debugserver path"
    xcrun -f debugserver || true
    echo
    echo "## pgrep"
    pgrep -fl 'debugserver|lldb-server|dlv|fncall|loopprog|bphitcountchain|setiterator' || true
    echo
    echo "## ps"
    ps -axo pid,ppid,user,%cpu,%mem,etime,state,command | grep -E 'debugserver|lldb-server|dlv|fncall|loopprog|bphitcountchain|setiterator|go test' | grep -v grep || true
    echo
    echo "## lsof listening tcp"
    lsof -nP -iTCP -sTCP:LISTEN | grep -E 'debugserver|lldb-server|dlv' || true
    echo
    echo "## vm_stat"
    vm_stat || true
    echo
    echo "## df -h"
    df -h || true
    echo
    echo "## ulimit -a"
    ulimit -a || true
  } >"$out" 2>&1

  if command -v sample >/dev/null 2>&1; then
    while read -r pid _; do
      sample "$pid" 1 1 -file "$artifact_dir/${phase}.sample.${pid}.txt" >/dev/null 2>&1 || true
    done < <(pgrep -fl 'debugserver|lldb-server' || true)
  fi
}

record_status() {
  local name=$1
  local status=$2
  printf '%s\n' "$status" >"$artifact_dir/${name}.status"
  if [[ $status -ne 0 && $overall_status -eq 0 ]]; then
    overall_status=$status
  fi
}

run_go_test_phase() {
  local name=$1
  shift
  local log_file="$artifact_dir/${name}.log"

  log "phase start name=${name} command=$*"
  set +e
  "$@" 2>&1 | tee "$log_file"
  local status=${PIPESTATUS[0]}
  set -e
  record_status "$name" "$status"
  log "phase end name=${name} status=${status}"
  dump_state "$name"
}

run_concurrent_waitfor_phase() {
  local name=$1
  local log_file="$artifact_dir/${name}.log"
  local -a pids=()
  local status=0
  local worker_status=0

  log "phase start name=${name} workers=4 test=TestWaitForAttach count=20"
  : >"$log_file"

  for worker in 1 2 3 4; do
    (
      set -euo pipefail
      env PROCTEST=lldb go test -C delve ./pkg/proc \
        -run '^TestWaitForAttach$' \
        -count=20 \
        -shuffle=off \
        -timeout=10m \
        -v
    ) >"$artifact_dir/${name}.worker${worker}.log" 2>&1 &
    pids+=("$!")
  done

  for i in "${!pids[@]}"; do
    worker=$((i + 1))
    set +e
    wait "${pids[$i]}"
    worker_status=$?
    set -e
    printf 'worker=%d status=%d\n' "$worker" "$worker_status" | tee -a "$log_file"
    if [[ $worker_status -ne 0 && $status -eq 0 ]]; then
      status=$worker_status
    fi
  done

  record_status "$name" "$status"
  log "phase end name=${name} status=${status}"
  dump_state "$name"
}

{
  echo "# preamble"
  date -u
  echo
  go version
  echo
  go env GOOS GOARCH GOROOT GOCACHE GOPATH GOMOD
  echo
  xcode-select -p || true
  echo
  xcrun -f debugserver || true
} >"$artifact_dir/preamble.txt" 2>&1

run_go_test_phase serial-suspect \
  env PROCTEST=lldb go test -C delve ./pkg/proc \
    -run '^(TestWaitForAttach|TestChainedBreakpoint|TestCallFunction|TestSetupRangeFramesCrash)$' \
    -count=30 \
    -shuffle=off \
    -timeout=20m \
    -v

run_concurrent_waitfor_phase concurrent-waitfor

run_go_test_phase full-proc \
  env PROCTEST=lldb go test -C delve ./pkg/proc \
    -parallel=20 \
    -count=1 \
    -timeout=20m \
    -v

log "all phases complete overall_status=${overall_status}"
exit "$overall_status"
