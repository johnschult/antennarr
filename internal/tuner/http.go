package tuner

import (
	"encoding/json"
	"net/http"
)

// StatusHandler serves the monitor's snapshot as JSON.
func StatusHandler(m *Monitor) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		s := m.Snapshot()
		if s.Slots == nil {
			s.Slots = []Slot{} // [] rather than null for clients
		}
		if s.Events == nil {
			s.Events = []Event{}
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(s)
	})
}
