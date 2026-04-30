#!/usr/bin/env bash
# M3 integration tests. Builds on M1 by exercising the new enrichment and
# outcome-detail paths:
#   - outcome.files_touched (via PostToolUse on Edit/Write/MultiEdit/NotebookEdit)
#   - outcome.status="errored" (via PostToolBatch error patterns)
#   - outcome.commits (via `git log --since`)
#   - enrichments.tokens.* and enrichments.cost_usd (via transcript reading)
#   - enrichments.duration_ms (Stop ts - submitted_at)
#
# Run with: bash test/m3_integration.sh
set -euo pipefail

REPO=$(cd "$(dirname "$0")/.." && pwd)
BIN="$REPO/plugin/bin"
SCHEMA="$REPO/test/fixtures/plf-1.schema.json"

if [ ! -x "$BIN/pc-hook-tool" ]; then
  echo "binaries not built — run 'make build' first" >&2
  exit 2
fi

pass() { echo "  ✓ $*"; }
fail() { echo "  ✗ $*" >&2; exit 1; }

mkrepo() {
  local d=$1
  rm -rf "$d"
  mkdir -p "$d"
  ( cd "$d" && git init -q && git config user.email "test@example.com" && git config user.name "Test" )
  echo "$d"
}

run_session_start() { echo "$1" | env AI_AGENT=claude-code/2.1.123/harness "$BIN/pc-hook-session" >/dev/null; }
run_prompt()        { echo "$1" | "$BIN/pc-hook-prompt" >/dev/null; }
run_stop()          { echo "$1" | "$BIN/pc-hook-stop"   >/dev/null; }
run_tool()          { echo "$1" | "$BIN/pc-hook-tool"   >/dev/null; }

# stdin builders. Use python to do JSON quoting safely.
build_payload() {
python3 -c "
import json, sys
print(json.dumps({k: v for k, v in [t.split('=', 1) for t in sys.argv[1:]] if not k.startswith('@')}))
" "$@"
}

validate_jsonl() {
  python3 - "$SCHEMA" "$1" <<'PY'
import json, sys, jsonschema
schema = json.load(open(sys.argv[1]))
records = [json.loads(line) for line in open(sys.argv[2]) if line.strip()]
for i, r in enumerate(records):
    try:
        jsonschema.validate(r, schema)
    except Exception as e:
        print(f"record {i}: SCHEMA FAIL: {e}", file=sys.stderr)
        sys.exit(1)
print(f"OK  {len(records)} record(s) in {sys.argv[2]}")
PY
}

read_field() {
  local jsonl=$1 path=$2
  python3 -c "
import json, sys
r = json.loads(open('$jsonl').readline())
out = r
for k in '$path'.split('.'):
    if out is None: break
    out = out.get(k) if isinstance(out, dict) else None
print('<absent>' if out is None else json.dumps(out))
"
}

# ─── M3.1: files_touched via PostToolUse on Edit/Write ────────────────────────
echo "[M3.1] outcome.files_touched"
SID=m3-files-1111-2222-3333-444444444444
TP=/tmp/pc-m3-test-1.transcript.jsonl
D=$(mkrepo /tmp/pc-m3-test-1)
( cd "$D" && echo x > a && git add a && git commit -q -m init )
: > "$TP"

run_session_start '{"session_id":"'$SID'","transcript_path":"'$TP'","cwd":"'$D'","permission_mode":"default","hook_event_name":"SessionStart","model":"claude-opus-4-7[1m]","source":"startup"}'
run_prompt '{"session_id":"'$SID'","transcript_path":"'$TP'","cwd":"'$D'","permission_mode":"default","hook_event_name":"UserPromptSubmit","prompt":"edit some files"}'
# PostToolUse for Edit on src/foo.go
run_tool '{"session_id":"'$SID'","transcript_path":"'$TP'","cwd":"'$D'","permission_mode":"default","hook_event_name":"PostToolUse","tool_name":"Edit","tool_input":{"file_path":"'$D'/src/foo.go","old_string":"a","new_string":"b"},"tool_use_id":"toolu_a","duration_ms":12}'
# PostToolUse for Write on src/bar.go
run_tool '{"session_id":"'$SID'","transcript_path":"'$TP'","cwd":"'$D'","permission_mode":"default","hook_event_name":"PostToolUse","tool_name":"Write","tool_input":{"file_path":"'$D'/src/bar.go","content":"package main"},"tool_use_id":"toolu_b","duration_ms":10}'
# PostToolUse for Read on README.md (should NOT show up in files_touched)
run_tool '{"session_id":"'$SID'","transcript_path":"'$TP'","cwd":"'$D'","permission_mode":"default","hook_event_name":"PostToolUse","tool_name":"Read","tool_input":{"file_path":"'$D'/README.md"},"tool_use_id":"toolu_c","duration_ms":3}'
# PostToolBatch with no errors
run_tool '{"session_id":"'$SID'","transcript_path":"'$TP'","cwd":"'$D'","permission_mode":"default","hook_event_name":"PostToolBatch","tool_calls":[{"tool_name":"Edit","tool_input":{},"tool_response":"successfully edited","tool_use_id":"toolu_a"},{"tool_name":"Write","tool_input":{},"tool_response":"file written","tool_use_id":"toolu_b"}]}'
run_stop '{"session_id":"'$SID'","transcript_path":"'$TP'","cwd":"'$D'","permission_mode":"default","hook_event_name":"Stop","last_assistant_message":"done","stop_hook_active":false}'

F=$(find "$D/.prompts" -name '*.jsonl' | head -1)
FILES=$(read_field "$F" "outcome.files_touched")
[ "$FILES" = '["src/foo.go", "src/bar.go"]' ] && pass "files_touched=[src/foo.go, src/bar.go] (relative paths, Read excluded)" || fail "expected [src/foo.go, src/bar.go], got $FILES"
STATUS=$(read_field "$F" "outcome.status")
[ "$STATUS" = '"completed"' ] && pass "status=completed" || fail "expected completed, got $STATUS"
validate_jsonl "$F"

# ─── M3.2: outcome.status="errored" via PostToolBatch error response ──────────
echo "[M3.2] outcome.status=errored on tool failure"
SID=m3-error-1111-2222-3333-444444444444
TP=/tmp/pc-m3-test-2.transcript.jsonl
D=$(mkrepo /tmp/pc-m3-test-2)
( cd "$D" && echo x > a && git add a && git commit -q -m init )
: > "$TP"

run_session_start '{"session_id":"'$SID'","transcript_path":"'$TP'","cwd":"'$D'","permission_mode":"default","hook_event_name":"SessionStart","model":"claude-opus-4-7[1m]","source":"startup"}'
run_prompt '{"session_id":"'$SID'","transcript_path":"'$TP'","cwd":"'$D'","permission_mode":"default","hook_event_name":"UserPromptSubmit","prompt":"cat a non-existent file"}'
# PostToolBatch with an error response (no PostToolUse fired — matches M0 finding)
run_tool '{"session_id":"'$SID'","transcript_path":"'$TP'","cwd":"'$D'","permission_mode":"default","hook_event_name":"PostToolBatch","tool_calls":[{"tool_name":"Read","tool_input":{"file_path":"/nonexistent"},"tool_response":"File does not exist. Note: ...","tool_use_id":"toolu_x"}]}'
run_stop '{"session_id":"'$SID'","transcript_path":"'$TP'","cwd":"'$D'","permission_mode":"default","hook_event_name":"Stop","last_assistant_message":"that file does not exist","stop_hook_active":false}'

F=$(find "$D/.prompts" -name '*.jsonl' | head -1)
STATUS=$(read_field "$F" "outcome.status")
[ "$STATUS" = '"errored"' ] && pass "status=errored on tool error pattern" || fail "expected errored, got $STATUS"
SUMMARY=$(read_field "$F" "outcome.summary")
[ "$SUMMARY" != '<absent>' ] && pass "outcome.summary still present on errored" || fail "summary should be present even on errored"
validate_jsonl "$F"

# ─── M3.3: enrichments (tokens + cost + duration) ─────────────────────────────
echo "[M3.3] enrichments.tokens, cost_usd, duration_ms"
SID=m3-tokens-1111-2222-3333-444444444444
TP=/tmp/pc-m3-test-3.transcript.jsonl
D=$(mkrepo /tmp/pc-m3-test-3)
( cd "$D" && echo x > a && git add a && git commit -q -m init )
: > "$TP"

run_session_start '{"session_id":"'$SID'","transcript_path":"'$TP'","cwd":"'$D'","permission_mode":"default","hook_event_name":"SessionStart","model":"claude-opus-4-7[1m]","source":"startup"}'
run_prompt '{"session_id":"'$SID'","transcript_path":"'$TP'","cwd":"'$D'","permission_mode":"default","hook_event_name":"UserPromptSubmit","prompt":"hi"}'
# Sleep so transcript ts > submitted_at, then write a synthetic assistant record.
sleep 0.5
TS=$(python3 -c "from datetime import datetime, timezone; print(datetime.now(timezone.utc).strftime('%Y-%m-%dT%H:%M:%S.%fZ')[:-4]+'Z')")
cat >> "$TP" <<EOF
{"type":"user","timestamp":"$TS","message":{"role":"user","content":"hi"}}
{"type":"assistant","timestamp":"$TS","requestId":"req_test","message":{"model":"claude-opus-4-7","usage":{"input_tokens":100,"output_tokens":50,"cache_read_input_tokens":1000,"cache_creation_input_tokens":200}}}
EOF
sleep 0.2
run_stop '{"session_id":"'$SID'","transcript_path":"'$TP'","cwd":"'$D'","permission_mode":"default","hook_event_name":"Stop","last_assistant_message":"hello","stop_hook_active":false}'

F=$(find "$D/.prompts" -name '*.jsonl' | head -1)
INPUT=$(read_field "$F" "enrichments.tokens.input")
[ "$INPUT" = "100" ] && pass "tokens.input=100" || fail "expected 100, got $INPUT"
OUTPUT=$(read_field "$F" "enrichments.tokens.output")
[ "$OUTPUT" = "50" ] && pass "tokens.output=50" || fail "expected 50, got $OUTPUT"
CREAD=$(read_field "$F" "enrichments.tokens.cache_read")
[ "$CREAD" = "1000" ] && pass "tokens.cache_read=1000" || fail "expected 1000, got $CREAD"
CWRITE=$(read_field "$F" "enrichments.tokens.cache_write")
[ "$CWRITE" = "200" ] && pass "tokens.cache_write=200" || fail "expected 200, got $CWRITE"
# Cost = 100*30 + 50*150 + 1000*3 + 200*37.5 / 1M = 3000 + 7500 + 3000 + 7500 = 21000 / 1M = 0.021
COST=$(read_field "$F" "enrichments.cost_usd")
EXPECTED_COST=$(python3 -c "print(0.021)")
python3 -c "import sys; sys.exit(0 if abs($COST - $EXPECTED_COST) < 0.0001 else 1)" \
  && pass "cost_usd=$COST (≈\$0.021 for opus-4-7[1m] tier)" \
  || fail "expected cost ≈ $EXPECTED_COST, got $COST"
DURATION=$(read_field "$F" "enrichments.duration_ms")
python3 -c "import sys; sys.exit(0 if int($DURATION) > 100 else 1)" \
  && pass "duration_ms=$DURATION (>100ms)" \
  || fail "expected duration_ms > 100, got $DURATION"
validate_jsonl "$F"

# ─── M3.4: outcome.commits via git log --since ────────────────────────────────
echo "[M3.4] outcome.commits"
SID=m3-commits-1111-2222-3333-44444444444a
TP=/tmp/pc-m3-test-4.transcript.jsonl
D=$(mkrepo /tmp/pc-m3-test-4)
( cd "$D" && echo x > a && git add a && git commit -q -m init )
SHA_BEFORE=$(cd "$D" && git rev-parse HEAD)
: > "$TP"

run_session_start '{"session_id":"'$SID'","transcript_path":"'$TP'","cwd":"'$D'","permission_mode":"default","hook_event_name":"SessionStart","model":"claude-opus-4-7[1m]","source":"startup"}'
run_prompt '{"session_id":"'$SID'","transcript_path":"'$TP'","cwd":"'$D'","permission_mode":"default","hook_event_name":"UserPromptSubmit","prompt":"add a feature"}'
# Make a commit during the prompt's window.
sleep 1
( cd "$D" && echo y > b && git add b && git commit -q -m "feature: add b" )
SHA_NEW=$(cd "$D" && git rev-parse HEAD)
run_stop '{"session_id":"'$SID'","transcript_path":"'$TP'","cwd":"'$D'","permission_mode":"default","hook_event_name":"Stop","last_assistant_message":"committed","stop_hook_active":false}'

F=$(find "$D/.prompts" -name '*.jsonl' | head -1)
COMMITS=$(read_field "$F" "outcome.commits")
echo "$COMMITS" | grep -q "$SHA_NEW" && pass "outcome.commits includes new SHA" || fail "expected $SHA_NEW in $COMMITS"
echo "$COMMITS" | grep -q "$SHA_BEFORE" && fail "init commit should not be in commits (it was before submitted_at)" || pass "old commit correctly excluded"
validate_jsonl "$F"

# ─── M3.5: tool error skips PostToolUse → no files_touched, status=errored ────
echo "[M3.5] failed Edit: no files_touched, status=errored"
SID=m3-fail-edit-2222-3333-444444444444
TP=/tmp/pc-m3-test-5.transcript.jsonl
D=$(mkrepo /tmp/pc-m3-test-5)
( cd "$D" && echo x > a && git add a && git commit -q -m init )
: > "$TP"

run_session_start '{"session_id":"'$SID'","transcript_path":"'$TP'","cwd":"'$D'","permission_mode":"default","hook_event_name":"SessionStart","model":"claude-opus-4-7[1m]","source":"startup"}'
run_prompt '{"session_id":"'$SID'","transcript_path":"'$TP'","cwd":"'$D'","permission_mode":"default","hook_event_name":"UserPromptSubmit","prompt":"edit a missing file"}'
# Simulating tool error: no PostToolUse, just PostToolBatch with error.
run_tool '{"session_id":"'$SID'","transcript_path":"'$TP'","cwd":"'$D'","permission_mode":"default","hook_event_name":"PostToolBatch","tool_calls":[{"tool_name":"Edit","tool_input":{"file_path":"/nope"},"tool_response":"Error: file not found","tool_use_id":"toolu_q"}]}'
run_stop '{"session_id":"'$SID'","transcript_path":"'$TP'","cwd":"'$D'","permission_mode":"default","hook_event_name":"Stop","last_assistant_message":"failed","stop_hook_active":false}'

F=$(find "$D/.prompts" -name '*.jsonl' | head -1)
FILES=$(read_field "$F" "outcome.files_touched")
[ "$FILES" = "<absent>" ] && pass "files_touched absent on tool error" || fail "expected absent, got $FILES"
STATUS=$(read_field "$F" "outcome.status")
[ "$STATUS" = '"errored"' ] && pass "status=errored" || fail "expected errored, got $STATUS"
validate_jsonl "$F"

echo
echo "All M3 integration scenarios passed."
