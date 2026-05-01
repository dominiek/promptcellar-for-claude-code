// Command pc-cli is the user-facing tool. It is the implementation behind the
// `/promptcellar:*` slash commands — those markdown files just instruct the
// agent to invoke this binary via the Bash tool.
//
// Subcommands (see also plugin/commands/*.md):
//
//	status                       Resolved config + counts.
//	enable  [--for-me|--global]  Write the corresponding config (default: team / committed).
//	disable [--for-me|--global]  Same.
//	log [N]                      Print the last N captured prompts in this repo.
//	doctor                       Check hook binaries, config, git state.
//	version                      Plugin version.
//	uninstall                    Remove the plugin entry; data is left intact.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"promptcellar/internal/capture"
	"promptcellar/internal/config"
	"promptcellar/internal/plf"
	"promptcellar/internal/plfread"
)

const Version = "0.4.0"

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(2)
	}
	cmd := os.Args[1]
	args := os.Args[2:]

	cwd, _ := os.Getwd()

	switch cmd {
	case "status":
		os.Exit(cmdStatus(cwd, args))
	case "enable":
		os.Exit(cmdSetEnabled(cwd, args, true))
	case "disable":
		os.Exit(cmdSetEnabled(cwd, args, false))
	case "log":
		os.Exit(cmdLog(cwd, args))
	case "doctor":
		os.Exit(cmdDoctor(cwd))
	case "version":
		fmt.Println(Version)
		return
	case "uninstall":
		os.Exit(cmdUninstall())
	case "help", "-h", "--help":
		printUsage()
		return
	default:
		fmt.Fprintln(os.Stderr, "unknown command:", cmd)
		printUsage()
		os.Exit(2)
	}
}

func printUsage() {
	fmt.Println(`pc-cli — Promptcellar control surface

Usage:
  pc-cli status
  pc-cli enable  [--for-me] [--global]
  pc-cli disable [--for-me] [--global]
  pc-cli log [N]
  pc-cli doctor
  pc-cli version
  pc-cli uninstall

Default scope for enable/disable is the repo (committed .promptcellar/config.json).
--for-me writes the gitignored personal override; --global writes ~/.promptcellar/config.json.`)
}

// ─── status ─────────────────────────────────────────────────────────────────

func cmdStatus(cwd string, _ []string) int {
	r := config.Resolve(cwd)
	icon := "ON "
	if !r.Enabled {
		icon = "OFF"
	}
	fmt.Printf("%s  %s\n", icon, r.Reason)
	fmt.Printf("     cwd:           %s\n", cwd)
	fmt.Printf("     prompts dir:   %s\n", capture.PromptsRoot(cwd))
	fmt.Printf("     state dir:     %s\n", capture.StateRoot(cwd))

	records, _ := plfread.ReadAll(capture.PromptsRoot(cwd))
	captured, excluded := 0, 0
	sessions := map[string]struct{}{}
	for _, rec := range records {
		sessions[rec.SessionID] = struct{}{}
		if rec.Excluded != nil {
			excluded++
		} else {
			captured++
		}
	}
	fmt.Printf("     records:       %d captured, %d excluded across %d session(s)\n",
		captured, excluded, len(sessions))
	return 0
}

// ─── enable / disable ───────────────────────────────────────────────────────

func cmdSetEnabled(cwd string, args []string, enabled bool) int {
	layer := "team"
	for _, a := range args {
		switch a {
		case "--for-me":
			layer = "personal"
		case "--global":
			layer = "global"
		default:
			fmt.Fprintln(os.Stderr, "unexpected arg:", a)
			return 2
		}
	}
	if (layer == "team" || layer == "personal") && !insideGitRepo(cwd) {
		fmt.Fprintln(os.Stderr, "must be run inside a git repo for --team / --for-me; use --global to set the machine-wide kill-switch")
		return 1
	}
	path, err := config.SetEnabled(cwd, layer, enabled)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	verb := "enabled"
	if !enabled {
		verb = "disabled"
	}
	fmt.Printf("%s capture at layer %q\n", verb, layer)
	fmt.Printf("wrote: %s\n", path)
	if layer == "team" {
		fmt.Println("\nThis is a committed file shared with your team.")
		fmt.Println("Stage and push so collaborators inherit the change:")
		fmt.Printf("  git add %s && git commit -m \"promptcellar: %s repo capture\" && git push\n", config.RepoConfigFile, verb)
	}
	return 0
}

// ─── log ────────────────────────────────────────────────────────────────────

func cmdLog(cwd string, args []string) int {
	n := 10
	if len(args) >= 1 {
		v, err := strconv.Atoi(args[0])
		if err != nil || v <= 0 {
			fmt.Fprintln(os.Stderr, "invalid N:", args[0])
			return 2
		}
		n = v
	}
	records, err := plfread.ReadAll(capture.PromptsRoot(cwd))
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	if len(records) == 0 {
		fmt.Println("(no records yet)")
		return 0
	}
	if n > len(records) {
		n = len(records)
	}
	for _, r := range records[:n] {
		fmt.Println(formatRecordLine(&r))
	}
	if n < len(records) {
		fmt.Printf("\n... %d more records. Pass a larger N to see them.\n", len(records)-n)
	}
	return 0
}

func formatRecordLine(r *plf.Record) string {
	ts := r.Timestamp
	if t, err := time.Parse(time.RFC3339, ts); err == nil {
		ts = t.Local().Format("2006-01-02 15:04:05")
	}
	if r.Excluded != nil {
		id := r.Excluded.PatternID
		if id == "" {
			id = "—"
		}
		return fmt.Sprintf("%s  [excluded:%s] %s", ts, id, r.Excluded.Reason)
	}
	status := "?"
	files, commits := 0, 0
	cost := 0.0
	if r.Outcome != nil {
		if r.Outcome.Status != "" {
			status = r.Outcome.Status
		}
		files = len(r.Outcome.FilesTouched)
		commits = len(r.Outcome.Commits)
	}
	if r.Enrichments != nil {
		cost = r.Enrichments.CostUSD
	}
	prompt := strings.ReplaceAll(r.Prompt, "\n", " ")
	if len(prompt) > 80 {
		prompt = prompt[:77] + "..."
	}
	extras := []string{}
	if files > 0 {
		extras = append(extras, fmt.Sprintf("files:%d", files))
	}
	if commits > 0 {
		extras = append(extras, fmt.Sprintf("commits:%d", commits))
	}
	if cost > 0 {
		extras = append(extras, fmt.Sprintf("$%.4f", cost))
	}
	tail := ""
	if len(extras) > 0 {
		tail = "  (" + strings.Join(extras, ", ") + ")"
	}
	return fmt.Sprintf("%s  [%s] %s%s", ts, status, prompt, tail)
}

// ─── doctor ─────────────────────────────────────────────────────────────────

func cmdDoctor(cwd string) int {
	checks := []struct {
		label string
		ok    bool
		note  string
	}{}

	r := config.Resolve(cwd)
	checks = append(checks, struct {
		label string
		ok    bool
		note  string
	}{
		"capture enabled in this cwd",
		r.Enabled,
		r.Reason,
	})

	binDir := siblingsDir()
	for _, name := range []string{"pc-hook-session", "pc-hook-prompt", "pc-hook-tool", "pc-hook-stop"} {
		path := filepath.Join(binDir, name)
		_, err := os.Stat(path)
		checks = append(checks, struct {
			label string
			ok    bool
			note  string
		}{
			"hook binary present: " + name,
			err == nil,
			path,
		})
	}

	manifest := filepath.Join(binDir, "..", ".claude-plugin", "plugin.json")
	_, err := os.Stat(manifest)
	checks = append(checks, struct {
		label string
		ok    bool
		note  string
	}{
		"plugin manifest readable",
		err == nil,
		manifest,
	})

	allOK := true
	for _, c := range checks {
		mark := "✓"
		if !c.ok {
			mark = "✗"
			allOK = false
		}
		fmt.Printf("  %s  %s — %s\n", mark, c.label, c.note)
	}
	if !allOK {
		return 1
	}
	return 0
}

func siblingsDir() string {
	exe, err := os.Executable()
	if err != nil {
		return "."
	}
	return filepath.Dir(exe)
}

// ─── uninstall ──────────────────────────────────────────────────────────────

func cmdUninstall() int {
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	ipPath := filepath.Join(home, ".claude/plugins/installed_plugins.json")
	cfgPath := filepath.Join(home, ".claude/plugins/config.json")

	removedFrom := []string{}

	if data, err := os.ReadFile(ipPath); err == nil {
		var d map[string]any
		if err := json.Unmarshal(data, &d); err == nil {
			if plugins, ok := d["plugins"].(map[string]any); ok {
				before := len(plugins)
				for k := range plugins {
					if strings.HasPrefix(k, "promptcellar@") {
						delete(plugins, k)
					}
				}
				if len(plugins) != before {
					out, _ := json.MarshalIndent(d, "", "  ")
					_ = os.WriteFile(ipPath, out, 0o644)
					removedFrom = append(removedFrom, ipPath)
				}
			}
		}
	}

	if data, err := os.ReadFile(cfgPath); err == nil {
		var d map[string]any
		if err := json.Unmarshal(data, &d); err == nil {
			if enabled, ok := d["enabledPlugins"].(map[string]any); ok {
				before := len(enabled)
				for k := range enabled {
					if strings.HasPrefix(k, "promptcellar@") {
						delete(enabled, k)
					}
				}
				if len(enabled) != before {
					out, _ := json.MarshalIndent(d, "", "  ")
					_ = os.WriteFile(cfgPath, out, 0o644)
					removedFrom = append(removedFrom, cfgPath)
				}
			}
		}
	}

	if len(removedFrom) == 0 {
		fmt.Println("No promptcellar plugin entries found to remove.")
		return 0
	}
	sort.Strings(removedFrom)
	fmt.Println("Removed promptcellar entries from:")
	for _, p := range removedFrom {
		fmt.Println(" -", p)
	}
	fmt.Println()
	fmt.Println("Captured .prompts/ data is left in place.")
	fmt.Println("To remove the plugin files: rm -rf ~/.claude/plugins/cache/local/promptcellar/")
	return 0
}

// ─── helpers ────────────────────────────────────────────────────────────────

func insideGitRepo(cwd string) bool {
	info, err := os.Lstat(filepath.Join(cwd, ".git"))
	if err != nil {
		return !errors.Is(err, os.ErrNotExist) && false
	}
	return info.IsDir() || info.Mode().IsRegular()
}
