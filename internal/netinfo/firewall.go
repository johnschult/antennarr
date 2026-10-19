package netinfo

import (
	"io/fs"
	"os"
	"runtime"
	"strconv"
	"strings"
)

// Firewall says which firewall, if any, looks to be in use on this machine.
//
// Yagi runs as an ordinary user, so it can't read the rules (that takes
// root) and it can't test whether a port is blocked either: a connection from
// this machine to itself never passes through the firewall. What it can do is
// notice which firewall is switched on, from the small world-readable files
// each one leaves behind, so the setup guide can offer the right commands.
type Firewall struct {
	// Tool is "ufw", "firewalld" or "iptables"; empty when none was found.
	Tool string `json:"tool"`
	// Active is true when the tool was seen to be switched on. For iptables it
	// is false: the kernel part being loaded doesn't mean any rules are set.
	Active bool `json:"active"`
}

// DetectFirewall looks at this machine. Only Linux is looked at; elsewhere
// (macOS, Windows) it finds nothing rather than guess.
func DetectFirewall() Firewall {
	if runtime.GOOS != "linux" {
		return Firewall{}
	}
	return detectFirewall(os.DirFS("/"))
}

// detectFirewall reads from fsys (paths without a leading slash), so tests can
// supply a made-up machine.
func detectFirewall(fsys fs.FS) Firewall {
	switch {
	case ufwEnabled(fsys):
		return Firewall{Tool: "ufw", Active: true}
	case firewalldRunning(fsys):
		return Firewall{Tool: "firewalld", Active: true}
	case iptablesLoaded(fsys):
		return Firewall{Tool: "iptables"}
	}
	return Firewall{}
}

// ufwEnabled reads ufw's own switch, /etc/ufw/ufw.conf: ENABLED=yes.
func ufwEnabled(fsys fs.FS) bool {
	b, err := fs.ReadFile(fsys, "etc/ufw/ufw.conf")
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if ok && strings.TrimSpace(k) == "ENABLED" {
			return strings.EqualFold(strings.Trim(strings.TrimSpace(v), `"'`), "yes")
		}
	}
	return false
}

// firewalldRunning checks firewalld's pid file against the running processes,
// so a pid file left behind by a crash doesn't count.
func firewalldRunning(fsys fs.FS) bool {
	b, err := fs.ReadFile(fsys, "run/firewalld.pid")
	if err != nil {
		return false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil || pid <= 0 {
		return false
	}
	_, err = fs.Stat(fsys, "proc/"+strconv.Itoa(pid))
	return err == nil
}

// iptablesLoaded sees whether the kernel's iptables filter table exists. Rules
// can't be read without root, so this only says iptables is available.
func iptablesLoaded(fsys fs.FS) bool {
	b, err := fs.ReadFile(fsys, "proc/net/ip_tables_names")
	if err != nil {
		return false
	}
	for _, name := range strings.Fields(string(b)) {
		if name == "filter" {
			return true
		}
	}
	return false
}
