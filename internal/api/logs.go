package api

import (
	"net/http"
	"strconv"

	"github.com/johnschult/antennarr/internal/logbuf"
)

// logsDoc is the Logs page: the recent lines that matched, oldest first.
type logsDoc struct {
	Entries []logbuf.Entry `json:"entries"`
	// Latest is the number of the newest line kept, to ask for what came after it with ?since=.
	Latest uint64 `json:"latest"`
	// Kept is how many lines are held and Capacity how many can be, so the page can say how far back it goes.
	Kept     int `json:"kept"`
	Capacity int `json:"capacity"`
	// Level is the level being logged at (the setting): lines below it are never kept.
	Level string `json:"level"`
}

const (
	defaultLogLimit = 500
	maxLogLimit     = 2000
)

// getLogs returns the recent log lines. ?level= (debug, info, warn or error) keeps that level and above, ?q=
// keeps lines that contain the text (in the message or an attribute, without regard to case), ?since= keeps only
// lines newer than that number, and ?limit= (default 500, at most 2000) keeps the newest that many.
func (a *API) getLogs(w http.ResponseWriter, r *http.Request) {
	if a.Logs == nil {
		writeErr(w, http.StatusNotFound, "there is no log to show")
		return
	}
	q := r.URL.Query()
	query := logbuf.Query{Text: q.Get("q"), Limit: defaultLogLimit}
	if v := q.Get("level"); v != "" {
		if !logbuf.Valid(v) {
			writeErr(w, http.StatusBadRequest, "level must be debug, info, warn or error")
			return
		}
		query.Level = v
	}
	if v := q.Get("since"); v != "" {
		n, err := strconv.ParseUint(v, 10, 64)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "since must be a number")
			return
		}
		query.Since = n
	}
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > maxLogLimit {
			writeErr(w, http.StatusBadRequest, "limit must be from 1 to 2000")
			return
		}
		query.Limit = n
	}
	if len(query.Text) > 200 {
		writeErr(w, http.StatusBadRequest, "q is too long")
		return
	}
	res := a.Logs.Find(query)
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, logsDoc{Entries: res.Entries, Latest: res.Latest, Kept: res.Kept, Capacity: res.Capacity, Level: a.Pol.Settings().LogLevelName()})
}

// clearLogs forgets the lines that are kept. They were only ever in memory.
func (a *API) clearLogs(w http.ResponseWriter, r *http.Request) {
	if a.Logs != nil {
		a.Logs.Clear()
	}
	w.WriteHeader(http.StatusNoContent)
}
