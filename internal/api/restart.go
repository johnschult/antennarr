package api

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"github.com/johnschult/yagi/internal/restart"
)

// restartInput is a request to restart a tuner. Interrupt says the person has been told that things are
// running on it and that restarting cuts them off.
type restartInput struct {
	Interrupt bool `json:"interrupt"`
}

// restartTuner restarts a tuner, in the background. Everything that makes it safe is checked here, on the
// server, whatever the page did: the page is only a convenience.
func (a *API) restartTuner(w http.ResponseWriter, r *http.Request) {
	var in restartInput
	if !decode(w, r, &in) {
		return
	}
	d := a.device(r.PathValue("name"))
	if d == nil {
		writeErr(w, http.StatusNotFound, "no such tuner")
		return
	}
	if a.Restart == nil {
		writeErr(w, http.StatusConflict, "restarting the tuner is not available")
		return
	}
	if a.Restart.Active(d.Name) {
		writeErr(w, http.StatusConflict, restart.ErrBusy.Error())
		return
	}
	if a.Upgrade != nil && a.Upgrade.Active(d.Name) {
		writeErr(w, http.StatusConflict, "the tuner is being updated: wait for that to finish")
		return
	}
	t := d.Mon.Snapshot()
	if !t.Online {
		writeErr(w, http.StatusConflict, "the tuner is offline, so it can't be restarted from here")
		return
	}
	if n, _ := t.InUse(); n > 0 && !in.Interrupt {
		writeErr(w, http.StatusConflict, fmt.Sprintf("%d of the tuner's tuners %s in use, and restarting cuts them off: confirm that you want to interrupt them", n, plural(n, "is", "are")))
		return
	}
	u, err := url.Parse(t.BaseURL)
	if err != nil || u.Hostname() == "" {
		writeErr(w, http.StatusConflict, "Yagi doesn't know where the tuner is")
		return
	}
	err = a.Restart.Start(restart.Request{Device: d.Name, Title: a.deviceTitle(d.Name, t.FriendlyName), Host: u.Hostname(), Tuner: d.Mon})
	switch {
	case errors.Is(err, restart.ErrBusy):
		writeErr(w, http.StatusConflict, err.Error())
	case err != nil:
		a.Log.Error("tuner restart could not start", "device", d.Name, "err", err)
		writeErr(w, http.StatusInternalServerError, "couldn't start the restart")
	default:
		a.Log.Info("tuner restart started", "device", d.Name, "interrupting", in.Interrupt)
		writeJSON(w, http.StatusAccepted, map[string]any{"restarting": true})
	}
}

// dismissRestart clears a finished restart's result, so the page stops showing it. One that is still
// running is left alone.
func (a *API) dismissRestart(w http.ResponseWriter, r *http.Request) {
	d := a.device(r.PathValue("name"))
	if d == nil {
		writeErr(w, http.StatusNotFound, "no such tuner")
		return
	}
	if a.Restart == nil {
		writeErr(w, http.StatusConflict, "restarting the tuner is not available")
		return
	}
	if a.Restart.Active(d.Name) {
		writeErr(w, http.StatusConflict, "the tuner is still restarting")
		return
	}
	a.Restart.Forget(d.Name)
	w.WriteHeader(http.StatusNoContent)
}
