package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/johnschult/antennarr/internal/hdhr"
	"github.com/johnschult/antennarr/internal/lineup"
	"github.com/johnschult/antennarr/internal/tuner"
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

// freeTuners are the numbers of the tuner's tuners with nothing on them, highest first: a new stream is
// given the lowest free one, so a test keeps out of its way by taking the highest.
func freeTuners(slots []tuner.Slot) []int {
	var out []int
	for _, sl := range slots {
		n, err := strconv.Atoi(strings.TrimPrefix(sl.Resource, "tuner"))
		if err == nil && !sl.InUse && strings.HasPrefix(sl.Resource, "tuner") {
			out = append(out, n)
		}
	}
	slices.SortFunc(out, func(a, b int) int { return b - a })
	return out
}

// testSignal tunes a free tuner to a channel for a few seconds and says how well it is received. It never
// interrupts anything: it needs a tuner with nothing on it (and checks that again at the tuner, which locks
// it with a key of its own for the test), and it gives the tuner back however it ends. One test at a time
// runs on a tuner, and none while it scans, restarts or installs.
func (a *API) testSignal(w http.ResponseWriter, r *http.Request) {
	number := r.PathValue("number")
	if !lineup.NumberRe.MatchString(number) {
		writeErr(w, http.StatusBadRequest, "that isn't a channel number")
		return
	}
	d, base, ok := a.tuner(w, r)
	if !ok {
		return
	}
	if a.Control == nil {
		writeErr(w, http.StatusConflict, "testing a signal is not available")
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
	u, err := url.Parse(base)
	if err != nil || u.Hostname() == "" {
		writeErr(w, http.StatusConflict, "Antennarr doesn't know where the tuner is")
		return
	}
	if _, running := a.testing.LoadOrStore(d.Name, true); running {
		writeErr(w, http.StatusConflict, "a signal test is already running on this tuner")
		return
	}
	defer a.testing.Delete(d.Name)

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	scan, err := a.Lineup.Scan(ctx, base)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	if scan.InProgress {
		writeErr(w, http.StatusConflict, "the tuner is scanning for channels: wait for that to finish")
		return
	}
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
		writeErr(w, http.StatusConflict, fmt.Sprintf("channel %s can't be tested: the tuner has it but isn't able to play it", number))
		return
	}
	free := freeTuners(d.Mon.Snapshot().Slots)
	if len(free) == 0 {
		writeErr(w, http.StatusConflict, "all of the tuner's tuners are in use: a test needs a free one, and never interrupts anything")
		return
	}

	dctx, dcancel := context.WithTimeout(ctx, hdhr.DialTimeout*2)
	conn, err := a.Control(dctx, u.Hostname())
	dcancel()
	if err != nil {
		a.Log.Warn("couldn't reach the tuner to test a signal", "device", d.Name, "err", err)
		writeErr(w, http.StatusBadGateway, "couldn't reach the tuner's control port")
		return
	}
	defer conn.Close()
	for _, n := range free {
		sig, err := conn.TestSignal(ctx, n, number, a.SignalTest)
		switch {
		case errors.Is(err, hdhr.ErrTunerBusy):
			continue // it turned out not to be free: try the next
		case err != nil:
			a.Log.Warn("couldn't test a signal", "device", d.Name, "channel", number, "err", err)
			writeErr(w, http.StatusBadGateway, err.Error())
			return
		}
		a.Log.Info("signal tested", "device", d.Name, "channel", number, "tuner", n, "locked", sig.Locked, "strength", sig.Strength, "quality", sig.Quality)
		writeJSON(w, http.StatusOK, sig)
		return
	}
	writeErr(w, http.StatusConflict, "every tuner that looked free turned out to be in use: try again in a moment")
}
