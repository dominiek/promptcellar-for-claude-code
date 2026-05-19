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
//	destination [<path>|--clear] Set/clear/show the workspace prompts destination
//	                             for cwd (writes cwd/.promptcellar/config.json).
//	                             Use this when running claude above multiple repos.
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

const Version = "0.7.0"

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
	case "destination":
		os.Exit(cmdDestination(cwd, args))
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
  pc-cli destination [<path>|--clear]
  pc-cli version
  pc-cli uninstall

Default scope for enable/disable is the repo (committed .promptcellar/config.json).
--for-me writes the gitignored personal override; --global writes ~/.promptcellar/config.json.

destination redirects prompt capture to the given directory. Set this in a
parent folder when running claude above multiple repos, so all prompts land
in one shared store. Without args, shows the current resolved destination.`)
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
	if r.Source == "none" {
		fmt.Println("     root:          (none — capture is OFF)")
		fmt.Println()
		fmt.Println("To capture prompts in this folder, set a destination:")
		fmt.Println("  pc-cli destination <path>          (or /promptcellar:destination <path>)")
		fmt.Println()
		fmt.Println("Use this when running `claude` above multiple repos and you want all")
		fmt.Println("prompts to land in one dedicated prompts/spec repo. The destination is")
		fmt.Printf("written to %s in the current folder.\n", config.RepoConfigFile)
		return 0
	}
	fmt.Printf("     root:          %s  (%s)\n", r.Root, r.Source)
	fmt.Printf("     prompts dir:   %s\n", capture.PromptsRoot(r.Root))
	fmt.Printf("     state dir:     %s\n", capture.StateRoot(r.Root))

	records, _ := plfread.ReadAll(capture.PromptsRoot(r.Root))
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
	r := config.Resolve(cwd)
	if r.Source == "none" {
		fmt.Fprintln(os.Stderr, "no prompts root: not in a git repo and no destination configured.")
		fmt.Fprintln(os.Stderr, "set one with `pc-cli destination <path>`.")
		return 1
	}
	records, err := plfread.ReadAll(capture.PromptsRoot(r.Root))
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

	manifest := findManifest(binDir)
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

// ─── destination ────────────────────────────────────────────────────────────

func cmdDestination(cwd string, args []string) int {
	clear := false
	dest := ""
	for _, a := range args {
		switch {
		case a == "--clear":
			clear = true
		case strings.HasPrefix(a, "--"):
			fmt.Fprintln(os.Stderr, "unexpected flag:", a)
			return 2
		default:
			if dest != "" {
				fmt.Fprintln(os.Stderr, "destination accepts a single path argument")
				return 2
			}
			dest = a
		}
	}

	if !clear && dest == "" {
		// Read-only: show resolved destination for cwd.
		r := config.Resolve(cwd)
		if r.Source == "none" {
			fmt.Println("no destination configured and no git repo above cwd.")
			fmt.Println("set one with: pc-cli destination <path>")
			return 0
		}
		fmt.Printf("root:        %s\n", r.Root)
		fmt.Printf("source:      %s\n", r.Source)
		fmt.Printf("prompts dir: %s\n", capture.PromptsRoot(r.Root))
		return 0
	}

	if clear {
		path, err := config.SetDestination(cwd, "")
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			return 1
		}
		fmt.Printf("cleared destination in %s\n", path)
		return 0
	}

	// Validate the path exists (or warn if it doesn't — we still write).
	abs := dest
	if !filepath.IsAbs(abs) {
		if a, err := filepath.Abs(filepath.Join(cwd, dest)); err == nil {
			abs = a
		}
	}
	if info, err := os.Stat(abs); err != nil {
		fmt.Fprintf(os.Stderr, "warning: %s does not exist yet — capture will fail until it is created.\n", abs)
	} else if !info.IsDir() {
		fmt.Fprintln(os.Stderr, "error: destination must be a directory:", abs)
		return 1
	}

	path, err := config.SetDestination(cwd, dest)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	fmt.Printf("set destination → %s\n", abs)
	fmt.Printf("wrote: %s\n", path)
	fmt.Println()
	fmt.Println("Open a new Claude Code session in this folder (or any descendant) for")
	fmt.Println("the change to take effect. Captured prompts will land at:")
	fmt.Printf("  %s\n", filepath.Join(abs, ".prompts"))
	return 0
}

func siblingsDir() string {
	exe, err := os.Executable()
	if err != nil {
		return "."
	}
	return filepath.Dir(exe)
}

// findManifest locates the plugin manifest relative to the binary. The build
// layout puts compiled binaries in plugin/bin/.real/, so the manifest is two
// levels up. Older installs (pre-shim) had binaries directly in plugin/bin/,
// where the manifest was one level up; we accept both so a freshly-built CLI
// keeps working against an older cache layout during upgrades.
func findManifest(binDir string) string {
	candidates := []string{
		filepath.Join(binDir, "..", "..", ".claude-plugin", "plugin.json"),
		filepath.Join(binDir, "..", ".claude-plugin", "plugin.json"),
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return candidates[0]
}

// ─── uninstall ──────────────────────────────────────────────────────────────

// cmdUninstall removes every trace of the plugin Claude Code uses to load
// commands and hooks: the registration in installed_plugins.json, the
// enabledPlugins entry in config.json, the cache directory under
// ~/.claude/plugins/cache/<marketplace>/promptcellar/ (which is what kept
// /promptcellar:* slash commands appearing after older uninstalls), and the
// dedicated promptcellar marketplace in known_marketplaces.json. Captured
// .prompts/ data in repos is intentionally left intact.
func cmdUninstall() int {
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}

	pluginsDir := filepath.Join(home, ".claude/plugins")
	ipPath := filepath.Join(pluginsDir, "installed_plugins.json")
	cfgPath := filepath.Join(pluginsDir, "config.json")
	kmPath := filepath.Join(pluginsDir, "known_marketplaces.json")
	cacheDir := filepath.Join(pluginsDir, "cache")

	actions := []string{}
	marketplaces := map[string]bool{}

	// 1. installed_plugins.json — drop any promptcellar@<marketplace> entries.
	if data, err := os.ReadFile(ipPath); err == nil {
		var d map[string]any
		if err := json.Unmarshal(data, &d); err == nil {
			if plugins, ok := d["plugins"].(map[string]any); ok {
				changed := false
				for k := range plugins {
					if strings.HasPrefix(k, "promptcellar@") {
						if parts := strings.SplitN(k, "@", 2); len(parts) == 2 {
							marketplaces[parts[1]] = true
						}
						delete(plugins, k)
						changed = true
					}
				}
				if changed {
					out, _ := json.MarshalIndent(d, "", "  ")
					if err := os.WriteFile(ipPath, out, 0o644); err == nil {
						actions = append(actions, "removed plugin entry from "+ipPath)
					}
				}
			}
		}
	}

	// 2. config.json — drop enabledPlugins["promptcellar@..."].
	if data, err := os.ReadFile(cfgPath); err == nil {
		var d map[string]any
		if err := json.Unmarshal(data, &d); err == nil {
			if enabled, ok := d["enabledPlugins"].(map[string]any); ok {
				changed := false
				for k := range enabled {
					if strings.HasPrefix(k, "promptcellar@") {
						delete(enabled, k)
						changed = true
					}
				}
				if changed {
					out, _ := json.MarshalIndent(d, "", "  ")
					if err := os.WriteFile(cfgPath, out, 0o644); err == nil {
						actions = append(actions, "removed enabledPlugins entry from "+cfgPath)
					}
				}
			}
		}
	}

	// 3. Cache directories — these are what actually feed slash commands and
	// hooks to Claude Code, and survive both `claude plugin uninstall` and the
	// pre-0.5.1 in-app uninstall. Walk every marketplace subdir under cache/
	// and remove any promptcellar/ subtree we find. Also clean the legacy
	// cache/local/promptcellar/ path used by very old dev installs.
	if entries, err := os.ReadDir(cacheDir); err == nil {
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			pcSub := filepath.Join(cacheDir, e.Name(), "promptcellar")
			if info, err := os.Stat(pcSub); err == nil && info.IsDir() {
				if err := os.RemoveAll(pcSub); err == nil {
					actions = append(actions, "removed cache directory "+pcSub)
					marketplaces[e.Name()] = true
				}
			}
		}
	}
	legacy := filepath.Join(cacheDir, "local", "promptcellar")
	if info, err := os.Stat(legacy); err == nil && info.IsDir() {
		if err := os.RemoveAll(legacy); err == nil {
			actions = append(actions, "removed legacy cache directory "+legacy)
		}
	}

	// 4. known_marketplaces.json — only drop a marketplace literally named
	// "promptcellar", since that's the dedicated one we ship. A marketplace
	// with a different name might host other plugins, so leave it alone even
	// if it carried promptcellar.
	if data, err := os.ReadFile(kmPath); err == nil {
		var d map[string]any
		if err := json.Unmarshal(data, &d); err == nil {
			if _, ok := d["promptcellar"]; ok {
				delete(d, "promptcellar")
				out, _ := json.MarshalIndent(d, "", "  ")
				if err := os.WriteFile(kmPath, out, 0o644); err == nil {
					actions = append(actions, "removed marketplace entry from "+kmPath)
				}
			}
		}
	}

	if len(actions) == 0 {
		fmt.Println("No promptcellar plugin entries found to remove.")
		return 0
	}
	sort.Strings(actions)
	fmt.Println("Uninstalled promptcellar:")
	for _, a := range actions {
		fmt.Println(" -", a)
	}
	fmt.Println()
	fmt.Println("Captured .prompts/ data is left in place.")
	fmt.Println("Restart Claude Code to drop loaded slash commands and hooks.")
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
