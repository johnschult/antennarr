package api

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"github.com/johnschult/yagi/internal/firmware"
	"github.com/johnschult/yagi/internal/upgrade"
)

// upgradeInput is a request to install firmware. Version must be the release the page was showing: if a
// newer one has been found since, the request is refused and the person looks again, so no one installs
// something they haven't seen offered. Interrupt says the person has been told that things are running on
// the tuner and that the install will cut them off.
type upgradeInput struct {
	Version   string `json:"version"`
	Interrupt bool   `json:"interrupt"`
}

// installFirmware starts installing the latest firmware on a tuner, in the background. Everything that
// makes it safe is checked here, on the server, whatever the page did: the page is only a convenience.
func (a *API) installFirmware(w http.ResponseWriter, r *http.Request) {
	var in upgradeInput
	if !decode(w, r, &in) {
		return
	}
	d := a.device(r.PathValue("name"))
	if d == nil {
		writeErr(w, http.StatusNotFound, "no such tuner")
		return
	}
	if a.Upgrade == nil || a.Firmware == nil {
		writeErr(w, http.StatusConflict, "installing firmware is not available")
		return
	}
	if !a.Pol.Settings().FirmwareCheckEnabled() {
		writeErr(w, http.StatusConflict, "turn on “Check for new tuner firmware” first: Yagi only installs what that check says is newer")
		return
	}
	if a.Upgrade.Active(d.Name) {
		writeErr(w, http.StatusConflict, upgrade.ErrBusy.Error())
		return
	}

	t := d.Mon.Snapshot()
	if !t.Online {
		writeErr(w, http.StatusConflict, "the tuner is offline, so it can't be updated")
		return
	}
	status := a.Firmware.StatusFor(firmware.Device{Model: t.Model, FirmwareName: t.Firmware.Name, Version: t.Firmware.Version, UpgradeAvailable: t.Firmware.UpgradeAvailable})
	release, ok, why := firmware.Installable(status, t.Firmware.Name)
	if !ok {
		writeErr(w, http.StatusBadRequest, why)
		return
	}
	if in.Version != release {
		writeErr(w, http.StatusConflict, fmt.Sprintf("the newest firmware is %s now, not %q: look again before updating", release, in.Version))
		return
	}
	if n, _ := t.InUse(); n > 0 && !in.Interrupt {
		writeErr(w, http.StatusConflict, fmt.Sprintf("%d of the tuner's tuners %s in use, and updating cuts them off: confirm that you want to interrupt them", n, plural(n, "is", "are")))
		return
	}
	u, err := url.Parse(t.BaseURL)
	if err != nil || u.Hostname() == "" {
		writeErr(w, http.StatusConflict, "Yagi doesn't know where the tuner is")
		return
	}

	err = a.Upgrade.Start(upgrade.Request{
		Device: d.Name, Title: a.deviceTitle(d.Name, t.FriendlyName), Host: u.Hostname(),
		Family: t.Firmware.Name, Current: status.Current, Release: release, Tuner: d.Mon,
	})
	switch {
	case errors.Is(err, upgrade.ErrBusy):
		writeErr(w, http.StatusConflict, err.Error())
	case err != nil:
		a.Log.Error("firmware update could not start", "device", d.Name, "err", err)
		writeErr(w, http.StatusInternalServerError, "couldn't start the update")
	default:
		a.Log.Info("firmware update started", "device", d.Name, "from", status.Current, "to", release, "interrupting", in.Interrupt)
		writeJSON(w, http.StatusAccepted, map[string]any{"release": release, "from": status.Current})
	}
}

// dismissUpgrade clears a finished install's result, so the page stops showing it. One that is still
// running is left alone.
func (a *API) dismissUpgrade(w http.ResponseWriter, r *http.Request) {
	d := a.device(r.PathValue("name"))
	if d == nil {
		writeErr(w, http.StatusNotFound, "no such tuner")
		return
	}
	if a.Upgrade == nil {
		writeErr(w, http.StatusConflict, "installing firmware is not available")
		return
	}
	if a.Upgrade.Active(d.Name) {
		writeErr(w, http.StatusConflict, "the update is still running")
		return
	}
	a.Upgrade.Forget(d.Name)
	w.WriteHeader(http.StatusNoContent)
}

// deviceTitle is what a tuner is called to a person: the name the owner gave it, or the model it reports.
func (a *API) deviceTitle(name, model string) string {
	if l := a.Pol.Label(name); l != "" {
		return l
	}
	if model != "" {
		return model
	}
	return name
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
