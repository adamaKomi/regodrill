package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/adamaKomi/regodrill/internal/mutate"
)

// defaultConfig is read from the current directory when --config is not given.
const defaultConfig = ".regodrill.yaml"

// config is the content of .regodrill.yaml (docs/mutation-testing-spec.md §7.2). Paths in it are relative
// to the file's directory. Command-line flags take precedence over it.
type config struct {
	Version   int      `yaml:"version"`
	Paths     []string `yaml:"paths"`
	Exclude   []string `yaml:"exclude"`
	Threshold *float64 `yaml:"threshold"`
	Operators struct {
		Include []string `yaml:"include"`
		Exclude []string `yaml:"exclude"`
	} `yaml:"operators"`
	Ignore []struct {
		File     string `yaml:"file"`
		Line     int    `yaml:"line"`
		Pattern  string `yaml:"pattern"`
		Operator string `yaml:"operator"`
		Reason   string `yaml:"reason"`
	} `yaml:"ignore"`
	IgnoreFiles   []string `yaml:"ignore-files"`
	V0Compatible  *bool    `yaml:"v0-compatible"`
	Parallel      *int     `yaml:"parallel"`
	TimeoutFactor *float64 `yaml:"timeout-factor"`
	AllowNetwork  *bool    `yaml:"allow-network"`

	dir  string // directory of the file
	name string // path of the file, for messages
}

// loadConfig reads a configuration file. With explicit false, a missing default file is not an error.
func loadConfig(path string, explicit bool) (*config, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) && !explicit {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var c config
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true) // a misspelled key is an error, not a silently ignored setting
	if err := dec.Decode(&c); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if c.Version != 1 {
		return nil, fmt.Errorf("%s: version must be 1, got %d", path, c.Version)
	}
	if c.Threshold != nil && (*c.Threshold < 0 || *c.Threshold > 100) {
		return nil, fmt.Errorf("%s: threshold must be between 0 and 100", path)
	}
	for _, id := range append(append([]string{}, c.Operators.Include...), c.Operators.Exclude...) {
		if !isOperator(id) {
			return nil, fmt.Errorf("%s: unknown operator %q", path, id)
		}
	}
	for i, ig := range c.Ignore {
		switch {
		case ig.Pattern != "" && (ig.File != "" || ig.Line != 0):
			return nil, fmt.Errorf("%s: ignore entry %d has both a pattern and a file or line", path, i+1)
		case ig.Pattern != "":
			if _, err := regexp.Compile(ig.Pattern); err != nil {
				return nil, fmt.Errorf("%s: ignore entry %d: invalid pattern: %w", path, i+1, err)
			}
		case ig.File == "" || ig.Line <= 0:
			return nil, fmt.Errorf("%s: ignore entry %d needs a file and a line, or a pattern", path, i+1)
		}
		if ig.Operator != "" && !isOperator(ig.Operator) {
			return nil, fmt.Errorf("%s: ignore entry %d: unknown operator %q", path, i+1, ig.Operator)
		}
	}
	for _, g := range c.Exclude {
		if _, err := globRegexp(g); err != nil {
			return nil, fmt.Errorf("%s: exclude %q: %w", path, g, err)
		}
	}
	c.name = path
	c.dir = filepath.Dir(path)
	return &c, nil
}

// paths returns the configured paths, relative to the current directory.
func (c *config) paths() []string {
	out := make([]string, len(c.Paths))
	for i, p := range c.Paths {
		out[i] = c.resolve(p)
	}
	return out
}

func (c *config) resolve(p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(c.dir, p)
}

// ignores returns the ignore entries as mutate.Ignore values.
func (c *config) ignores() []*mutate.Ignore {
	var out []*mutate.Ignore
	for i, ig := range c.Ignore {
		m := &mutate.Ignore{Line: ig.Line, Reason: ig.Reason, Origin: fmt.Sprintf("%s (ignore entry %d)", c.name, i+1)}
		if ig.Pattern != "" {
			m.Pattern = regexp.MustCompile(ig.Pattern) // validated by loadConfig
		} else {
			m.File = c.resolve(ig.File)
		}
		if ig.Operator != "" {
			m.Operators = []string{ig.Operator}
		}
		out = append(out, m)
	}
	return out
}

// excluded reports whether a policy file matches one of the exclude globs. Globs are relative to the
// configuration file's directory and use `/`; `**` matches any number of directories.
func (c *config) excluded(file string) bool {
	if len(c.Exclude) == 0 {
		return false
	}
	abs, err := filepath.Abs(file)
	if err != nil {
		return false
	}
	base, err := filepath.Abs(c.dir)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(base, abs)
	if err != nil || strings.HasPrefix(rel, "..") {
		return false
	}
	rel = filepath.ToSlash(rel)
	for _, g := range c.Exclude {
		if re, err := globRegexp(g); err == nil && re.MatchString(rel) {
			return true
		}
	}
	return false
}

// globRegexp converts a glob with `**`, `*` and `?` to an anchored regular expression.
func globRegexp(glob string) (*regexp.Regexp, error) {
	var b strings.Builder
	b.WriteString("^")
	for i := 0; i < len(glob); i++ {
		switch c := glob[i]; c {
		case '*':
			if i+1 < len(glob) && glob[i+1] == '*' {
				i++
				if i+1 < len(glob) && glob[i+1] == '/' {
					i++
					b.WriteString("(?:.*/)?") // `**/` matches zero or more directories
				} else {
					b.WriteString(".*")
				}
			} else {
				b.WriteString("[^/]*")
			}
		case '?':
			b.WriteString("[^/]")
		default:
			b.WriteString(regexp.QuoteMeta(string(c)))
		}
	}
	b.WriteString("$")
	return regexp.Compile(b.String())
}

func isOperator(id string) bool {
	for _, op := range mutate.Operators {
		if op == id {
			return true
		}
	}
	return false
}
