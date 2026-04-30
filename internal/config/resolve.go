// Package config decides whether Promptcellar should capture in a given cwd
// based on a layered config:
//
//   - cwd must be a git repo
//   - .promptcellar/config.local.json (per-clone, gitignored)  — personal opt-out
//   - .promptcellar/config.json       (committed, team-wide)   — repo-level decision
//   - ~/.promptcellar/config.json                              — machine kill-switch
//
// "Most-restrictive wins": if any layer says `enabled: false`, capture is off.
// `--for-me enable` only removes the personal opt-out; it does not override a
// repo-level or global disable (consistent with the M2 design — see §3 of the
// implementation plan).
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
	Layer   string // "default" | "personal" | "team" | "global" | "non-git"
}

// IsEnabled is the fast path used by hooks.
func IsEnabled(cwd string) bool {
	return Resolve(cwd).Enabled
}

func Resolve(cwd string) Resolved {
	if !isGitRepo(cwd) {
		return Resolved{Enabled: false, Reason: "not in a git repo", Layer: "non-git"}
	}

	if e, ok := readEnabled(filepath.Join(cwd, RepoConfigLocalFile)); ok && !e {
		return Resolved{Enabled: false, Reason: "personal opt-out (" + RepoConfigLocalFile + ")", Layer: "personal"}
	}
	if e, ok := readEnabled(filepath.Join(cwd, RepoConfigFile)); ok && !e {
		return Resolved{Enabled: false, Reason: "team opt-out (" + RepoConfigFile + " — committed)", Layer: "team"}
	}
	if home, err := os.UserHomeDir(); err == nil {
		if e, ok := readEnabled(filepath.Join(home, GlobalConfigSubpath)); ok && !e {
			return Resolved{Enabled: false, Reason: "global kill-switch (~/" + GlobalConfigSubpath + ")", Layer: "global"}
		}
	}
	return Resolved{Enabled: true, Reason: "default: enabled in git repo", Layer: "default"}
}

func isGitRepo(cwd string) bool {
	info, err := os.Lstat(filepath.Join(cwd, ".git"))
	if err != nil {
		return false
	}
	return info.IsDir() || info.Mode().IsRegular()
}

type fileSchema struct {
	Enabled *bool `json:"enabled"`
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
	data, err := json.MarshalIndent(fileSchema{Enabled: &enabled}, "", "  ")
	if err != nil {
		return "", err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o644); err != nil {
		return "", err
	}
	return path, os.Rename(tmp, path)
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
