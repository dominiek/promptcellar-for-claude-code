#!/usr/bin/env bash
# Shared assertion helper for both E2E scenarios.
#
# Run inside the test container (or wherever has /repo mounted with the
# plugin checkout) AFTER `claude -p "$EXPECTED_PROMPT"` has finished.
#
# Asserts:
#   1. Exactly one .jsonl file under <work-dir>/.prompts/YYYY/MM/DD/.
#   2. The file contains exactly one PLF record (one line of JSON).
#   3. The record validates against /repo/test/fixtures/plf-1.schema.json.
#   4. record.prompt == $EXPECTED_PROMPT
#   5. record.outcome.status == "completed"
#   6. record.author.email is non-empty (proves git config was picked up).
#
# Exits 0 on success, non-zero with a clear stderr message on the first
# failure encountered.

set -euo pipefail

WORK_DIR="${1:-$PWD}"
EXPECTED_PROMPT="${2:?usage: assert-prompt-captured.sh <work-dir> <expected-prompt>}"
SCHEMA="${SCHEMA:-/repo/test/fixtures/plf-1.schema.json}"

prompts_dir="$WORK_DIR/.prompts"

fail() { echo "ASSERT FAIL: $*" >&2; exit 1; }
ok()   { echo "  ✓ $*"; }

# 1. Exactly one .jsonl file somewhere under .prompts/YYYY/MM/DD/.
if [ ! -d "$prompts_dir" ]; then
  fail "no .prompts/ directory at $prompts_dir — capture didn't fire at all"
fi

mapfile -t records < <(find "$prompts_dir" -type f -name '*.jsonl' | sort)
if [ "${#records[@]}" -eq 0 ]; then
  fail "no .jsonl files under $prompts_dir — hooks ran but wrote nothing"
fi
if [ "${#records[@]}" -gt 1 ]; then
  fail "expected exactly 1 captured file, found ${#records[@]}: ${records[*]}"
fi
record_file="${records[0]}"
ok "exactly one record file: ${record_file#$WORK_DIR/}"

# 2. Exactly one JSON line.
line_count=$(wc -l < "$record_file" | tr -d ' ')
if [ "$line_count" != "1" ]; then
  fail "expected 1 line in $record_file, found $line_count"
fi
ok "single JSONL record"

# 3. Schema validation. Uses python3 + jsonschema — same validator the
#    M1/M2/M3 suites use, so "valid" means the same thing across suites.
if ! command -v python3 >/dev/null 2>&1; then
  fail "python3 not on PATH — image is missing python3-jsonschema"
fi
if ! python3 - "$SCHEMA" "$record_file" <<'PY' 2>/tmp/ajv.err
import json, sys, jsonschema
schema = json.load(open(sys.argv[1]))
records = [json.loads(line) for line in open(sys.argv[2]) if line.strip()]
for i, r in enumerate(records):
    jsonschema.validate(r, schema)
PY
then
  echo "schema validation output:" >&2
  cat /tmp/ajv.err >&2
  fail "record failed PLF-1 schema validation"
fi
ok "validates against PLF-1 schema"

# 4. Prompt text matches exactly.
captured_prompt=$(jq -r '.prompt // empty' "$record_file")
if [ "$captured_prompt" != "$EXPECTED_PROMPT" ]; then
  fail "prompt mismatch: expected '$EXPECTED_PROMPT', got '$captured_prompt'"
fi
ok "prompt text matches"

# 5. Outcome status. Hooks set this at Stop time; "completed" means the
#    session finished normally. "errored" or "interrupted" usually means
#    something went wrong during the round trip itself, not in capture.
status=$(jq -r '.outcome.status // empty' "$record_file")
if [ "$status" != "completed" ]; then
  fail "expected outcome.status=completed, got '$status'"
fi
ok "outcome.status=completed"

# 6. Author email present (proves the git identity was picked up; if this is
#    empty the capture is from a non-git directory or git config wasn't set).
author_email=$(jq -r '.author.email // empty' "$record_file")
if [ -z "$author_email" ]; then
  fail "author.email is empty — Promptcellar didn't pick up the git identity"
fi
ok "author.email=$author_email"

echo "PASS: $(basename "$record_file")"
