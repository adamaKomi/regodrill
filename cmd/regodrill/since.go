package main

import (
	"bufio"
	"bytes"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// changes are the lines added or changed since a git reference, by absolute file path
// (docs/mutation-testing-spec.md §8).
type changes map[string]*fileChanges

type fileChanges struct {
	all   bool // a new, untracked file: every line counts
	lines map[int]bool
}

// contains reports whether a line of a file was added or changed.
func (c changes) contains(file string, line int) bool {
	abs, err := filepath.Abs(file)
	if err != nil {
		return false
	}
	// git prints paths without symbolic links (/private/tmp on macOS, for /tmp).
	if real, err := filepath.EvalSymlinks(abs); err == nil {
		abs = real
	}
	fc, ok := c[abs]
	return ok && (fc.all || fc.lines[line])
}

// changedSince runs git in dir: the lines added or changed between the merge base of ref and HEAD, and the
// working tree. Uncommitted changes count, and so do untracked files, which are new.
func changedSince(dir, ref string) (changes, error) {
	top, err := git(dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, err
	}
	top = strings.TrimSpace(top)
	if real, err := filepath.EvalSymlinks(top); err == nil {
		top = real
	}
	base, err := git(dir, "merge-base", ref, "HEAD")
	if err != nil {
		return nil, fmt.Errorf("--since %s: %w", ref, err)
	}
	diff, err := git(top, "-c", "core.quotePath=off", "diff", "--unified=0", "--no-color", "--no-ext-diff", "--no-renames", strings.TrimSpace(base))
	if err != nil {
		return nil, err
	}
	c := parseDiff(top, diff)
	untracked, err := git(top, "-c", "core.quotePath=off", "ls-files", "--others", "--exclude-standard")
	if err != nil {
		return nil, err
	}
	for _, f := range strings.Split(strings.TrimSpace(untracked), "\n") {
		if f != "" {
			c[filepath.Join(top, f)] = &fileChanges{all: true}
		}
	}
	return c, nil
}

// parseDiff reads the added line ranges of a `git diff --unified=0`: `+++ b/path` names the file and
// `@@ -a,b +c,d @@` adds lines c to c+d-1 (d is 1 when omitted, 0 for a pure deletion).
func parseDiff(top, diff string) changes {
	c := changes{}
	var cur *fileChanges
	sc := bufio.NewScanner(strings.NewReader(diff))
	sc.Buffer(make([]byte, 1024*1024), 64*1024*1024)
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, "+++ "):
			name := strings.TrimPrefix(line, "+++ ")
			if name == "/dev/null" {
				cur = nil
				continue
			}
			name = strings.TrimPrefix(name, "b/")
			path := filepath.Join(top, filepath.FromSlash(name))
			if c[path] == nil {
				c[path] = &fileChanges{lines: map[int]bool{}}
			}
			cur = c[path]
		case strings.HasPrefix(line, "@@ ") && cur != nil:
			// @@ -a,b +c,d @@ context
			fields := strings.Fields(line)
			if len(fields) < 3 || !strings.HasPrefix(fields[2], "+") {
				continue
			}
			start, count := fields[2][1:], "1"
			if i := strings.IndexByte(start, ','); i >= 0 {
				start, count = start[:i], start[i+1:]
			}
			s, err1 := strconv.Atoi(start)
			n, err2 := strconv.Atoi(count)
			if err1 != nil || err2 != nil {
				continue
			}
			for l := s; l < s+n; l++ {
				cur.lines[l] = true
			}
		}
	}
	return c
}

func git(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), msg)
	}
	return stdout.String(), nil
}
