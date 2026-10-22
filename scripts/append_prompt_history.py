#!/usr/bin/env python3
"""Append a timestamped paraphrased prompt to PROMPT_HISTORY.md.

Usage:
  python3 scripts/append_prompt_history.py "Paraphrased prompt text" "Short outcome"

This script will append a new bullet with the current ISO timestamp.

The bullet has the form ``- [YYYY-MM-DDTHH:MM:SSZ] "paraphrase"`` followed by
`` — Outcome: outcome`` when a non-empty second argument is given. The
timestamp is UTC with microseconds dropped. Both arguments are stripped of
surrounding whitespace; any arguments after the second are ignored.

PROMPT_HISTORY.md is resolved relative to the current working directory, so
run the script from the repository root. The file is never created: if it is
missing the script prints a message and exits 1. It also exits 1 (after
printing usage) when no paraphrase argument is given, and exits 0 after a
successful append. A blank line is written before each entry.
"""
import sys
import datetime
from pathlib import Path

if len(sys.argv) < 2:
    print("Usage: append_prompt_history.py \"paraphrase\" [outcome]")
    sys.exit(1)

paraphrase = sys.argv[1].strip()
outcome = sys.argv[2].strip() if len(sys.argv) > 2 else ''
filep = Path('PROMPT_HISTORY.md')
if not filep.exists():
    print('PROMPT_HISTORY.md not found in repo root')
    sys.exit(1)

now = datetime.datetime.utcnow().replace(microsecond=0).isoformat() + 'Z'
entry_lines = []
entry_lines.append(f"- [{now}] \"{paraphrase}\"")
if outcome:
    entry_lines[-1] += f" — Outcome: {outcome}"

with filep.open('a', encoding='utf-8') as f:
    f.write('\n')
    f.write('\n'.join(entry_lines))
    f.write('\n')

print('Appended prompt to PROMPT_HISTORY.md')
