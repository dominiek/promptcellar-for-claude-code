#!/usr/bin/env bash
# M1 integration tests. Drives the compiled hook binaries via synthetic stdin
# in a series of temp git repos, asserts state + JSONL output for each scenario,
# and validates every emitted record against promptcellar-format/schemas/plf-1.schema.json.
#
# Run with: bash test/m1_integration.sh
set -euo pipefail

REPO=$(cd "$(dirname "$0")/.." && pwd)
BIN="$REPO/plugin/bin"
SCHEMA="$REPO/promptcellar-format/schemas/plf-1.schema.json"

if [ ! -x "$BIN/pc-hook-session" ]; then
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

# stdin payload helpers
session_start() {
  local sid=$1 cwd=$2 tp=$3 model=${4:-claude-opus-4-7[1m]} src=${5:-startup}
  cat <<EOF
{"session_id":"$sid","transcript_path":"$tp","cwd":"$cwd","permission_mode":"default","hook_event_name":"SessionStart","model":"$model","source":"$src"}
EOF
}
user_prompt_submit() {
  local sid=$1 cwd=$2 tp=$3 prompt=$4
  python3 -c "
import json, sys
sys.stdout.write(json.dumps({
  'session_id': '$sid',
  'transcript_path': '$tp',
  'cwd': '$cwd',
  'permission_mode': 'default',
  'hook_event_name': 'UserPromptSubmit',
  'prompt': '''$prompt'''
}))
"
}
stop_event() {
  local sid=$1 cwd=$2 tp=$3 msg=$4
  python3 -c "
import json
print(json.dumps({
  'session_id': '$sid',
  'transcript_path': '$tp',
  'cwd': '$cwd',
  'permission_mode': 'default',
  'hook_event_name': 'Stop',
  'last_assistant_message': '''$msg''',
  'stop_hook_active': False
}))
"
}

validate_jsonl() {
  local file=$1
  python3 - "$SCHEMA" "$file" <<'PY'
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

# ─── 1. Happy path: prompt → stop → record present immediately ──────────────────
echo "[1] happy path (flush-at-Stop)"
SID=11111111-1111-1111-1111-111111111111
TP=/tmp/pc-m1-test-1.jsonl
D=$(mkrepo /tmp/pc-m1-test-1)
( cd "$D" && echo x > a && git add a && git commit -q -m init )
: > "$TP"

session_start $SID "$D" "$TP" | env AI_AGENT=claude-code/2.1.123/harness "$BIN/pc-hook-session" >/dev/null
user_prompt_submit $SID "$D" "$TP" "first prompt" | "$BIN/pc-hook-prompt" >/dev/null
[ ! -d "$D/.prompts" ] && pass "no jsonl yet — only buffered state after UserPromptSubmit" || fail "flushed without Stop"
stop_event $SID "$D" "$TP" "did the work" | "$BIN/pc-hook-stop" >/dev/null
LINES=$(find "$D/.prompts" -name '*.jsonl' -exec wc -l {} \; | awk '{print $1}' | tr -d ' ')
[ "$LINES" = "1" ] && pass "1 record emitted at Stop (synchronous flush)" || fail "expected 1 record, got $LINES"
validate_jsonl "$D/.prompts"/*/*/*/*/*.jsonl
F=$(find "$D/.prompts" -name '*.jsonl' | head -1)
STATUS=$(python3 -c "import json; print(json.loads(open('$F').readline())['outcome']['status'])")
[ "$STATUS" = "completed" ] && pass "outcome.status=completed" || fail "expected completed, got $STATUS"
TV=$(python3 -c "import json; print(json.loads(open('$F').readline())['tool']['version'])")
[ "$TV" = "2.1.123" ] && pass "tool.version=2.1.123 (parsed from AI_AGENT)" || fail "expected 2.1.123, got '$TV'"
# Submit a second prompt + stop; should produce a 2nd record with parent linking
user_prompt_submit $SID "$D" "$TP" "second prompt" | "$BIN/pc-hook-prompt" >/dev/null
stop_event $SID "$D" "$TP" "more work" | "$BIN/pc-hook-stop" >/dev/null
LINES=$(find "$D/.prompts" -name '*.jsonl' -exec wc -l {} \; | awk '{print $1}' | tr -d ' ')
[ "$LINES" = "2" ] && pass "2 records after second prompt+stop" || fail "expected 2 records, got $LINES"
PARENT=$(python3 -c "import json; lines=open('$F').readlines(); print(json.loads(lines[1])['parent']['prompt_id'])")
EXPECTED_PARENT=$(python3 -c "import json; lines=open('$F').readlines(); print(json.loads(lines[0])['id'])")
[ "$PARENT" = "$EXPECTED_PARENT" ] && pass "second record's parent.prompt_id links to first record" || fail "parent mismatch: $PARENT vs $EXPECTED_PARENT"

# ─── 2. Interrupted: prompt with NO stop → next prompt flushes as interrupted ──
echo "[2] interrupted (no Stop before next prompt)"
SID=22222222-2222-2222-2222-222222222222
TP=/tmp/pc-m1-test-2.jsonl
D=$(mkrepo /tmp/pc-m1-test-2)
( cd "$D" && echo x > a && git add a && git commit -q -m init )
: > "$TP"
session_start $SID "$D" "$TP" | env AI_AGENT=claude-code/2.1.123/harness "$BIN/pc-hook-session" >/dev/null
user_prompt_submit $SID "$D" "$TP" "interrupted prompt" | "$BIN/pc-hook-prompt" >/dev/null
# Skip Stop. Submit a new prompt — should flush prior as interrupted.
user_prompt_submit $SID "$D" "$TP" "next prompt" | "$BIN/pc-hook-prompt" >/dev/null
F=$(find "$D/.prompts" -name '*.jsonl' | head -1)
STATUS=$(python3 -c "import json; print(json.loads(open('$F').readline())['outcome']['status'])")
[ "$STATUS" = "interrupted" ] && pass "outcome.status=interrupted" || fail "expected interrupted, got $STATUS"
SUMMARY=$(python3 -c "import json; r=json.loads(open('$F').readline()); print(repr(r['outcome'].get('summary','<absent>')))")
[ "$SUMMARY" = "'<absent>'" ] && pass "no outcome.summary on interrupted" || fail "summary should be absent: $SUMMARY"
validate_jsonl "$F"

# ─── 3. Orphan recovery: a session that crashed mid-prompt (no Stop ever) ─────
echo "[3] orphan recovery (mid-prompt crash)"
SID_OLD=33330000-aaaa-bbbb-cccc-000000000001
SID_NEW=33333333-3333-3333-3333-333333333333
TP=/tmp/pc-m1-test-3.jsonl
D=$(mkrepo /tmp/pc-m1-test-3)
( cd "$D" && echo x > a && git add a && git commit -q -m init )
: > "$TP"
# Phase A: simulate a crashed session — prompt buffered, but no Stop ever fires.
session_start $SID_OLD "$D" "$TP" | env AI_AGENT=claude-code/2.1.123/harness "$BIN/pc-hook-session" >/dev/null
user_prompt_submit $SID_OLD "$D" "$TP" "old prompt that never finished" | "$BIN/pc-hook-prompt" >/dev/null
[ ! -d "$D/.prompts" ] && pass "crashed session left a buffered pending (no jsonl)" || fail "unexpected jsonl"
[ -f "$D/.promptcellar/state/$SID_OLD.json" ] && pass "old state file present" || fail "old state file missing"
# Phase B: new session starts → orphan recovery flushes the old session as interrupted.
session_start $SID_NEW "$D" "$TP" | env AI_AGENT=claude-code/2.1.123/harness "$BIN/pc-hook-session" >/dev/null
F=$(find "$D/.prompts" -name '*.jsonl' | head -1)
LINES=$(find "$D/.prompts" -name '*.jsonl' -exec cat {} \; | wc -l | tr -d ' ')
[ "$LINES" = "1" ] && pass "old session flushed by orphan recovery" || fail "expected 1 record, got $LINES"
STATUS=$(python3 -c "import json; print(json.loads(open('$F').readline())['outcome']['status'])")
[ "$STATUS" = "interrupted" ] && pass "orphan flushed as interrupted" || fail "expected interrupted, got $STATUS"
[ ! -f "$D/.promptcellar/state/$SID_OLD.json" ] && pass "old state file removed" || fail "old state file not cleaned up"
[ -f "$D/.promptcellar/state/$SID_NEW.json" ] && pass "new state file present" || fail "new state file missing"
validate_jsonl "$F"

# ─── 4. Non-git dir: hooks are silent no-ops ──────────────────────────────────
echo "[4] non-git directory"
SID=44444444-4444-4444-4444-444444444444
TP=/tmp/pc-m1-test-4.jsonl
D=/tmp/pc-m1-nongit
rm -rf "$D"; mkdir -p "$D"
: > "$TP"
session_start $SID "$D" "$TP" | env AI_AGENT=claude-code/2.1.123/harness "$BIN/pc-hook-session" >/dev/null
user_prompt_submit $SID "$D" "$TP" "should be ignored" | "$BIN/pc-hook-prompt" >/dev/null
stop_event $SID "$D" "$TP" "should be ignored" | "$BIN/pc-hook-stop" >/dev/null
[ ! -d "$D/.prompts" ] && [ ! -d "$D/.promptcellar" ] && pass "non-git dir: no .prompts, no .promptcellar" || fail "non-git dir touched"

# ─── 5. Empty repo (no HEAD): record without git.head_commit ──────────────────
echo "[5] empty repo (no HEAD)"
SID=55555555-5555-5555-5555-555555555555
TP=/tmp/pc-m1-test-5.jsonl
D=$(mkrepo /tmp/pc-m1-test-5)
: > "$TP"
session_start $SID "$D" "$TP" | env AI_AGENT=claude-code/2.1.123/harness "$BIN/pc-hook-session" >/dev/null
user_prompt_submit $SID "$D" "$TP" "first prompt in empty repo" | "$BIN/pc-hook-prompt" >/dev/null
stop_event $SID "$D" "$TP" "answer" | "$BIN/pc-hook-stop" >/dev/null
F=$(find "$D/.prompts" -name '*.jsonl' | head -1)
HC=$(python3 -c "import json; r=json.loads(open('$F').readline()); print(repr(r.get('git',{}).get('head_commit','<absent>')))")
[ "$HC" = "'<absent>'" ] && pass "git.head_commit omitted for empty repo" || fail "head_commit should be absent: $HC"
validate_jsonl "$F"

# ─── 6. Concurrent sessions: two session ids → two distinct files ─────────────
echo "[6] concurrent sessions, no merge conflict"
SID_A=66666666-aaaa-aaaa-aaaa-666666666666
SID_B=66666666-bbbb-bbbb-bbbb-666666666666
TP_A=/tmp/pc-m1-test-6a.jsonl
TP_B=/tmp/pc-m1-test-6b.jsonl
D=$(mkrepo /tmp/pc-m1-test-6)
( cd "$D" && echo x > a && git add a && git commit -q -m init )
: > "$TP_A"; : > "$TP_B"
session_start $SID_A "$D" "$TP_A" | env AI_AGENT=claude-code/2.1.123/harness "$BIN/pc-hook-session" >/dev/null
session_start $SID_B "$D" "$TP_B" | env AI_AGENT=claude-code/2.1.123/harness "$BIN/pc-hook-session" >/dev/null
user_prompt_submit $SID_A "$D" "$TP_A" "session A prompt" | "$BIN/pc-hook-prompt" >/dev/null
user_prompt_submit $SID_B "$D" "$TP_B" "session B prompt" | "$BIN/pc-hook-prompt" >/dev/null
stop_event $SID_A "$D" "$TP_A" "answer A" | "$BIN/pc-hook-stop" >/dev/null
stop_event $SID_B "$D" "$TP_B" "answer B" | "$BIN/pc-hook-stop" >/dev/null
FILES=$(find "$D/.prompts" -name '*.jsonl' | wc -l | tr -d ' ')
[ "$FILES" = "2" ] && pass "two distinct session files" || fail "expected 2 files, got $FILES"
NAMES=$(find "$D/.prompts" -name '*.jsonl' -exec basename {} \; | sort)
echo "$NAMES" | grep -q "$SID_A" && pass "session A file present" || fail "session A file missing"
echo "$NAMES" | grep -q "$SID_B" && pass "session B file present" || fail "session B file missing"
for f in $(find "$D/.prompts" -name '*.jsonl'); do validate_jsonl "$f"; done

echo
echo "All M1 integration scenarios passed."
