// Package gitsnap reads cheap git metadata via shelling out. All errors are
// non-fatal — capture should proceed even if a particular field can't be read.
package gitsnap

import (
	"bytes"
	"os/exec"
	"strings"
)

type Snapshot struct {
	Branch     string
	HeadCommit string
	Dirty      bool
}

func runIn(cwd string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = cwd
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", err
	}
	return strings.TrimSpace(stdout.String()), nil
}

func ConfigEmail(cwd string) string {
	v, _ := runIn(cwd, "config", "user.email")
	return v
}

func ConfigName(cwd string) string {
	v, _ := runIn(cwd, "config", "user.name")
	return v
}

func ConfigSigningKey(cwd string) string {
	v, _ := runIn(cwd, "config", "user.signingkey")
	return v
}

func Read(cwd string) Snapshot {
	branch, _ := runIn(cwd, "rev-parse", "--abbrev-ref", "HEAD")
	if branch == "HEAD" {
		// Detached HEAD: leave branch empty.
		branch = ""
	}
	head, _ := runIn(cwd, "rev-parse", "HEAD")
	if !isHexSha(head) {
		head = ""
	}
	porcelain, _ := runIn(cwd, "status", "--porcelain")
	return Snapshot{
		Branch:     branch,
		HeadCommit: head,
		Dirty:      porcelain != "",
	}
}

func isHexSha(s string) bool {
	if len(s) < 7 || len(s) > 40 {
		return false
	}
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}

// CommitsSinceSha returns commit SHAs reachable from HEAD that are not
// reachable from baseSha — i.e. commits made between then and now on the
// current branch. Returns nil for empty baseSha (e.g. prompt was on an empty
// repo) or any git failure.
//
// More precise than `--since=<date>`: not affected by clock-skew or
// second-resolution timestamp comparisons. The downside is that we can't
// detect commits made on other branches that haven't been merged into HEAD —
// fine for v1, since the typical agent flow is "edit + commit on the current
// branch".
func CommitsSinceSha(cwd, baseSha string) []string {
	if !isHexSha(baseSha) {
		return nil
	}
	out, err := runIn(cwd, "log", baseSha+"..HEAD", "--pretty=format:%H")
	if err != nil || out == "" {
		return nil
	}
	lines := strings.Split(out, "\n")
	var sha []string
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if isHexSha(l) {
			sha = append(sha, l)
		}
	}
	return sha
}
