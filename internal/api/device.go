package api

import (
	"net/http"
)

// deviceInfo is what the tuner says about itself (the Device panel). It is read when asked, not polled, and
// remembered for a short while; ?refresh=1 reads again. Only a fixed list of values is ever read.
func (a *API) deviceInfo(w http.ResponseWriter, r *http.Request) {
	d := a.device(r.PathValue("name"))
	if d == nil {
		writeErr(w, http.StatusNotFound, "no such tuner")
		return
	}
	if a.Device == nil {
		writeErr(w, http.StatusConflict, "reading the tuner's details is not available")
		return
	}
	t := d.Mon.Snapshot()
	if !t.Online {
		writeErr(w, http.StatusConflict, "the tuner is offline")
		return
	}
	info, err := a.Device.Read(r.Context(), d.Name, t.BaseURL, r.URL.Query().Get("refresh") == "1")
	if err != nil {
		a.Log.Warn("couldn't read the tuner's details", "device", d.Name, "err", err)
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, info)
}
