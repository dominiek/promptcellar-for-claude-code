// Package config decides whether Promptcellar should capture in a given cwd
// based on a layered config:
//
//   - There must be a "root" — either a workspace (a directory containing a
//     .promptcellar/config.json with `destination`) or, failing that, a git
//     repo. The root determines where .prompts/ and .promptcellar/state/ live.
//   - .promptcellar/config.local.json (per-clone, gitignored)  — personal opt-out
//   - .promptcellar/config.json       (committed, team-wide)   — repo-level decision
//   - ~/.promptcellar/config.json                              — machine kill-switch
//
// "Most-restrictive wins": if any layer says `enabled: false`, capture is off.
// `--for-me enable` only removes the personal opt-out; it does not override a
// repo-level or global disable (consistent with the M2 design — see §3 of the
// implementation plan).
//
// Workspace destination (cross-repo capture):
// A directory containing .promptcellar/config.json with a non-empty
// `destination` field acts as a workspace root that redirects capture to the
// destination path (resolved relative to the workspace dir). Used when running
// `claude` a level above multiple repos and wanting all prompts to land in one
// dedicated prompts/spec repo. Workspace lookup walks up from cwd; the nearest
// ancestor with a destination wins. Workspace destination takes precedence over
// any git repo root underneath it.
package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

const (
	// File names relative to the repo root.
	RepoConfigFile      = ".promptcellar/config.json"       // committed
	RepoConfigLocalFile = ".promptcellar/config.local.json" // gitignored

	// Path under $HOME.
	GlobalConfigSubpath = ".promptcellar/config.json"
)

type Resolved struct {
	Enabled bool
	Reason  string
	Layer   string // "default" | "personal" | "team" | "global" | "no-root"

	// Root is the absolute directory under which .prompts/ and .promptcellar/
	// state/ live. Empty when Source == "none" (capture is OFF).
	Root string
	// Source describes how Root was found:
	//   "workspace" — nearest ancestor with a .promptcellar/config.json that
	//                 sets `destination`; Root is the resolved destination.
	//   "git-repo"  — nearest ancestor that is a git repo; Root is the repo.
	//   "none"      — neither found; capture is OFF.
	Source string
}

// IsEnabled is the fast path used by hooks.
func IsEnabled(cwd string) bool {
	return Resolve(cwd).Enabled
}

func Resolve(cwd string) Resolved {
	root, source := findRoot(cwd)
	if source == "none" {
		return Resolved{
			Enabled: false,
			Reason:  "no git repo above cwd and no destination configured. Run `pc-cli destination <path>` (or /promptcellar:destination <path>) to capture prompts in this folder.",
			Layer:   "no-root",
			Source:  "none",
		}
	}

	if e, ok := readEnabled(filepath.Join(root, RepoConfigLocalFile)); ok && !e {
		return Resolved{Enabled: false, Reason: "personal opt-out (" + RepoConfigLocalFile + ")", Layer: "personal", Root: root, Source: source}
	}
	if e, ok := readEnabled(filepath.Join(root, RepoConfigFile)); ok && !e {
		return Resolved{Enabled: false, Reason: "team opt-out (" + RepoConfigFile + " — committed)", Layer: "team", Root: root, Source: source}
	}
	if home, err := os.UserHomeDir(); err == nil {
		if e, ok := readEnabled(filepath.Join(home, GlobalConfigSubpath)); ok && !e {
			return Resolved{Enabled: false, Reason: "global kill-switch (~/" + GlobalConfigSubpath + ")", Layer: "global", Root: root, Source: source}
		}
	}

	reason := "default: enabled in git repo"
	if source == "workspace" {
		reason = "default: enabled via workspace destination"
	}
	return Resolved{Enabled: true, Reason: reason, Layer: "default", Root: root, Source: source}
}

// findRoot walks up from cwd looking for, in order:
//  1. an ancestor whose .promptcellar/config.json has a non-empty `destination`
//     — returns the resolved destination path (absolute) and "workspace".
//  2. an ancestor that is a git repo — returns the repo path and "git-repo".
//
// Workspace match wins even if a git repo lives below it. Returns ("", "none")
// when neither is found.
func findRoot(cwd string) (root, source string) {
	abs, err := filepath.Abs(cwd)
	if err != nil {
		return "", "none"
	}
	dir := abs
	var firstGitRepo string
	for {
		if dest, ok := readDestination(filepath.Join(dir, RepoConfigFile)); ok {
			resolved := dest
			if !filepath.IsAbs(resolved) {
				resolved = filepath.Join(dir, resolved)
			}
			if abs, err := filepath.Abs(resolved); err == nil {
				resolved = abs
			}
			return resolved, "workspace"
		}
		if firstGitRepo == "" && isGitRepo(dir) {
			firstGitRepo = dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	if firstGitRepo != "" {
		return firstGitRepo, "git-repo"
	}
	return "", "none"
}

func isGitRepo(dir string) bool {
	info, err := os.Lstat(filepath.Join(dir, ".git"))
	if err != nil {
		return false
	}
	return info.IsDir() || info.Mode().IsRegular()
}

type fileSchema struct {
	Enabled     *bool  `json:"enabled,omitempty"`
	Destination string `json:"destination,omitempty"`
}

func readEnabled(path string) (enabled, present bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return false, false
	}
	var c fileSchema
	if err := json.Unmarshal(data, &c); err != nil {
		return false, false
	}
	if c.Enabled == nil {
		return false, false
	}
	return *c.Enabled, true
}

func readDestination(path string) (dest string, present bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	var c fileSchema
	if err := json.Unmarshal(data, &c); err != nil {
		return "", false
	}
	if c.Destination == "" {
		return "", false
	}
	return c.Destination, true
}

// SetEnabled writes the `enabled` field at the requested layer.
//
//   - layer == "personal": cwd's .promptcellar/config.local.json
//   - layer == "team":     cwd's .promptcellar/config.json
//   - layer == "global":   ~/.promptcellar/config.json
//
// Returns the path written.
func SetEnabled(cwd, layer string, enabled bool) (string, error) {
	path, err := layerPath(cwd, layer)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	existing, _ := readFileSchema(path)
	existing.Enabled = &enabled
	return path, writeFileSchema(path, existing)
}

// SetDestination writes (or clears) the `destination` field in
// cwd/.promptcellar/config.json. Pass an empty path to clear.
// Returns the path written.
func SetDestination(cwd, destination string) (string, error) {
	path := filepath.Join(cwd, RepoConfigFile)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	existing, _ := readFileSchema(path)
	existing.Destination = destination
	return path, writeFileSchema(path, existing)
}

func readFileSchema(path string) (fileSchema, error) {
	var c fileSchema
	data, err := os.ReadFile(path)
	if err != nil {
		return c, err
	}
	if err := json.Unmarshal(data, &c); err != nil {
		return fileSchema{}, err
	}
	return c, nil
}

func writeFileSchema(path string, c fileSchema) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func layerPath(cwd, layer string) (string, error) {
	switch layer {
	case "personal":
		return filepath.Join(cwd, RepoConfigLocalFile), nil
	case "team":
		return filepath.Join(cwd, RepoConfigFile), nil
	case "global":
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, GlobalConfigSubpath), nil
	default:
		return "", errors.New("unknown layer: " + layer)
	}
}
