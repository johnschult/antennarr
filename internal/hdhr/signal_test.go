package hdhr_test

import (
	"context"
	"encoding/binary"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/johnschult/yagi/internal/hdhr"
	"github.com/johnschult/yagi/internal/hdhr/hdhrtest"
)

var fast = hdhr.TestOptions{Wait: 600 * time.Millisecond, Settle: 20 * time.Millisecond, Poll: 20 * time.Millisecond, Key: func() uint32 { return 424242 }}

func dial(t *testing.T, o hdhrtest.Options) (*hdhr.Conn, *hdhrtest.Server) {
	t.Helper()
	srv, err := hdhrtest.New("127.0.0.1:0", o)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(srv.Close)
	c, err := hdhr.Dial(context.Background(), srv.Addr())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	return c, srv
}

// sets are the names and values of every set the tuner was sent, with the lock key that came with it.
func sets(srv *hdhrtest.Server) [][3]string {
	var out [][3]string
	for _, f := range srv.Frames() {
		tlvs, _ := hdhr.ParseTLVs(f.Payload)
		var name, value, key string
		var hasValue bool
		for _, t := range tlvs {
			switch t.Tag {
			case hdhr.TagGetSetName:
				name = strings.TrimRight(string(t.Value), "\x00")
			case hdhr.TagGetSetValue:
				value, hasValue = strings.TrimRight(string(t.Value), "\x00"), true
			case hdhr.TagLockKey:
				key = "key:" + string(rune('0'+len(t.Value))) + ":" + itoa(binary.BigEndian.Uint32(t.Value))
			}
		}
		if hasValue {
			out = append(out, [3]string{name, value, key})
		}
	}
	return out
}

func itoa(n uint32) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for ; n > 0; n /= 10 {
		b = append([]byte{byte('0' + n%10)}, b...)
	}
	return string(b)
}

func TestAFreeTunerIsTunedReadAndLetGo(t *testing.T) {
	c, srv := dial(t, hdhrtest.Options{Signals: map[string]hdhrtest.Signal{"2.1": {Lock: "atsc3", Strength: 84, Quality: 97, Symbol: 100}}})
	got, err := c.TestSignal(context.Background(), 3, "2.1", fast)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Locked || got.Lock != "atsc3" || got.Strength != 84 || got.Quality != 97 || got.Symbol != 100 || got.Tuner != 3 || got.Channel != "2.1" {
		t.Fatalf("signal = %+v", got)
	}
	if srv.Held(3) {
		t.Error("the tuner was left held")
	}
	if tunes := srv.Tunes(); len(tunes) != 1 || tunes[0] != "tuner3 2.1" {
		t.Errorf("tunes = %v", tunes)
	}
}

func TestEveryChangeAfterTheLockCarriesTheKey(t *testing.T) {
	c, srv := dial(t, hdhrtest.Options{})
	if _, err := c.TestSignal(context.Background(), 1, "5.1", fast); err != nil {
		t.Fatal(err)
	}
	want := [][3]string{
		{"/tuner1/lockkey", "424242", ""},           // taking it needs no key
		{"/tuner1/vchannel", "5.1", "key:4:424242"}, // from then on, the key goes with every change
		{"/tuner1/channel", "none", "key:4:424242"}, // giving the channel back
		{"/tuner1/lockkey", "none", "key:4:424242"}, // and the lock
	}
	got := sets(srv)
	if len(got) != len(want) {
		t.Fatalf("sets = %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("set %d = %v, want %v", i, got[i], want[i])
		}
	}
}

func TestATunerThatIsInUseIsLeftAloneWhateverWayItIsInUse(t *testing.T) {
	for state := range map[string]bool{"locked": true, "tuned": true, "streaming": true} {
		c, srv := dial(t, hdhrtest.Options{Busy: map[int]string{2: state}})
		srv.Forget()
		_, err := c.TestSignal(context.Background(), 2, "2.1", fast)
		if !errors.Is(err, hdhr.ErrTunerBusy) {
			t.Errorf("%s: %v", state, err)
		}
		if s := sets(srv); len(s) != 0 {
			t.Errorf("%s: a busy tuner was changed: %v", state, s)
		}
		if len(srv.Tunes()) != 0 {
			t.Errorf("%s: a busy tuner was tuned", state)
		}
	}
}

func TestNoLockIsAnAnswerNotAnError(t *testing.T) {
	c, srv := dial(t, hdhrtest.Options{Signals: map[string]hdhrtest.Signal{"9.9": {Lock: "", Strength: 12}}})
	got, err := c.TestSignal(context.Background(), 0, "9.9", fast)
	if err != nil {
		t.Fatal(err)
	}
	if got.Locked || got.Strength != 12 {
		t.Errorf("signal = %+v", got)
	}
	if srv.Held(0) {
		t.Error("the tuner was left held after a test that never locked")
	}
}

func TestALockThatComesLateIsWaitedFor(t *testing.T) {
	c, _ := dial(t, hdhrtest.Options{LockAfter: 150 * time.Millisecond})
	got, err := c.TestSignal(context.Background(), 0, "2.1", fast)
	if err != nil || !got.Locked {
		t.Fatalf("signal = %+v, %v", got, err)
	}
	if got.Quality != 90 {
		t.Errorf("quality = %d: it should be the settled reading", got.Quality)
	}
}

func TestATunerThatWillNotTuneIsGivenBack(t *testing.T) {
	c, srv := dial(t, hdhrtest.Options{RefuseTune: true})
	_, err := c.TestSignal(context.Background(), 0, "2.1", fast)
	if err == nil || !strings.Contains(err.Error(), "wouldn't tune") {
		t.Fatalf("err = %v", err)
	}
	if srv.Held(0) {
		t.Error("the tuner was left held after a refused tune")
	}
}

func TestEndingTheTestEarlyStillGivesTheTunerBack(t *testing.T) {
	c, srv := dial(t, hdhrtest.Options{LockAfter: time.Minute})
	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()
	o := fast
	o.Wait = 10 * time.Second
	_, err := c.TestSignal(ctx, 0, "2.1", o)
	if err == nil {
		t.Fatal("a cancelled test said it finished")
	}
	deadline := time.Now().Add(2 * time.Second)
	for srv.Held(0) && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if srv.Held(0) {
		t.Error("the tuner was left held when the test was cut short")
	}
}

func TestNothingOddIsSent(t *testing.T) {
	c, srv := dial(t, hdhrtest.Options{})
	srv.Forget()
	for _, v := range []string{"", "x", "2.1/../x", "2.1 5.1", "2.1\n", "1234567", "../2.1"} {
		if _, err := c.TestSignal(context.Background(), 0, v, fast); err == nil {
			t.Errorf("%q was sent", v)
		}
	}
	for _, n := range []int{-1, 16, 99} {
		if _, err := c.TestSignal(context.Background(), n, "2.1", fast); err == nil {
			t.Errorf("tuner %d was used", n)
		}
	}
	if len(srv.Frames()) != 0 {
		t.Errorf("%d frames were sent for requests that should have been refused", len(srv.Frames()))
	}
}

func TestParseStatus(t *testing.T) {
	cases := map[string]hdhr.Signal{
		"ch=atsc3:575000000 lock=atsc3 ss=84 snq=100 seq=99 bps=1 pps=2": {Locked: true, Lock: "atsc3", Strength: 84, Quality: 100, Symbol: 99},
		"ch=none lock=none ss=0 snq=0 seq=0 bps=0 pps=0":                 {},
		"lock=8vsb ss=250 snq=-5 seq=x":                                  {Locked: true, Lock: "8vsb", Strength: 100},
		"":                                                               {},
		"garbage with = signs ss":                                        {},
	}
	for in, want := range cases {
		if got := hdhr.ParseStatus(in); got != want {
			t.Errorf("%q: got %+v, want %+v", in, got, want)
		}
	}
}

// A real tuner answers a lock with the address that holds it, not the key that was set, and gives the lock back
// only for that key. The test has to work against that: it once called a free tuner busy, and left it locked.
func TestATunerThatShowsWhoHoldsTheLockInsteadOfTheKeyIsTestedAndGivenBack(t *testing.T) {
	c, srv := dial(t, hdhrtest.Options{LockShowsOwner: "192.168.1.5"})
	sig, err := c.TestSignal(context.Background(), 3, "2.1", fast)
	if err != nil {
		t.Fatalf("%v", err)
	}
	if !sig.Locked || sig.Channel != "2.1" || sig.Tuner != 3 {
		t.Errorf("sig = %+v", sig)
	}
	if srv.Held(3) {
		t.Error("the tuner is still held: the lock was not given back with its key")
	}
}

func TestATunerThatShowsItsOwnerAndIsLockedBySomeoneElseIsLeftAlone(t *testing.T) {
	c, srv := dial(t, hdhrtest.Options{LockShowsOwner: "192.168.1.5", Busy: map[int]string{3: "locked"}})
	if _, err := c.TestSignal(context.Background(), 3, "2.1", fast); !errors.Is(err, hdhr.ErrTunerBusy) {
		t.Fatalf("err = %v, want busy", err)
	}
	if !srv.Held(3) {
		t.Error("somebody else's lock was taken away")
	}
	if len(srv.Tunes()) != 0 {
		t.Errorf("tuned a tuner that was not ours: %v", srv.Tunes())
	}
}

func TestAnErrorAfterTakingTheLockStillGivesItBack(t *testing.T) {
	c, srv := dial(t, hdhrtest.Options{LockShowsOwner: "192.168.1.5", RefuseTune: true})
	if _, err := c.TestSignal(context.Background(), 3, "2.1", fast); err == nil {
		t.Fatal("no error")
	}
	if srv.Held(3) {
		t.Error("held after a failed tune")
	}
}
