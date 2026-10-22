package access

import (
	"net/http"
	"testing"
	"time"
)

func asking(remote string) *http.Request { return req(remote) }

func askingAs(remote string, id Identity) *http.Request {
	r := req(remote)
	return r.WithContext(WithIdentity(r.Context(), id))
}

func TestSomeoneWhoIsNotAViewerIsListedOnceHoweverOftenTheyAsk(t *testing.T) {
	p, _ := newPolicy(t, twoViewers, &fakeMon{})
	now := time.Unix(1_000_000, 0)
	p.now = func() time.Time { return now }
	for i := 0; i < 7; i++ {
		now = now.Add(time.Minute)
		if err := p.Authorize(asking("100.64.0.9:5000")); err == nil {
			t.Fatal("a stranger was let in")
		}
	}
	got := p.Requests()
	if len(got) != 1 {
		t.Fatalf("requests = %+v", got)
	}
	r := got[0]
	if r.Address != "100.64.0.9" || r.Count != 7 || r.Device != testDevice || r.Label() != "100.64.0.9" || r.Entry() != "100.64.0.9" || len(r.ID) != 12 {
		t.Errorf("request = %+v", r)
	}
	if !r.First.Equal(time.Unix(1_000_060, 0)) || !r.Last.Equal(time.Unix(1_000_420, 0)) {
		t.Errorf("first %v, last %v", r.First, r.Last)
	}
	if again, ok := p.RequestByID(r.ID); !ok || again.Count != 7 {
		t.Errorf("by ID: %+v %v", again, ok)
	}
	if _, ok := p.RequestByID("000000000000"); ok {
		t.Error("an unknown ID was found")
	}
}

func TestAViewerIsNeverListed(t *testing.T) {
	p, _ := newPolicy(t, twoViewers, &fakeMon{})
	p.Authorize(asking("10.8.0.5:1")) // Mom's range
	if got := p.Requests(); len(got) != 0 {
		t.Errorf("a viewer was listed: %+v", got)
	}
}

func TestATailnetLoginIsOnePersonWhicheverDeviceTheyAskFrom(t *testing.T) {
	p, _ := newPolicy(t, twoViewers, &fakeMon{})
	laptop := Identity{Login: "Friend@Example.com", Node: "laptop.tail1.ts.net."}
	phone := Identity{Login: "friend@example.com", Node: "phone.tail1.ts.net"}
	p.Authorize(askingAs("100.64.0.9:1", laptop))
	p.Authorize(askingAs("100.64.0.10:1", phone))
	got := p.Requests()
	if len(got) != 1 || got[0].Count != 2 {
		t.Fatalf("requests = %+v", got)
	}
	if got[0].Label() != "friend@example.com" || got[0].Entry() != "tailscale:friend@example.com" {
		t.Errorf("label %q, entry %q", got[0].Label(), got[0].Entry())
	}
	// A tagged device has no login: it is that device.
	p.Authorize(askingAs("100.64.0.20:1", Identity{Node: "kiosk.tail1.ts.net"}))
	var kiosk Request
	for _, r := range p.Requests() {
		if r.Login == "" {
			kiosk = r
		}
	}
	if kiosk.Entry() != "tailscale:kiosk.tail1.ts.net" || kiosk.Label() != "kiosk.tail1.ts.net" {
		t.Errorf("kiosk = %+v", kiosk)
	}
	// And an address with no identity is just the address, apart from the same device's identity.
	p.Authorize(asking("100.64.0.9:2"))
	if n := len(p.Requests()); n != 3 {
		t.Errorf("%d requests, want 3 (the login, the tagged device, the bare address)", n)
	}
}

func TestARequesterWhoBecomesAViewerLeavesTheList(t *testing.T) {
	p, _ := newPolicy(t, twoViewers, &fakeMon{})
	p.Authorize(asking("100.64.0.9:1"))
	p.Authorize(askingAs("100.64.0.30:1", Identity{Login: "pal@example.com"}))
	p.Authorize(askingAs("100.64.0.31:1", Identity{Node: "den.tail1.ts.net"}))
	if n := len(p.Requests()); n != 3 {
		t.Fatalf("%d requests", n)
	}
	s := p.Settings()
	s.Viewers = append(s.Viewers,
		Viewer{Name: "A", Addresses: []string{"100.64.0.9"}, Limit: 1},
		Viewer{Name: "B", Addresses: []string{"tailscale:pal@example.com"}, Limit: 1},
		Viewer{Name: "C", Addresses: []string{"tailscale:den.tail1.ts.net"}, Limit: 1})
	if err := p.Update(s); err != nil {
		t.Fatal(err)
	}
	if got := p.Requests(); len(got) != 0 {
		t.Errorf("still listed after being allowed: %+v", got)
	}
}

func TestADismissedRequesterStaysQuiet(t *testing.T) {
	p, _ := newPolicy(t, twoViewers, &fakeMon{})
	ev := &fakeEvents{}
	p.SetEvents(ev)
	p.Authorize(asking("100.64.0.9:1"))
	id := p.Requests()[0].ID
	if !p.DismissRequest(id) {
		t.Fatal("not dismissed")
	}
	if p.DismissRequest(id) {
		t.Error("dismissed twice")
	}
	p.Authorize(asking("100.64.0.9:1"))
	if got := p.Requests(); len(got) != 0 {
		t.Errorf("they came back: %+v", got)
	}
	if len(ev.requested) != 1 {
		t.Errorf("told %d times", len(ev.requested))
	}
}

func TestTheOwnerIsToldOnceWhenSomeoneFirstAsks(t *testing.T) {
	s := twoViewers
	s.Labels = map[string]string{testDevice: "Living room"}
	p, _ := newPolicy(t, s, &fakeMon{})
	ev := &fakeEvents{}
	p.SetEvents(ev)
	for i := 0; i < 5; i++ {
		p.Authorize(askingAs("100.64.0.9:1", Identity{Login: "friend@example.com", Node: "laptop.tail1.ts.net"}))
	}
	p.Authorize(asking("100.64.0.12:1"))
	if len(ev.requested) != 2 || ev.requested[0] != "antenna|Living room|friend@example.com" || ev.requested[1] != "antenna|Living room|100.64.0.12" {
		t.Errorf("requested = %v", ev.requested)
	}
}

func TestTheLatestRequestIsFirstAndOnlyAFewAreKept(t *testing.T) {
	p, _ := newPolicy(t, twoViewers, &fakeMon{})
	now := time.Unix(1_000_000, 0)
	p.now = func() time.Time { return now }
	for i := 0; i < maxRequests+10; i++ {
		now = now.Add(time.Second)
		p.Authorize(asking("100.65." + string(rune('0'+i/100)) + "." + itoa(i%100+1) + ":1"))
	}
	got := p.Requests()
	if len(got) != maxRequests {
		t.Fatalf("%d kept, want %d", len(got), maxRequests)
	}
	if !got[0].Last.After(got[len(got)-1].Last) {
		t.Error("not the latest first")
	}
	first := time.Unix(1_000_001, 0)
	for _, r := range got {
		if r.First.Equal(first) {
			t.Error("the oldest was kept, and a newer one dropped")
		}
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for ; n > 0; n /= 10 {
		b = append([]byte{byte('0' + n%10)}, b...)
	}
	return string(b)
}

func TestTheRequestEventIsOnUnlessSwitchedOff(t *testing.T) {
	var s Settings
	if !s.NotifyEnabled(NotifyRequest) {
		t.Error("request events should be on by default")
	}
	off := false
	s.Notifications = &Notifications{Events: NotifyEvents{Request: &off}}
	if s.NotifyEnabled(NotifyRequest) || s.Clone().NotifyEnabled(NotifyRequest) {
		t.Error("switched off but still on")
	}
}
