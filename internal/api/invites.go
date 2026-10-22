package api

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/johnschult/yagi/internal/access"
	"github.com/johnschult/yagi/internal/enroll"
)

// InviteDoc is an invite as the admin page shows it: never the link or its token, which are shown once, when the
// invite is made.
type InviteDoc struct {
	ID      string         `json:"id"`
	Name    string         `json:"name"`
	Limit   int            `json:"limit"`
	Limits  map[string]int `json:"limits,omitempty"`
	Created time.Time      `json:"created"`
	Expires time.Time      `json:"expires"`
	// Used is whether someone has enrolled with it, and Viewer the viewer that made.
	Used   bool   `json:"used"`
	Viewer string `json:"viewer,omitempty"`
	// Expired is an unused invite whose time ran out. It stays listed, so the owner can see it and clear it.
	Expired bool `json:"expired"`
}

func inviteDocs(set access.Settings, now time.Time) []InviteDoc {
	out := make([]InviteDoc, 0, len(set.Invites))
	for _, inv := range set.Invites {
		out = append(out, InviteDoc{
			ID: inv.ID, Name: inv.Name, Limit: inv.Limit, Limits: inv.Limits, Created: inv.Created, Expires: inv.Expires,
			Used: inv.Used(), Viewer: inv.ViewerName, Expired: !inv.Live(now),
		})
	}
	return out
}

func (a *API) listInvites(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, inviteDocs(a.Pol.Settings(), time.Now()))
}

type inviteInput struct {
	Name   string         `json:"name"`
	Limit  int            `json:"limit"`
	Limits map[string]int `json:"limits"` // per-device overrides, by device name
}

// inviteCreated is the reply to making an invite: the only time the link is shown.
type inviteCreated struct {
	InviteDoc
	// Path is the link without a host, and URL is the link as this request reached Yagi, which is what the owner
	// copies. A viewer reaches Yagi by some other name, so the page may need to swap the host.
	Path string `json:"path"`
	URL  string `json:"url"`
}

func (a *API) addInvite(w http.ResponseWriter, r *http.Request) {
	var in inviteInput
	if !decode(w, r, &in) {
		return
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		writeErr(w, http.StatusBadRequest, "give the viewer a name")
		return
	}
	if err := access.ValidateLabel(name); err != nil {
		writeErr(w, http.StatusBadRequest, "the name "+err.Error())
		return
	}
	if in.Limit < 0 {
		writeErr(w, http.StatusBadRequest, "the limit cannot be negative")
		return
	}
	now := time.Now()
	var token string
	var inv access.Invite
	set, err := a.Store.Edit(func(s *access.Settings) error {
		if !s.EnrollmentEnabled() {
			return &httpErr{http.StatusConflict, "turn on Sign in with Plex, and let people enroll themselves, first"}
		}
		if len(s.Invites) >= access.MaxInvites {
			return &httpErr{http.StatusConflict, fmt.Sprintf("there are %d invites already: remove some first", access.MaxInvites)}
		}
		if indexOf(s.Viewers, name) >= 0 {
			return &httpErr{http.StatusConflict, fmt.Sprintf("a viewer named %q already exists", name)}
		}
		for _, other := range s.Invites {
			if strings.EqualFold(other.Name, name) {
				return &httpErr{http.StatusConflict, fmt.Sprintf("there is an invite for %q already", name)}
			}
		}
		limits := map[string]int{}
		for dev, n := range in.Limits {
			if n < 0 {
				return &httpErr{http.StatusBadRequest, fmt.Sprintf("the limit for %q cannot be negative", dev)}
			}
			known := false
			for _, d := range s.Devices {
				known = known || d.Name == dev
			}
			if !known {
				return &httpErr{http.StatusBadRequest, fmt.Sprintf("there is no device named %q", dev)}
			}
			limits[dev] = n
		}
		token, inv = access.NewInvite(name, in.Limit, limits, now)
		s.Invites = append(s.Invites, inv)
		return nil
	})
	if err != nil {
		a.fail(w, err)
		return
	}
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	path := enroll.Path + token
	docs := inviteDocs(set, now)
	writeJSON(w, http.StatusCreated, inviteCreated{InviteDoc: docs[len(docs)-1], Path: path, URL: scheme + "://" + r.Host + path})
}

func (a *API) deleteInvite(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	_, err := a.Store.Edit(func(s *access.Settings) error {
		for i, inv := range s.Invites {
			if inv.ID == id {
				s.Invites = append(s.Invites[:i:i], s.Invites[i+1:]...)
				return nil
			}
		}
		return &httpErr{http.StatusNotFound, "no such invite"}
	})
	if err != nil {
		a.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
