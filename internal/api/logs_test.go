package api

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/johnschult/antennarr/internal/logbuf"
)

const logsPath = "/antennarr/api/logs"

// withLogs gives the API a log with some lines in it.
func withLogs(t *testing.T) (*env, *logbuf.Buffer) {
	t.Helper()
	e := newEnv(t, johnAndMom, true)
	b := logbuf.New(100)
	l := slog.New(logbuf.Tee(slog.NewTextHandler(&bytes.Buffer{}, &slog.HandlerOptions{Level: slog.LevelDebug}), b))
	l.Debug("detail")
	l.Info("tuner online", "device", "antenna")
	l.Warn("no viewers configured")
	l.Error("couldn't read the guide", "err", "boom")
	e.api.Logs = b
	return e, b
}

func entries(t *testing.T, v any) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, x := range obj(t, v)["entries"].([]any) {
		out = append(out, obj(t, x))
	}
	return out
}

func TestLogsAreListedOldestFirstWithWhatIsNeededToAskForMore(t *testing.T) {
	e, _ := withLogs(t)
	code, v := e.call(t, "GET", logsPath, "")
	d := obj(t, v)
	es := entries(t, v)
	if code != 200 || len(es) != 4 || es[0]["message"] != "detail" || es[3]["level"] != "error" {
		t.Fatalf("%d %v", code, v)
	}
	if d["latest"] != float64(4) || d["kept"] != float64(4) || d["capacity"] != float64(100) || d["level"] != "info" {
		t.Errorf("doc = %v", d)
	}
	attrs := es[1]["attrs"].([]any)
	if len(attrs) != 1 || obj(t, attrs[0])["key"] != "device" || obj(t, attrs[0])["value"] != "antenna" {
		t.Errorf("attrs = %v", es[1]["attrs"])
	}
}

func TestLogsCanBeNarrowedByLevelTextSinceAndLimit(t *testing.T) {
	e, _ := withLogs(t)
	_, v := e.call(t, "GET", logsPath+"?level=warn", "")
	if es := entries(t, v); len(es) != 2 {
		t.Errorf("warn and up: %v", es)
	}
	_, v = e.call(t, "GET", logsPath+"?q=ANTENNA", "")
	if es := entries(t, v); len(es) != 1 || es[0]["message"] != "tuner online" {
		t.Errorf("text: %v", es)
	}
	_, v = e.call(t, "GET", logsPath+"?since=3", "")
	if es := entries(t, v); len(es) != 1 || es[0]["level"] != "error" {
		t.Errorf("since: %v", es)
	}
	_, v = e.call(t, "GET", logsPath+"?limit=2", "")
	if es := entries(t, v); len(es) != 2 || es[0]["level"] != "warn" {
		t.Errorf("the newest two: %v", es)
	}
	_, v = e.call(t, "GET", logsPath+"?since=4", "")
	if es := entries(t, v); len(es) != 0 || obj(t, v)["latest"] != float64(4) || obj(t, v)["entries"] == nil {
		t.Errorf("nothing newer: %v", v)
	}
}

func TestLogQueriesAreChecked(t *testing.T) {
	e, _ := withLogs(t)
	for _, q := range []string{"?level=trace", "?since=-1", "?since=x", "?limit=0", "?limit=2001", "?limit=x", "?q=" + strings.Repeat("a", 201)} {
		if code, _ := e.call(t, "GET", logsPath+q, ""); code != http.StatusBadRequest {
			t.Errorf("%s: %d, want 400", q, code)
		}
	}
}

func TestLogsCanBeClearedAndAreAdminOnly(t *testing.T) {
	e, b := withLogs(t)
	if code, _ := e.call(t, "DELETE", logsPath, ""); code != 204 {
		t.Errorf("clear: %d", code)
	}
	if r := b.Find(logbuf.Query{}); len(r.Entries) != 0 {
		t.Errorf("not cleared: %v", r.Entries)
	}
	if code, _ := e.call(t, "POST", logsPath, ""); code != 405 {
		t.Errorf("POST: %d", code)
	}
	// someone who is not an admin gets nothing
	for _, m := range []string{"GET", "DELETE"} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(m, logsPath, nil)
		req.RemoteAddr = "100.67.117.32:1" // John is a viewer, not an admin
		e.pol.AdminOnly(e.api.Handler()).ServeHTTP(rec, req)
		if rec.Code != 403 {
			t.Errorf("%s as a viewer: %d", m, rec.Code)
		}
	}
	if r := b.Find(logbuf.Query{}); len(r.Entries) != 0 {
		t.Error("a viewer's DELETE had an effect")
	}
}

func TestNoLogIsNotFound(t *testing.T) {
	e := newEnv(t, johnAndMom, true)
	if code, _ := e.call(t, "GET", logsPath, ""); code != 404 {
		t.Errorf("%d", code)
	}
}

func TestTheLogLevelIsASettingThatIsCheckedAndNotWrittenWhenInfo(t *testing.T) {
	e := newEnv(t, johnAndMom, true)
	_, v := e.call(t, "GET", "/antennarr/api/settings", "")
	if obj(t, v)["logLevel"] != "info" {
		t.Errorf("default = %v", obj(t, v)["logLevel"])
	}
	if code, v := e.call(t, "PATCH", "/antennarr/api/settings", `{"logLevel":"debug"}`); code != 200 || obj(t, v)["logLevel"] != "debug" {
		t.Fatalf("%d %v", code, v)
	}
	if fileSettings(t, e).LogLevel != "debug" {
		t.Error("not saved")
	}
	e.call(t, "PATCH", "/antennarr/api/settings", `{"logLevel":"info"}`)
	if fileSettings(t, e).LogLevel != "" {
		t.Errorf("the default was written: %q", fileSettings(t, e).LogLevel)
	}
	if code, _ := e.call(t, "PATCH", "/antennarr/api/settings", `{"logLevel":"trace"}`); code != 400 {
		t.Errorf("a bad level: %d", code)
	}
	e.call(t, "PATCH", "/antennarr/api/settings", `{"sharedCap":2}`)
	e.call(t, "PATCH", "/antennarr/api/settings", `{"logLevel":"warn"}`)
	e.call(t, "PATCH", "/antennarr/api/settings", `{"sharedCap":3}`)
	if fileSettings(t, e).LogLevel != "warn" {
		t.Error("an unrelated patch changed the level")
	}
}
