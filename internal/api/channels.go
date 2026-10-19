package api

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"time"

	"github.com/johnschult/antennarr/internal/lineup"
)

// channelsDoc is a tuner's channels and its scan, as the tuner says them.
type channelsDoc struct {
	Channels []lineup.Channel `json:"channels"`
	Scan     lineup.Scan      `json:"scan"`
}

// tuner is where to reach the web side of an online tuner, or writes why not and returns false.
func (a *API) tuner(w http.ResponseWriter, r *http.Request) (*Device, string, bool) {
	d := a.device(r.PathValue("name"))
	if d == nil {
		writeErr(w, http.StatusNotFound, "no such tuner")
		return nil, "", false
	}
	if a.Lineup == nil {
		writeErr(w, http.StatusConflict, "managing channels is not available")
		return nil, "", false
	}
	t := d.Mon.Snapshot()
	if !t.Online {
		writeErr(w, http.StatusConflict, "the tuner is offline")
		return nil, "", false
	}
	if t.BaseURL == "" {
		writeErr(w, http.StatusConflict, "Antennarr doesn't know where the tuner is")
		return nil, "", false
	}
	return d, t.BaseURL, true
}

func (a *API) tunerCtx(r *http.Request) (context.Context, context.CancelFunc) {
	return context.WithTimeout(r.Context(), 15*time.Second)
}

// getChannels lists every channel the tuner has found, and how its scan stands. It reads the tuner each time:
// the page asks again while a scan runs.
func (a *API) getChannels(w http.ResponseWriter, r *http.Request) {
	d, base, ok := a.tuner(w, r)
	if !ok {
		return
	}
	ctx, cancel := a.tunerCtx(r)
	defer cancel()
	chans, err := a.Lineup.Found(ctx, base)
	if err != nil {
		a.Log.Warn("couldn't read the tuner's channels", "device", d.Name, "err", err)
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	scan, err := a.Lineup.Scan(ctx, base)
	if err != nil {
		a.Log.Warn("couldn't read the tuner's scan status", "device", d.Name, "err", err)
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, channelsDoc{Channels: chans, Scan: scan})
}

// lineupStateInput is a channel's new state.
type lineupStateInput struct {
	State lineup.State `json:"state"`
}

// setChannel enables, disables or favorites one channel. Only a channel the tuner itself lists is touched, and
// the change is checked by asking the tuner again.
func (a *API) setChannel(w http.ResponseWriter, r *http.Request) {
	var in lineupStateInput
	if !decode(w, r, &in) {
		return
	}
	number := r.PathValue("number")
	if !lineup.NumberRe.MatchString(number) {
		writeErr(w, http.StatusBadRequest, "that isn't a channel number")
		return
	}
	if !in.State.Valid() {
		writeErr(w, http.StatusBadRequest, "state must be enabled, favorite or disabled")
		return
	}
	d, base, ok := a.tuner(w, r)
	if !ok {
		return
	}
	ctx, cancel := a.tunerCtx(r)
	defer cancel()
	chans, err := a.Lineup.Found(ctx, base)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	i := slices.IndexFunc(chans, func(c lineup.Channel) bool { return c.Number == number })
	if i < 0 {
		writeErr(w, http.StatusNotFound, fmt.Sprintf("the tuner has no channel %s", number))
		return
	}
	if !chans[i].Subscribed {
		writeErr(w, http.StatusConflict, fmt.Sprintf("channel %s can't be changed: the tuner has it but isn't able to play it", number))
		return
	}
	if err := a.Lineup.SetState(ctx, base, number, in.State); err != nil {
		a.Log.Warn("couldn't change a channel", "device", d.Name, "channel", number, "err", err)
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	// Ask again, so what is shown is what the tuner holds.
	after, err := a.Lineup.Found(ctx, base)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	j := slices.IndexFunc(after, func(c lineup.Channel) bool { return c.Number == number })
	if j < 0 || after[j].State != in.State {
		writeErr(w, http.StatusBadGateway, "the tuner didn't change it")
		return
	}
	a.Log.Info("channel changed", "device", d.Name, "channel", number, "state", string(in.State))
	writeJSON(w, http.StatusOK, after[j])
}

// scanInput is a request to scan for channels. Interrupt says the person has been told that a scan takes the
// tuners, and that whatever is running on them is cut off.
type scanInput struct {
	Source    string `json:"source"`
	Interrupt bool   `json:"interrupt"`
}

// startScan begins a channel scan. A scan takes the tuners for as long as it runs, so what is in use is
// refused unless the request says it may be interrupted, as for a restart.
func (a *API) startScan(w http.ResponseWriter, r *http.Request) {
	var in scanInput
	if !decode(w, r, &in) {
		return
	}
	d, base, ok := a.tuner(w, r)
	if !ok {
		return
	}
	if a.Restart != nil && a.Restart.Active(d.Name) {
		writeErr(w, http.StatusConflict, "the tuner is restarting: wait for that to finish")
		return
	}
	if a.Upgrade != nil && a.Upgrade.Active(d.Name) {
		writeErr(w, http.StatusConflict, "the tuner is being updated: wait for that to finish")
		return
	}
	ctx, cancel := a.tunerCtx(r)
	defer cancel()
	scan, err := a.Lineup.Scan(ctx, base)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	if scan.InProgress {
		writeErr(w, http.StatusConflict, "a scan is already running")
		return
	}
	if !scan.Possible {
		writeErr(w, http.StatusConflict, "the tuner can't scan for channels right now")
		return
	}
	source := in.Source
	if source == "" {
		source = scan.Source
	}
	if len(scan.Sources) > 0 && !slices.Contains(scan.Sources, source) {
		writeErr(w, http.StatusBadRequest, fmt.Sprintf("the tuner can scan %v, not %q", scan.Sources, source))
		return
	}
	if n, _ := d.Mon.Snapshot().InUse(); n > 0 && !in.Interrupt {
		writeErr(w, http.StatusConflict, fmt.Sprintf("%d of the tuner's tuners %s in use, and a scan cuts them off: confirm that you want to interrupt them", n, plural(n, "is", "are")))
		return
	}
	if err := a.Lineup.StartScan(ctx, base, source); err != nil {
		a.Log.Warn("couldn't start a channel scan", "device", d.Name, "err", err)
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	a.Log.Info("channel scan started", "device", d.Name, "source", source, "interrupting", in.Interrupt)
	d.Mon.Note("info", fmt.Sprintf("Scanning %s for channels", a.deviceTitle(d.Name, d.Mon.Snapshot().FriendlyName)))
	writeJSON(w, http.StatusAccepted, map[string]any{"scanning": true, "source": source})
}

// stopScan ends a scan that is running.
func (a *API) stopScan(w http.ResponseWriter, r *http.Request) {
	d, base, ok := a.tuner(w, r)
	if !ok {
		return
	}
	ctx, cancel := a.tunerCtx(r)
	defer cancel()
	if err := a.Lineup.AbortScan(ctx, base); err != nil {
		a.Log.Warn("couldn't stop the channel scan", "device", d.Name, "err", err)
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
