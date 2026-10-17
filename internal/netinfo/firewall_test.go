package netinfo

import (
	"testing"
	"testing/fstest"
)

func machine(files map[string]string) fstest.MapFS {
	fsys := fstest.MapFS{}
	for name, content := range files {
		fsys[name] = &fstest.MapFile{Data: []byte(content)}
	}
	return fsys
}

func TestFindsUFWWhenItIsSwitchedOn(t *testing.T) {
	got := detectFirewall(machine(map[string]string{"etc/ufw/ufw.conf": "# comment\nENABLED=yes\nLOGLEVEL=low\n"}))
	if got != (Firewall{Tool: "ufw", Active: true}) {
		t.Errorf("got %+v", got)
	}
}

func TestUFWThatIsInstalledButOffIsNotAFirewall(t *testing.T) {
	for name, conf := range map[string]string{
		"off":       "ENABLED=no\n",
		"commented": "#ENABLED=yes\n",
		"empty":     "",
		"no key":    "LOGLEVEL=low\n",
	} {
		if got := detectFirewall(machine(map[string]string{"etc/ufw/ufw.conf": conf})); got != (Firewall{}) {
			t.Errorf("%s: got %+v", name, got)
		}
	}
}

func TestUFWSettingIsReadLeniently(t *testing.T) {
	for _, conf := range []string{"ENABLED=YES", "ENABLED = yes", `ENABLED="yes"`, "  ENABLED=yes  \r"} {
		if got := detectFirewall(machine(map[string]string{"etc/ufw/ufw.conf": conf})); got.Tool != "ufw" {
			t.Errorf("%q: got %+v", conf, got)
		}
	}
}

func TestFindsFirewalldWhenItsProcessIsAlive(t *testing.T) {
	got := detectFirewall(machine(map[string]string{"run/firewalld.pid": "812\n", "proc/812/stat": "x"}))
	if got != (Firewall{Tool: "firewalld", Active: true}) {
		t.Errorf("got %+v", got)
	}
}

func TestAStaleFirewalldPidFileDoesNotCount(t *testing.T) {
	for name, files := range map[string]map[string]string{
		"process gone": {"run/firewalld.pid": "812\n"},
		"garbage":      {"run/firewalld.pid": "not a pid", "proc/1/stat": "x"},
		"zero":         {"run/firewalld.pid": "0", "proc/0/stat": "x"},
		"negative":     {"run/firewalld.pid": "-3"},
		"empty":        {"run/firewalld.pid": ""},
	} {
		if got := detectFirewall(machine(files)); got != (Firewall{}) {
			t.Errorf("%s: got %+v", name, got)
		}
	}
}

func TestPlainIptablesIsReportedButNotAsActive(t *testing.T) {
	got := detectFirewall(machine(map[string]string{"proc/net/ip_tables_names": "nat\nfilter\n"}))
	if got != (Firewall{Tool: "iptables", Active: false}) {
		t.Errorf("got %+v (its rules can't be read, so it can't claim to be blocking anything)", got)
	}
	if got := detectFirewall(machine(map[string]string{"proc/net/ip_tables_names": "nat\n"})); got != (Firewall{}) {
		t.Errorf("no filter table: got %+v", got)
	}
}

func TestUFWAndFirewalldWinOverTheIptablesTheyManage(t *testing.T) {
	both := map[string]string{"proc/net/ip_tables_names": "filter\n", "run/firewalld.pid": "5", "proc/5/stat": "x"}
	if got := detectFirewall(machine(both)); got.Tool != "firewalld" || !got.Active {
		t.Errorf("firewalld over iptables: got %+v", got)
	}
	both["etc/ufw/ufw.conf"] = "ENABLED=yes"
	if got := detectFirewall(machine(both)); got.Tool != "ufw" {
		t.Errorf("ufw over the rest: got %+v", got)
	}
}

func TestNothingFoundOnABareMachine(t *testing.T) {
	if got := detectFirewall(machine(nil)); got != (Firewall{}) {
		t.Errorf("got %+v", got)
	}
}

func TestDetectFirewallRunsAgainstThisMachine(t *testing.T) {
	got := DetectFirewall() // whatever is here, it must be well formed
	switch got.Tool {
	case "", "ufw", "firewalld", "iptables":
	default:
		t.Errorf("unknown tool %q", got.Tool)
	}
	if got.Tool == "" && got.Active {
		t.Error("active with no tool")
	}
}
