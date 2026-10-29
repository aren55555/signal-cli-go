// SPDX-License-Identifier: GPL-3.0-or-later

package repocheck

import (
	"debug/buildinfo"
	"debug/elf"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// The production binaries are every command under cmd/. They are built
// with CGO_ENABLED=0 and must contain no C code (operator decision,
// 2026-10-29): only the race-detector test run may enable cgo.

func productionCommands(t *testing.T) []string {
	entries, err := os.ReadDir(filepath.Join(repoRoot, "cmd"))
	if err != nil {
		t.Fatal(err)
	}
	var cmds []string
	for _, e := range entries {
		if e.IsDir() {
			cmds = append(cmds, e.Name())
		}
	}
	if !slices.Contains(cmds, "signal-cli-go") {
		t.Fatalf("cmd/ lists %q; signal-cli-go missing", cmds)
	}
	return cmds
}

// goEnv is the environment for the go tool: cgo off, vendored modules.
func goEnv(extra ...string) []string {
	var env []string
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "CGO_ENABLED=") && !strings.HasPrefix(kv, "GOFLAGS=") {
			env = append(env, kv)
		}
	}
	return append(append(env, "GOFLAGS=-mod=vendor"), extra...)
}

func buildCommand(t *testing.T, name string) string {
	out := filepath.Join(t.TempDir(), name)
	cmd := exec.Command("go", "build", "-o", out, "./cmd/"+name)
	cmd.Dir = repoRoot
	cmd.Env = goEnv("CGO_ENABLED=0")
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build ./cmd/%s: %v\n%s", name, err, b)
	}
	return out
}

// TestProductionBinariesHaveNoC builds each command the way it ships and
// checks the result: built with CGO_ENABLED=0, statically linked (no ELF
// interpreter, no shared libraries), and only allowed modules inside.
// The main binary may contain protobuf and nothing else.
func TestProductionBinariesHaveNoC(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the binaries")
	}
	for _, name := range productionCommands(t) {
		t.Run(name, func(t *testing.T) {
			path := buildCommand(t, name)
			bi, err := buildinfo.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			cgo := ""
			for _, s := range bi.Settings {
				if s.Key == "CGO_ENABLED" {
					cgo = s.Value
				}
			}
			if cgo != "0" {
				t.Errorf("CGO_ENABLED=%q in the build info; want 0", cgo)
			}
			f, err := elf.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			for _, p := range f.Progs {
				if p.Type == elf.PT_INTERP {
					t.Error("binary has an ELF interpreter (dynamically linked)")
				}
			}
			if libs, _ := f.ImportedLibraries(); len(libs) > 0 {
				t.Errorf("binary links shared libraries %q", libs)
			}
			var deps []string
			for _, d := range bi.Deps {
				deps = append(deps, d.Path+" "+d.Version)
				if d.Replace != nil {
					t.Errorf("module %s is replaced", d.Path)
				}
				if v, ok := allowedModules[d.Path]; !ok || v != d.Version {
					t.Errorf("binary contains module %s %s, which is not allowed", d.Path, d.Version)
				}
			}
			if name == "signal-cli-go" {
				if want := allowedModule + " " + allowedVersion; len(deps) != 1 || deps[0] != want {
					t.Errorf("signal-cli-go contains modules %q; only %q is allowed", deps, want)
				}
			}
		})
	}
}

// TestNoCgoPackages lists every non-standard package the commands depend
// on with cgo enabled, so that a package with C files is caught even if
// the CGO_ENABLED=0 build silently drops those files.
func TestNoCgoPackages(t *testing.T) {
	cmd := exec.Command("go", "list", "-deps", "-test=false", "-f", "{{if not .Standard}}{{.ImportPath}} {{len .CgoFiles}} {{len .CFiles}} {{len .SFiles}} {{len .SysoFiles}}{{end}}", "./cmd/...")
	cmd.Dir = repoRoot
	cmd.Env = goEnv("CGO_ENABLED=1")
	b, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list: %v", err)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		f := strings.Fields(line)
		if len(f) == 5 && (f[1] != "0" || f[2] != "0" || f[4] != "0") {
			t.Errorf("package %s has cgo, C or syso files (%s)", f[0], line)
		}
	}
}
