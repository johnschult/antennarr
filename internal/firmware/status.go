package firmware

import (
	"regexp"
	"sort"
	"time"
)

// States a tuner's firmware can be in.
const (
	StateUpToDate        = "up-to-date"
	StateUpdateAvailable = "update-available"
	StatePossibleUpdate  = "possible-update" // newer releases exist, but it's unclear they apply
	StateUnknown         = "unknown"
	StateDisabled        = "disabled" // the check is turned off
)

// Update is a release newer than the tuner's firmware that applies to it.
type Update struct {
	Version string `json:"version"`
	// Certain is true when a note in the release is plainly for this model (or
	// for all of them); false when it only might be.
	Certain bool   `json:"certain"`
	Notes   []Note `json:"notes"`
}

// Status is what we can say about one tuner's firmware.
type Status struct {
	State   string `json:"state"`
	Current string `json:"current"`
	// Latest is the newest release that applies, or Current if none does.
	Latest string `json:"latest"`
	// DeviceReported is the version the tuner itself says is available, if it
	// says so. When it does, that is authoritative.
	DeviceReported string `json:"deviceReported,omitempty"`
	// Updates lists the applicable newer releases, newest first.
	Updates []Update `json:"updates"`
	// ModelRecognized is false when we couldn't place the model number, so the
	// scoping of notes is a guess.
	ModelRecognized bool      `json:"modelRecognized"`
	CheckedAt       time.Time `json:"checkedAt"`
	// Error says why the state is unknown: the page couldn't be read, or the
	// check is off.
	Error string `json:"error,omitempty"`
}

var versionRe = regexp.MustCompile(`^\d{8}$`)

// Evaluate compares a tuner with the releases. releases may be nil (nothing
// fetched yet), in which case only the tuner's own report can say anything.
func Evaluate(releases []Release, d Device) Status {
	st := Status{State: StateUnknown, Current: d.Version, Latest: d.Version, DeviceReported: d.UpgradeAvailable, Updates: []Update{}}
	p := newProfile(d)
	st.ModelRecognized = p.known

	if versionRe.MatchString(d.Version) {
		for _, r := range releases {
			if r.Version <= d.Version {
				continue
			}
			var notes []Note
			certain := false
			for _, n := range r.Notes {
				switch p.appliesTo(n) {
				case yes:
					notes, certain = append(notes, n), true
				case maybe:
					notes = append(notes, n)
				}
			}
			if len(notes) > 0 {
				st.Updates = append(st.Updates, Update{Version: r.Version, Certain: certain, Notes: notes})
			}
		}
	}

	// Newest first, whatever order the page listed them in.
	sort.Slice(st.Updates, func(i, j int) bool { return st.Updates[i].Version > st.Updates[j].Version })

	hasCertain, hasMaybe := false, false
	for _, u := range st.Updates {
		hasCertain = hasCertain || u.Certain
		hasMaybe = hasMaybe || !u.Certain
	}
	if len(st.Updates) > 0 {
		st.Latest = st.Updates[0].Version
	}

	switch {
	case d.UpgradeAvailable != "":
		st.State = StateUpdateAvailable
		if d.UpgradeAvailable > st.Latest {
			st.Latest = d.UpgradeAvailable
		}
	case hasCertain:
		st.State = StateUpdateAvailable
	case hasMaybe:
		st.State = StatePossibleUpdate
	case releases != nil && versionRe.MatchString(d.Version):
		st.State = StateUpToDate
	}
	return st
}
