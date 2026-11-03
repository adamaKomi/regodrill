package project

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/open-policy-agent/opa/v1/ast"
	"github.com/open-policy-agent/opa/v1/storage"
)

// tree writes files under a temporary directory and returns it.
func tree(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestLoadClassifiesTestFiles(t *testing.T) {
	dir := tree(t, map[string]string{
		"policy.rego":        "package p\n\nallow if input.ok\n",
		"policy_test.rego":   "package p_test\n\ntest_ok if true\n",
		"fixtures_test.rego": "package p_test\n\npod := {}\n",           // no test_ rule, but a test file by name
		"helpers.rego":       "package p_test\n\ntest_helper if true\n", // a test file by content
	})
	p, err := Load([]string{dir}, ast.RegoV1)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, f := range p.Files {
		got[filepath.Base(f.Path)] = f.IsTest
	}
	want := map[string]bool{"policy.rego": false, "policy_test.rego": true, "fixtures_test.rego": true, "helpers.rego": true}
	for name, isTest := range want {
		if got[name] != isTest {
			t.Errorf("%s: IsTest %v, want %v", name, got[name], isTest)
		}
	}
}

func TestLoadDataFilesAndIgnore(t *testing.T) {
	dir := tree(t, map[string]string{
		"policy.rego":         "package p\n\nok if data.limits.max > 1\n",
		"data.json":           `{"limits": {"max": 3}}`,
		"teams/data.yaml":     "names: [a, b]\n",
		"manifests/pod-a.yml": "kind: Pod\n",
		"manifests/pod-b.yml": "kind: Pod\n",
	})
	// The two manifests load as conflicting data.
	_, err := Load([]string{dir}, ast.RegoV1)
	var invalid *InvalidError
	if !errors.As(err, &invalid) || !strings.Contains(err.Error(), "merge error") || !strings.Contains(err.Error(), "--ignore") {
		t.Fatalf("got %v, want a merge error with a hint about --ignore", err)
	}
	// Ignoring them by name, like opa test --ignore.
	p, err := Load([]string{dir}, ast.RegoV1, "*.yml")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	txn, err := p.Store.NewTransaction(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Store.Abort(ctx, txn)
	for path, want := range map[string]string{"/limits/max": "3", "/teams/names/1": "b"} {
		v, err := p.Store.Read(ctx, txn, storage.MustParsePath(path))
		if err != nil {
			t.Errorf("%s: %v", path, err)
			continue
		}
		if got := ast.MustInterfaceToValue(v).String(); strings.Trim(got, `"`) != want {
			t.Errorf("%s = %s, want %s", path, got, want)
		}
	}
}

func TestLoadErrors(t *testing.T) {
	// A missing path is a plain error (a usage error), not an InvalidError.
	_, err := Load([]string{filepath.Join(t.TempDir(), "missing")}, ast.RegoV1)
	var invalid *InvalidError
	if err == nil || errors.As(err, &invalid) || !strings.Contains(err.Error(), "no such file or directory") {
		t.Errorf("missing path: got %v", err)
	}

	// Rego v0 loaded as v1: an InvalidError with a hint.
	v0 := tree(t, map[string]string{"policy.rego": "package p\n\nallow {\n\tinput.ok\n}\n"})
	_, err = Load([]string{v0}, ast.RegoV1)
	if !errors.As(err, &invalid) || !invalid.ParsesAsV0 || !strings.Contains(err.Error(), "--v0-compatible") {
		t.Errorf("v0 as v1: got %v", err)
	}
	if _, err := Load([]string{v0}, ast.RegoV0); err != nil {
		t.Errorf("v0 with RegoV0: %v", err)
	}

	// A file that parses in neither version: no v0 hint.
	broken := tree(t, map[string]string{"policy.rego": "package p\n\nallow if {\n"})
	_, err = Load([]string{broken}, ast.RegoV1)
	if !errors.As(err, &invalid) || invalid.ParsesAsV0 {
		t.Errorf("broken file: got %v (ParsesAsV0 %v)", err, invalid != nil && invalid.ParsesAsV0)
	}

	// No Rego file at all.
	_, err = Load([]string{tree(t, map[string]string{"data.json": "{}"})}, ast.RegoV1)
	if err == nil || !strings.Contains(err.Error(), "no .rego file") {
		t.Errorf("no rego: got %v", err)
	}
}

func TestLoadAbsolutePathKeepsLocations(t *testing.T) {
	dir := tree(t, map[string]string{"policy.rego": "package p\n\nallow if input.ok\n"})
	abs, err := filepath.Abs(dir)
	if err != nil {
		t.Fatal(err)
	}
	p, err := Load([]string{abs}, ast.RegoV1)
	if err != nil {
		t.Fatal(err)
	}
	// The file path must match the parsed locations: the loader's map keys drop the leading "/".
	f := p.Files[0]
	if f.Path != f.Module.Rules[0].Location.File {
		t.Errorf("path %q, location file %q", f.Path, f.Module.Rules[0].Location.File)
	}
}

func TestNetworkBuiltinsAreOffByDefault(t *testing.T) {
	p, err := Load([]string{tree(t, map[string]string{"policy.rego": "package p\n\nallow if input.ok\n"})}, ast.RegoV1)
	if err != nil {
		t.Fatal(err)
	}
	has := func(name string) bool {
		for _, b := range p.Capabilities.Builtins {
			if b.Name == name {
				return true
			}
		}
		return false
	}
	if has("http.send") || has("net.lookup_ip_addr") || !has("count") {
		t.Errorf("default capabilities: http.send %v, net.lookup_ip_addr %v, count %v", has("http.send"), has("net.lookup_ip_addr"), has("count"))
	}
	p.AllowNetwork()
	if !has("http.send") {
		t.Error("AllowNetwork did not restore http.send")
	}
}
