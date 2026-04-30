#!/usr/bin/env python3
"""Promptcellar discovery: dump every hook invocation for M0 design research.

Always exits 0. Never blocks the session. Best-effort logging.
"""
import json
import os
import shutil
import sys
from datetime import datetime, timezone
from pathlib import Path

DUMP_ROOT = Path.home() / ".promptcellar-discovery"


def main() -> None:
    event = sys.argv[1] if len(sys.argv) > 1 else "UnknownEvent"

    raw_stdin = sys.stdin.read()
    try:
        stdin_payload = json.loads(raw_stdin) if raw_stdin else None
        parse_error = None
    except Exception as e:
        stdin_payload = None
        parse_error = str(e)

    payload_dict = stdin_payload if isinstance(stdin_payload, dict) else {}
    session_id = (
        payload_dict.get("session_id")
        or os.environ.get("CLAUDE_SESSION_ID")
        or "unknown"
    )

    session_dir = DUMP_ROOT / session_id
    session_dir.mkdir(parents=True, exist_ok=True)

    seq_file = session_dir / ".seq"
    seq = int(seq_file.read_text()) + 1 if seq_file.exists() else 1
    seq_file.write_text(str(seq))

    transcript_stats = None
    transcript_path = payload_dict.get("transcript_path")
    if transcript_path and Path(transcript_path).is_file():
        try:
            with open(transcript_path, "r", encoding="utf-8") as f:
                lines = f.readlines()
            type_counts: dict[str, int] = {}
            for line in lines:
                try:
                    rec = json.loads(line)
                    t = rec.get("type", "_no_type")
                    type_counts[t] = type_counts.get(t, 0) + 1
                except Exception:
                    type_counts["_unparseable"] = type_counts.get("_unparseable", 0) + 1
            transcript_stats = {
                "line_count": len(lines),
                "type_counts": type_counts,
            }
        except Exception as e:
            transcript_stats = {"_read_error": str(e)}

    record = {
        "seq": seq,
        "event": event,
        "wall_ts": datetime.now(timezone.utc).isoformat(),
        "stdin_parsed": stdin_payload,
        "stdin_raw": raw_stdin if stdin_payload is None else None,
        "stdin_parse_error": parse_error,
        "env_full": dict(os.environ),
        "argv": sys.argv,
        "cwd": os.getcwd(),
        "pid": os.getpid(),
        "transcript_stats": transcript_stats,
    }

    out = session_dir / f"{seq:04d}-{event}.json"
    out.write_text(json.dumps(record, indent=2, default=str, sort_keys=True))

    if transcript_path and Path(transcript_path).is_file():
        snap = session_dir / f"{seq:04d}-{event}.transcript.jsonl"
        try:
            shutil.copy2(transcript_path, snap)
        except Exception as e:
            (session_dir / f"{seq:04d}-{event}.transcript-error.txt").write_text(str(e))

    print("{}", flush=True)
    sys.exit(0)


if __name__ == "__main__":
    try:
        main()
    except Exception as e:
        try:
            err_dir = DUMP_ROOT / "_errors"
            err_dir.mkdir(parents=True, exist_ok=True)
            (err_dir / f"{datetime.now(timezone.utc).isoformat()}.txt").write_text(
                f"event={sys.argv[1:]!r}\nerror={e!r}\n"
            )
        except Exception:
            pass
        print("{}", flush=True)
        sys.exit(0)
