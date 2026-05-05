---
description: Set, clear, or show the workspace prompts destination — use this when running claude above multiple repos and you want all prompts to land in one shared store
argument-hint: "[<path>|--clear]"
allowed-tools: ["Bash"]
---

Run `pc-cli destination $ARGUMENTS` via the Bash tool and show the output verbatim.

If the user passed a path, the destination is written to `.promptcellar/config.json` in the current folder; that folder becomes the workspace root and all prompt capture in it (or any descendant cwd) redirects to `<destination>/.prompts/`. With no arguments the command prints the resolved destination. With `--clear`, the destination is removed and capture falls back to the nearest git repo (or OFF if none).
