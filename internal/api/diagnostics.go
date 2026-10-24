package api

import (
	"net/http"
	"time"

	"github.com/johnschult/yagi/internal/diag"
)

// DiagnosticsDoc is whether diagnostics mode is on. Until is when it turns itself off.
type DiagnosticsDoc struct {
	On    bool       `json:"on"`
	Until *time.Time `json:"until,omitempty"`
}

func (a *API) diagnostics() DiagnosticsDoc {
	if u := a.Diag.Until(); !u.IsZero() {
		return DiagnosticsDoc{On: true, Until: &u}
	}
	return DiagnosticsDoc{}
}

func (a *API) getDiagnostics(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, a.diagnostics())
}

// diagnosticsInput turns diagnostics mode on or off. Minutes says for how long when turning it on; leaving it out
// is an hour, and it is never more than a day.
type diagnosticsInput struct {
	On      bool `json:"on"`
	Minutes int  `json:"minutes"`
}

func (a *API) putDiagnostics(w http.ResponseWriter, r *http.Request) {
	var in diagnosticsInput
	if !decode(w, r, &in) {
		return
	}
	if a.Diag == nil {
		writeErr(w, http.StatusConflict, "diagnostics mode is not available")
		return
	}
	if in.Minutes < 0 || time.Duration(in.Minutes)*time.Minute > diag.MaxFor {
		writeErr(w, http.StatusBadRequest, "minutes must be from 1 to 1440")
		return
	}
	if in.On {
		a.Diag.Enable(time.Duration(in.Minutes) * time.Minute)
	} else {
		a.Diag.Disable()
	}
	writeJSON(w, http.StatusOK, a.diagnostics())
}
