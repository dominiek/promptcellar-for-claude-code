#!/usr/bin/env bash
# M2 + M5 integration tests:
#   - .promptcellarignore → excluded stub
#   - pc-cli enable/disable resolution across team / personal / global
#   - pc-cli status / log
#   - pc-mcp JSON-RPC: initialize, tools/list, tools/call (search, log)
set -euo pipefail

REPO=$(cd "$(dirname "$0")/.." && pwd)
BIN="$REPO/plugin/bin"
SCHEMA="$REPO/test/fixtures/plf-1.schema.json"

if [ ! -x "$BIN/pc-cli" ] || [ ! -x "$BIN/pc-mcp" ]; then
  echo "binaries not built — run 'make build' first" >&2
  exit 2
fi

pass() { echo "  ✓ $*"; }
fail() { echo "  ✗ $*" >&2; exit 1; }

mkrepo() {
  local d=$1
  rm -rf "$d"
  mkdir -p "$d"
  ( cd "$d" && git init -q && git config user.email "test@example.com" && git config user.name "Test" && echo x > a && git add a && git commit -q -m init )
  echo "$d"
}

run_session_start() { echo "$1" | env AI_AGENT=claude-code/2.1.123/harness "$BIN/pc-hook-session" >/dev/null; }
run_prompt()        { echo "$1" | "$BIN/pc-hook-prompt" >/dev/null; }
run_stop()          { echo "$1" | "$BIN/pc-hook-stop"   >/dev/null; }

# ─── M2.1: .promptcellarignore → excluded stub ────────────────────────────────
echo "[M2.1] .promptcellarignore matches → excluded stub written immediately"
SID=m2-ign-1111-2222-3333-444444444444
TP=/tmp/pc-m2-test-1.transcript.jsonl
D=$(mkrepo /tmp/pc-m2-test-1)
: > "$TP"
cat > "$D/.promptcellarignore" <<'EOF'
id: secrets
(GITHUB_TOKEN|AWS_SECRET_ACCESS_KEY)

id: credential-shapes
ghp_[A-Za-z0-9]{36}
EOF

run_session_start '{"session_id":"'$SID'","transcript_path":"'$TP'","cwd":"'$D'","permission_mode":"default","hook_event_name":"SessionStart","model":"claude-opus-4-7[1m]","source":"startup"}'
# A clean prompt — should be captured normally.
run_prompt '{"session_id":"'$SID'","transcript_path":"'$TP'","cwd":"'$D'","permission_mode":"default","hook_event_name":"UserPromptSubmit","prompt":"clean prompt"}'
run_stop '{"session_id":"'$SID'","transcript_path":"'$TP'","cwd":"'$D'","permission_mode":"default","hook_event_name":"Stop","last_assistant_message":"ok","stop_hook_active":false}'
# A prompt with a secret-shaped value — should produce an excluded stub, no normal record.
run_prompt '{"session_id":"'$SID'","transcript_path":"'$TP'","cwd":"'$D'","permission_mode":"default","hook_event_name":"UserPromptSubmit","prompt":"my GITHUB_TOKEN is ghp_abcdefghijklmnopqrstuvwxyzABCDEFGHIJ"}'

F=$(find "$D/.prompts" -name '*.jsonl' | head -1)
LINES=$(wc -l < "$F" | tr -d ' ')
[ "$LINES" = "2" ] && pass "2 records (1 captured + 1 excluded stub)" || fail "expected 2 records, got $LINES"
PATTERN=$(python3 -c "import json; r=json.loads(open('$F').readlines()[1]); print(r.get('excluded',{}).get('pattern_id','-'))")
[ "$PATTERN" = "secrets" ] && pass "stub.pattern_id=secrets" || fail "expected pattern_id=secrets, got $PATTERN"
HAS_PROMPT=$(python3 -c "import json; r=json.loads(open('$F').readlines()[1]); print('yes' if 'prompt' in r else 'no')")
[ "$HAS_PROMPT" = "no" ] && pass "stub omits prompt text (no leak)" || fail "stub should not contain prompt"
python3 - "$SCHEMA" "$F" <<'PY'
import json, sys, jsonschema
s = json.load(open(sys.argv[1])); ok = 0
for line in open(sys.argv[2]):
    if not line.strip(): continue
    jsonschema.validate(json.loads(line), s); ok += 1
print(f"OK  {ok} record(s) validate")
PY

# ─── M2.2: pc-cli enable / disable / status (team layer) ──────────────────────
echo "[M2.2] pc-cli enable/disable (team layer is the default)"
D=$(mkrepo /tmp/pc-m2-test-2)
( cd "$D" && "$BIN/pc-cli" status >/dev/null )
( cd "$D" && "$BIN/pc-cli" disable >/dev/null )
[ -f "$D/.promptcellar/config.json" ] && pass "team disable wrote .promptcellar/config.json" || fail "team config.json not written"
ENABLED=$(python3 -c "import json; print(json.load(open('$D/.promptcellar/config.json'))['enabled'])")
[ "$ENABLED" = "False" ] && pass "team config.enabled=false" || fail "expected False, got $ENABLED"
STATUS_OUT=$( cd "$D" && "$BIN/pc-cli" status )
echo "$STATUS_OUT" | grep -q "OFF" && pass "status reports OFF after team disable" || fail "status should report OFF"
echo "$STATUS_OUT" | grep -q "team opt-out" && pass "status names the team layer as the cause" || fail "status should mention team layer"
( cd "$D" && "$BIN/pc-cli" enable >/dev/null )
ENABLED=$(python3 -c "import json; print(json.load(open('$D/.promptcellar/config.json'))['enabled'])")
[ "$ENABLED" = "True" ] && pass "team enable flips it back to True" || fail "expected True, got $ENABLED"

# ─── M2.3: pc-cli disable --for-me overrides team-enabled ─────────────────────
echo "[M2.3] --for-me layer takes precedence (disable wins over team-enabled)"
( cd "$D" && "$BIN/pc-cli" disable --for-me >/dev/null )
[ -f "$D/.promptcellar/config.local.json" ] && pass "personal disable wrote config.local.json (gitignored)" || fail "config.local.json not written"
STATUS_OUT=$( cd "$D" && "$BIN/pc-cli" status )
echo "$STATUS_OUT" | grep -q "OFF" && pass "status=OFF (personal opt-out beats team-enabled)" || fail "status should be OFF"
echo "$STATUS_OUT" | grep -q "personal opt-out" && pass "status names the personal layer" || fail "status should mention personal"

# ─── M2.4: pc-cli log emits one-line summaries ────────────────────────────────
echo "[M2.4] pc-cli log shows captured records"
SID=m2-log-1111-2222-3333-444444444444
TP=/tmp/pc-m2-test-4.transcript.jsonl
D=$(mkrepo /tmp/pc-m2-test-4)
: > "$TP"
run_session_start '{"session_id":"'$SID'","transcript_path":"'$TP'","cwd":"'$D'","permission_mode":"default","hook_event_name":"SessionStart","model":"claude-opus-4-7[1m]","source":"startup"}'
run_prompt '{"session_id":"'$SID'","transcript_path":"'$TP'","cwd":"'$D'","permission_mode":"default","hook_event_name":"UserPromptSubmit","prompt":"hello world"}'
run_stop '{"session_id":"'$SID'","transcript_path":"'$TP'","cwd":"'$D'","permission_mode":"default","hook_event_name":"Stop","last_assistant_message":"ok","stop_hook_active":false}'
LOG_OUT=$( cd "$D" && "$BIN/pc-cli" log )
echo "$LOG_OUT" | grep -q "hello world" && pass "log contains the prompt text" || fail "expected 'hello world' in log"
echo "$LOG_OUT" | grep -q "completed" && pass "log shows status=completed" || fail "expected 'completed' in log"

# ─── M2.5: pc-cli doctor passes when binaries are siblings ────────────────────
echo "[M2.5] pc-cli doctor"
D=$(mkrepo /tmp/pc-m2-test-5)
DOCTOR_OUT=$( cd "$D" && "$BIN/pc-cli" doctor 2>&1 )
echo "$DOCTOR_OUT" | grep -q "✓  hook binary present: pc-hook-session" && pass "doctor sees pc-hook-session" || fail "doctor missed pc-hook-session"
echo "$DOCTOR_OUT" | grep -q "✓  hook binary present: pc-hook-prompt"  && pass "doctor sees pc-hook-prompt"  || fail "doctor missed pc-hook-prompt"
echo "$DOCTOR_OUT" | grep -q "✓  hook binary present: pc-hook-tool"    && pass "doctor sees pc-hook-tool"    || fail "doctor missed pc-hook-tool"
echo "$DOCTOR_OUT" | grep -q "✓  hook binary present: pc-hook-stop"    && pass "doctor sees pc-hook-stop"    || fail "doctor missed pc-hook-stop"
echo "$DOCTOR_OUT" | grep -q "✓  plugin manifest readable" && pass "doctor finds plugin manifest" || fail "doctor missed plugin manifest"

# ─── M5.1: pc-mcp tools/list and tools/call ───────────────────────────────────
echo "[M5.1] pc-mcp JSON-RPC: initialize + tools/list + tools/call(search)"
D=/tmp/pc-m2-test-4 # reuse — has 1 captured prompt
TOOLS_OUT=$( ( printf '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}\n{"jsonrpc":"2.0","id":2,"method":"tools/list"}\n' ) | ( cd "$D" && "$BIN/pc-mcp" ) )
echo "$TOOLS_OUT" | grep -q '"name":"promptcellar.search"' && pass "tools/list includes promptcellar.search" || fail "missing search tool"
echo "$TOOLS_OUT" | grep -q '"name":"promptcellar.log"' && pass "tools/list includes promptcellar.log" || fail "missing log tool"
echo "$TOOLS_OUT" | grep -q '"name":"promptcellar.touched"' && pass "tools/list includes promptcellar.touched" || fail "missing touched tool"
echo "$TOOLS_OUT" | grep -q '"name":"promptcellar.session"' && pass "tools/list includes promptcellar.session" || fail "missing session tool"

CALL_OUT=$( ( printf '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}\n{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"promptcellar.search","arguments":{"query":"hello"}}}\n' ) | ( cd "$D" && "$BIN/pc-mcp" ) )
echo "$CALL_OUT" | grep -q "hello world" && pass "tools/call(search query=hello) returns matching prompt" || fail "search did not return expected match"

LOG_OUT=$( ( printf '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}\n{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"promptcellar.log","arguments":{}}}\n' ) | ( cd "$D" && "$BIN/pc-mcp" ) )
echo "$LOG_OUT" | grep -q "hello world" && pass "tools/call(log) returns recent records" || fail "log did not return records"

# ─── M5.2: pc-mcp unknown tool → error ────────────────────────────────────────
echo "[M5.2] pc-mcp returns JSON-RPC error for unknown tool"
ERR_OUT=$( ( printf '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}\n{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"promptcellar.nope","arguments":{}}}\n' ) | ( cd "$D" && "$BIN/pc-mcp" ) )
echo "$ERR_OUT" | grep -q "unknown tool" && pass "unknown tool yields error response" || fail "expected error for unknown tool"

echo
echo "All M2 + M5 integration scenarios passed."
