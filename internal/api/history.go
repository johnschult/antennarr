package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/johnschult/antennarr/internal/history"
)

// historyDoc is the Activity page: the sessions in the last Days days, oldest first, and when it was read.
type historyDoc struct {
	Days     int               `json:"days"`
	Now      time.Time         `json:"now"`
	Sessions []history.Session `json:"sessions"`
}

// getHistory lists who watched what. ?days=N (1 to 30, default 7) says how far back.
func (a *API) getHistory(w http.ResponseWriter, r *http.Request) {
	days := 7
	if v := r.URL.Query().Get("days"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 30 {
			writeErr(w, http.StatusBadRequest, "days must be from 1 to 30")
			return
		}
		days = n
	}
	now := time.Now()
	doc := historyDoc{Days: days, Now: now, Sessions: []history.Session{}}
	if a.History != nil {
		if s := a.History.Sessions(now.AddDate(0, 0, -days)); s != nil {
			doc.Sessions = s
		}
	}
	writeJSON(w, http.StatusOK, doc)
}
