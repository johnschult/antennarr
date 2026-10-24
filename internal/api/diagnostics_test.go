package api

import (
	"testing"

	"github.com/johnschult/yagi/internal/diag"
)

func TestDiagnosticsIsOffUntilTurnedOnAndShowsInTheState(t *testing.T) {
	e := newEnv(t, johnAndMom, true)
	e.api.Diag = &diag.Mode{}
	t.Cleanup(e.api.Diag.Disable)
	_, v := e.call(t, "GET", "/api/diagnostics", "")
	if v.(map[string]any)["on"] != false {
		t.Errorf("on at the start: %v", v)
	}
	code, v := e.call(t, "PUT", "/api/diagnostics", `{"on":true,"minutes":30}`)
	if code != 200 || v.(map[string]any)["on"] != true || v.(map[string]any)["until"] == nil {
		t.Fatalf("%d %v", code, v)
	}
	_, st := e.call(t, "GET", "/api/state", "")
	if st.(map[string]any)["diagnostics"].(map[string]any)["on"] != true {
		t.Errorf("state = %v", st.(map[string]any)["diagnostics"])
	}
	code, v = e.call(t, "PUT", "/api/diagnostics", `{"on":false}`)
	if code != 200 || v.(map[string]any)["on"] != false {
		t.Errorf("%d %v", code, v)
	}
}

func TestDiagnosticsRefusals(t *testing.T) {
	e := newEnv(t, johnAndMom, true)
	if code, _ := e.call(t, "PUT", "/api/diagnostics", `{"on":true}`); code != 409 {
		t.Errorf("unavailable: %d", code)
	}
	e.api.Diag = &diag.Mode{}
	for _, body := range []string{`{"on":true,"minutes":-1}`, `{"on":true,"minutes":1441}`} {
		if code, _ := e.call(t, "PUT", "/api/diagnostics", body); code != 400 {
			t.Errorf("%s: %d", body, code)
		}
	}
}
