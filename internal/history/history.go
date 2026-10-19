// Package history remembers who watched what and for how long, for the Activity page. It watches the
// tuners' streams as the server sees them (a viewer's stream through Antennarr, the owner's own device
// straight from the tuner) and turns them into sessions: one tuner on one channel for one viewer, from when
// it began to when it stopped.
//
// Sessions are kept for thirty days in a file beside the settings, one JSON object per line, appended as
// each session ends. A session still going when Antennarr stops is closed at that moment; one lost to a
// crash is lost.
package history

import (
	"bufio"
	"encoding/json"
	"os"
	"sort"
	"sync"
	"time"
)

// Kind is whose stream a session was.
type Kind string

const (
	KindViewer Kind = "viewer" // a viewer, through Antennarr, whose name is known
	KindRemote Kind = "remote" // through Antennarr, but not matched to a viewer
	KindLocal  Kind = "local"  // the owner's own device, straight from the tuner
)

// Session is one tuner on one channel for one viewer.
type Session struct {
	Device      string    `json:"device"`
	Tuner       string    `json:"tuner"` // tuner0, tuner1...
	Channel     string    `json:"channel"`
	ChannelName string    `json:"channelName,omitempty"`
	Kind        Kind      `json:"kind"`
	Viewer      string    `json:"viewer,omitempty"` // for a viewer
	Client      string    `json:"client,omitempty"` // for the owner's own device: its address on their network
	Start       time.Time `json:"start"`
	End         time.Time `json:"end"`
	// Titles are the programs that were on, in order, as the guide named them.
	Titles []string `json:"titles,omitempty"`
	// Bytes is about how much the tuner sent for it: the bitrate it reported, over the time it was
	// watched. It is an estimate (the tuner is asked every few seconds), and it is the tuner's output,
	// which for a viewer is what went out over the sharer's upload.
	Bytes int64 `json:"bytes,omitempty"`
	// Quality is the average signal quality, 0 to 100, over the times the tuner reported one. Absent when
	// it never did.
	Quality int `json:"quality,omitempty"`
	// Open is true for a session that has not ended yet (End is the last time it was seen).
	Open bool `json:"open,omitempty"`

	// what is needed to go on measuring it
	bps        int64 // the last bitrate seen
	qualitySum int
	qualityN   int
}

// Observation is what the server sees of one tuner that is in use right now.
type Observation struct {
	Device      string
	Tuner       string
	Channel     string
	ChannelName string
	Kind        Kind
	Viewer      string
	Client      string
	Title       string // what the guide says is on, if it says
	// BitsPerSecond and Quality are what the tuner reports for the stream now; zero means it doesn't say.
	BitsPerSecond int64
	Quality       int
}

const (
	// Keep is how long sessions are kept.
	Keep = 30 * 24 * time.Hour
	// MinSession is the shortest session worth keeping: flicking through channels isn't watching.
	MinSession = 15 * time.Second
	maxTitles  = 6
	maxTitle   = 120
	pruneEvery = time.Hour
	// maxGap is the longest time between two looks that is counted as watching: after a longer silence
	// (Antennarr was busy or stopped) the bytes in between are not guessed at.
	maxGap = time.Minute
)

// Recorder turns observations into sessions.
type Recorder struct {
	store *Store
	// OnError, if set, hears of a session that could not be written to the file. The session is still
	// kept in memory.
	OnError func(error)

	mu        sync.Mutex
	open      map[string]*Session
	done      []Session
	lastPrune time.Time
}

// New makes a Recorder. store may be nil: sessions are then only kept in memory. Sessions already in the
// store, still within Keep, are loaded.
func New(store *Store, now time.Time) *Recorder {
	r := &Recorder{store: store, open: map[string]*Session{}, lastPrune: now}
	if store != nil {
		sessions, dropped := store.Load(now.Add(-Keep))
		r.done = sessions
		if dropped > 0 {
			_ = store.Rewrite(r.done) // best effort: what is kept loads again next time, and the old lines are dropped again
		}
	}
	return r
}

func key(o Observation) string { return o.Device + "/" + o.Tuner }

func same(s *Session, o Observation) bool {
	return s.Channel == o.Channel && s.Kind == o.Kind && s.Viewer == o.Viewer && s.Client == o.Client
}

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// Observe says what is in use at now. Call it regularly (about as often as the tuners are polled). A tuner
// not listed is not in use; one listed with a different channel or viewer than before has a new session.
func (r *Recorder) Observe(now time.Time, obs []Observation) {
	r.mu.Lock()
	defer r.mu.Unlock()

	seen := map[string]bool{}
	for _, o := range obs {
		if o.Channel == "" {
			continue // something is on it but we can't say what: not a session worth keeping
		}
		k := key(o)
		seen[k] = true
		if s, ok := r.open[k]; ok && same(s, o) {
			if gap := now.Sub(s.End); gap > 0 && gap <= maxGap && o.BitsPerSecond > 0 {
				// the rate over the gap: the average of the last look and this one
				rate := o.BitsPerSecond
				if s.bps > 0 {
					rate = (s.bps + o.BitsPerSecond) / 2
				}
				s.Bytes += int64(float64(rate) / 8 * gap.Seconds())
			}
			s.End = now
			observeStream(s, o)
			continue
		} else if ok {
			r.closeLocked(k) // it changed hands or channels
		}
		s := &Session{Device: o.Device, Tuner: o.Tuner, Channel: o.Channel, ChannelName: o.ChannelName, Kind: o.Kind, Viewer: o.Viewer, Client: o.Client, Start: now, End: now}
		observeStream(s, o)
		r.open[k] = s
	}
	for k := range r.open {
		if !seen[k] {
			r.closeLocked(k)
		}
	}
	if now.Sub(r.lastPrune) >= pruneEvery {
		r.pruneLocked(now)
	}
}

// observeStream takes in what a look at the tuner says about a session: its name, the program on, the
// bitrate to measure the next stretch with, and the signal quality.
func observeStream(s *Session, o Observation) {
	if o.ChannelName != "" {
		s.ChannelName = o.ChannelName
	}
	addTitle(s, o.Title)
	s.bps = o.BitsPerSecond
	if o.Quality > 0 {
		s.qualitySum += min(o.Quality, 100)
		s.qualityN++
		s.Quality = (s.qualitySum + s.qualityN/2) / s.qualityN
	}
}

func addTitle(s *Session, title string) {
	title = clip(title, maxTitle)
	if title == "" || len(s.Titles) >= maxTitles {
		return
	}
	if n := len(s.Titles); n > 0 && s.Titles[n-1] == title {
		return
	}
	s.Titles = append(s.Titles, title)
}

// closeLocked ends the session on a tuner at the last time it was seen, and keeps it if it was long enough.
func (r *Recorder) closeLocked(k string) {
	s := r.open[k]
	delete(r.open, k)
	if s.End.Sub(s.Start) < MinSession {
		return
	}
	r.done = append(r.done, *s)
	if r.store != nil {
		if err := r.store.Append(*s); err != nil && r.OnError != nil {
			r.OnError(err)
		}
	}
}

// Close ends every session still going, at the last time each was seen: for when Antennarr stops.
func (r *Recorder) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for k := range r.open {
		r.closeLocked(k)
	}
}

func (r *Recorder) pruneLocked(now time.Time) {
	r.lastPrune = now
	cut := now.Add(-Keep)
	kept := r.done[:0]
	for _, s := range r.done {
		if s.End.After(cut) {
			kept = append(kept, s)
		}
	}
	if len(kept) != len(r.done) && r.store != nil {
		if err := r.store.Rewrite(kept); err != nil && r.OnError != nil {
			r.OnError(err)
		}
	}
	r.done = kept
}

// Clear forgets every session, finished or still going, in memory and in the file.
func (r *Recorder) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.done = nil
	r.open = map[string]*Session{}
	if r.store != nil {
		if err := r.store.Rewrite(nil); err != nil && r.OnError != nil {
			r.OnError(err)
		}
	}
}

// Sessions are the sessions that ended or were going at any time since, oldest first, including those
// still going (marked Open).
func (r *Recorder) Sessions(since time.Time) []Session {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []Session
	for _, s := range r.done {
		if s.End.After(since) {
			out = append(out, s)
		}
	}
	for _, s := range r.open {
		if s.End.Sub(s.Start) >= MinSession && s.End.After(since) {
			c := *s
			c.Open = true
			c.Titles = append([]string(nil), s.Titles...)
			out = append(out, c)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Start.Before(out[j].Start) })
	return out
}

// Store is the file sessions are kept in.
type Store struct {
	Path string
	mu   sync.Mutex
}

// Load reads the sessions in the file that ended after cut. Lines it can't read are skipped. dropped is
// how many it left out, so the caller can tidy the file.
func (st *Store) Load(cut time.Time) (kept []Session, dropped int) {
	st.mu.Lock()
	defer st.mu.Unlock()
	f, err := os.Open(st.Path)
	if err != nil {
		return nil, 0
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64<<10), 1<<20)
	for sc.Scan() {
		var s Session
		if err := json.Unmarshal(sc.Bytes(), &s); err != nil || s.Channel == "" || s.End.Before(s.Start) {
			dropped++
			continue
		}
		if !s.End.After(cut) {
			dropped++
			continue
		}
		kept = append(kept, s)
	}
	return kept, dropped
}

// Append adds one session to the end of the file.
func (st *Store) Append(s Session) error {
	st.mu.Lock()
	defer st.mu.Unlock()
	b, err := json.Marshal(s)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(st.Path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(append(b, '\n'))
	return err
}

// Rewrite replaces the file with these sessions, by writing a new one and renaming it into place.
func (st *Store) Rewrite(sessions []Session) error {
	st.mu.Lock()
	defer st.mu.Unlock()
	tmp := st.Path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	w := bufio.NewWriter(f)
	for _, s := range sessions {
		b, err := json.Marshal(s)
		if err != nil {
			f.Close()
			return err
		}
		_, _ = w.Write(append(b, '\n')) // a failed write shows at Flush: the writer keeps its first error
	}
	if err := w.Flush(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, st.Path)
}
