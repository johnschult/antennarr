package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// buildRig is a copy of build.sh in a folder of its own, with stand-ins for go, node and npm that say what version
// they are and note what they were asked to do, so the script's checks and messages can be tried without building.
type buildRig struct {
	t     *testing.T
	dir   string // the project: build.sh, a go.mod, a web folder
	bin   string // the stand-in programs, and links to the few tools the script itself uses
	calls string // what the stand-ins were asked, one line each
}

func newBuildRig(t *testing.T) *buildRig {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("build.sh is for macOS and Linux")
	}
	script, err := os.ReadFile("../../build.sh")
	if err != nil {
		t.Skip("not run from a checkout:", err)
	}
	r := &buildRig{t: t, dir: t.TempDir(), bin: t.TempDir()}
	r.calls = filepath.Join(r.dir, "calls.log")
	must(t, os.WriteFile(filepath.Join(r.dir, "build.sh"), script, 0o755))
	must(t, os.WriteFile(filepath.Join(r.dir, "go.mod"), []byte("module x\n\ngo 1.27.3\n"), 0o644))
	must(t, os.MkdirAll(filepath.Join(r.dir, "web"), 0o755))
	must(t, os.WriteFile(filepath.Join(r.dir, "web", "package-lock.json"), []byte("{}"), 0o644))
	for _, tool := range []string{"awk", "sed", "mktemp", "cat", "rm", "dirname", "sh", "mkdir"} {
		p, err := exec.LookPath(tool)
		if err != nil {
			t.Skipf("no %s on this machine", tool)
		}
		must(t, os.Symlink(p, filepath.Join(r.bin, tool)))
	}
	r.stub("go", `case "$1" in
env) echo "go${GO_V:-1.27.3}" ;;
build) echo "go $*" >> "$CALLS"; : > yagi ;;
esac`)
	r.stub("node", `echo "${NODE_V:-24.1.0}"`)
	r.stub("npm", `echo "npm $*" >> "$CALLS"
if [ "$1" = ci ] && [ -n "${NPM_CI_FAILS:-}" ]; then echo "npm error code EACCES"; exit 1; fi
if [ "$1" = ci ]; then mkdir -p node_modules; fi`)
	return r
}

func (r *buildRig) stub(name, body string) {
	r.t.Helper()
	must(r.t, os.WriteFile(filepath.Join(r.bin, name), []byte("#!/bin/sh\n"+body+"\n"), 0o755))
}

func (r *buildRig) without(name string) { must(r.t, os.Remove(filepath.Join(r.bin, name))) }

// run runs build.sh and returns what it said (to either output) and whether it succeeded.
func (r *buildRig) run(env ...string) (string, bool) {
	r.t.Helper()
	cmd := exec.Command("/bin/sh", filepath.Join(r.dir, "build.sh"))
	cmd.Dir = r.dir
	cmd.Env = append([]string{"PATH=" + r.bin, "HOME=" + r.dir, "CALLS=" + r.calls}, env...)
	out, err := cmd.CombinedOutput()
	return string(out), err == nil
}

func (r *buildRig) called() string {
	b, _ := os.ReadFile(r.calls)
	return string(b)
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func TestBuildScriptBuildsThePageThenTheProgram(t *testing.T) {
	r := newBuildRig(t)
	out, ok := r.run()
	if !ok || !strings.Contains(out, "Done: ./yagi") || !strings.Contains(out, "Go 1.27.3, Node 24.1.0") {
		t.Fatalf("ok=%v\n%s", ok, out)
	}
	if got := r.called(); got != "npm ci\nnpm run build\ngo build -o yagi ./cmd/yagi\n" {
		t.Errorf("calls = %q", got)
	}
	if _, err := os.Stat(filepath.Join(r.dir, "yagi")); err != nil {
		t.Error("no program was built")
	}
}

func TestBuildScriptSkipsInstallingPackagesThatAreThereAlready(t *testing.T) {
	r := newBuildRig(t)
	r.run() // installs
	os.Remove(r.calls)
	out, ok := r.run()
	if !ok || !strings.Contains(out, "installed already") {
		t.Fatalf("ok=%v\n%s", ok, out)
	}
	if got := r.called(); strings.Contains(got, "npm ci") {
		t.Errorf("installed again: %q", got)
	}
	// A newer lockfile means the packages are out of date.
	lock := filepath.Join(r.dir, "web", "package-lock.json")
	future := time.Now().Add(time.Hour)
	must(t, os.Chtimes(lock, future, future))
	os.Remove(r.calls)
	if out, ok := r.run(); !ok || !strings.Contains(r.called(), "npm ci") {
		t.Errorf("a changed lockfile didn't reinstall: ok=%v\n%s\n%s", ok, out, r.called())
	}
}

func TestBuildScriptSaysPlainlyWhenNodeIsMissingOrTooOld(t *testing.T) {
	for name, c := range map[string]struct {
		node string // "" removes node
		want []string
	}{
		"missing":        {"", []string{"Node.js isn't installed", "22.12", "nvm", "docker run ghcr.io/johnschult/yagi"}},
		"node 18":        {"18.19.1", []string{"Node 18.19.1 is too old", "22.12 or newer", "nvm install 22"}},
		"just below":     {"22.11.9", []string{"Node 22.11.9 is too old"}},
		"node 20":        {"20.18.0", []string{"too old"}},
		"a long version": {"9.99.1", []string{"too old"}},
	} {
		r := newBuildRig(t)
		if c.node == "" {
			r.without("node")
		}
		out, ok := r.run("NODE_V=" + c.node)
		if ok {
			t.Errorf("%s: it built anyway", name)
		}
		for _, w := range c.want {
			if !strings.Contains(out, w) {
				t.Errorf("%s: no %q in\n%s", name, w, out)
			}
		}
		if strings.Contains(r.called(), "npm") || strings.Contains(r.called(), "go build") {
			t.Errorf("%s: it went on to build: %q", name, r.called())
		}
	}
	for _, good := range []string{"22.12.0", "22.12", "22.13.1", "23.0.0", "24.22.1", "100.0.0"} {
		r := newBuildRig(t)
		if out, ok := r.run("NODE_V=" + good); !ok {
			t.Errorf("Node %s refused:\n%s", good, out)
		}
	}
}

func TestBuildScriptSaysPlainlyWhenGoIsMissingOrTooOld(t *testing.T) {
	r := newBuildRig(t)
	r.without("go")
	if out, ok := r.run(); ok || !strings.Contains(out, "Go isn't installed") || !strings.Contains(out, "https://go.dev/dl/") {
		t.Errorf("no go: ok=%v\n%s", ok, out)
	}
	r = newBuildRig(t)
	if out, ok := r.run("GO_V=1.20.5"); ok || !strings.Contains(out, "Go 1.20.5 is too old") || !strings.Contains(out, "1.21 or newer") {
		t.Errorf("go 1.20: ok=%v\n%s", ok, out)
	}
	// An older Go than go.mod asks for is fine while it can fetch the right one itself, and not when it can't.
	r = newBuildRig(t)
	if out, ok := r.run("GO_V=1.22.0"); !ok {
		t.Errorf("go 1.22 should fetch 1.27.3 itself:\n%s", out)
	}
	r = newBuildRig(t)
	if out, ok := r.run("GO_V=1.26.0", "GOTOOLCHAIN=local"); ok || !strings.Contains(out, "GOTOOLCHAIN=local") || !strings.Contains(out, "1.27.3") {
		t.Errorf("local toolchain: ok=%v\n%s", ok, out)
	}
}

func TestBuildScriptShowsWhatNpmSaidOnlyWhenItFails(t *testing.T) {
	r := newBuildRig(t)
	out, ok := r.run("NPM_CI_FAILS=1")
	if ok || !strings.Contains(out, "npm error code EACCES") || !strings.Contains(out, "sudo chown") || !strings.Contains(out, "~/.npm") {
		t.Errorf("ok=%v\n%s", ok, out)
	}
	if strings.Contains(r.called(), "go build") {
		t.Error("it built the program after the page failed")
	}
	r = newBuildRig(t)
	if out, _ := r.run(); strings.Contains(out, "npm ci") && !strings.Contains(out, "Installing") {
		t.Errorf("npm's own output was shown on success:\n%s", out)
	}
}

func TestBuildScriptHelpSaysWhatItDoes(t *testing.T) {
	r := newBuildRig(t)
	cmd := exec.Command("/bin/sh", filepath.Join(r.dir, "build.sh"), "-h")
	cmd.Env = []string{"PATH=" + r.bin}
	out, err := cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(out), "Node 22.12 or newer") || strings.Contains(string(out), "npm ci") && strings.Contains(r.called(), "npm") {
		t.Errorf("err=%v\n%s", err, out)
	}
}
