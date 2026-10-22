package api

import (
	"fmt"
	"net/http"
	"time"

	"github.com/johnschult/yagi/internal/access"
)

// RequestDoc is someone who asked to use a tuner and isn't a viewer yet, as the Viewers page shows them.
type RequestDoc struct {
	ID string `json:"id"`
	// Label is what to call them: their tailnet login if known, else their device, else their address.
	Label   string    `json:"label"`
	Address string    `json:"address"`
	Login   string    `json:"login,omitempty"`
	Node    string    `json:"node,omitempty"`
	Device  string    `json:"device"`
	First   time.Time `json:"first"`
	Last    time.Time `json:"last"`
	Count   int       `json:"count"`
}

func requestDocs(reqs []access.Request) []RequestDoc {
	out := make([]RequestDoc, 0, len(reqs))
	for _, r := range reqs {
		out = append(out, RequestDoc{ID: r.ID, Label: r.Label(), Address: r.Address, Login: r.Login, Node: r.Node, Device: r.Device, First: r.First, Last: r.Last, Count: r.Count})
	}
	return out
}

func (a *API) listRequests(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, requestDocs(a.Pol.Requests()))
}

// allowInput is what the owner chooses when allowing someone: not their address, which is what Yagi saw.
type allowInput struct {
	Name   string         `json:"name"`
	Limit  int            `json:"limit"`
	Limits map[string]int `json:"limits"` // per-device overrides, by device name
}

// allowRequest makes a viewer of someone who asked: the same name and limits as adding one, and their tailnet login
// or device, or else their address, as the address. They leave the list because they are a viewer now.
func (a *API) allowRequest(w http.ResponseWriter, r *http.Request) {
	var body allowInput
	if !decode(w, r, &body) {
		return
	}
	req, ok := a.Pol.RequestByID(r.PathValue("id"))
	if !ok {
		writeErr(w, http.StatusNotFound, "that request is gone: they may be a viewer already, or were dismissed")
		return
	}
	v := viewerInput{Name: body.Name, Addresses: []string{req.Entry()}, Limit: body.Limit, Limits: body.Limits}.viewer()
	if v.Name == "" {
		writeErr(w, http.StatusBadRequest, "give the viewer a name")
		return
	}
	_, err := a.Store.Edit(func(s *access.Settings) error {
		if indexOf(s.Viewers, v.Name) >= 0 {
			return &httpErr{http.StatusConflict, fmt.Sprintf("viewer %q already exists", v.Name)}
		}
		s.Viewers = append(s.Viewers, v)
		return nil
	})
	if err != nil {
		a.fail(w, err)
		return
	}
	vs, _ := a.viewerStatus(v.Name)
	writeJSON(w, http.StatusCreated, vs)
}

func (a *API) dismissRequest(w http.ResponseWriter, r *http.Request) {
	if !a.Pol.DismissRequest(r.PathValue("id")) {
		writeErr(w, http.StatusNotFound, "no such request")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
