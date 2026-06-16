package ws

import (
	"testing"
	"time"
)

func assertReceives(t *testing.T, c *Client, want string) {
	t.Helper()
	select {
	case msg := <-c.send:
		if string(msg) != want {
			t.Fatalf("got %q, want %q", msg, want)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for a message")
	}
}

func TestHubBroadcastReachesAll(t *testing.T) {
	h := newHub("r1")
	go h.run()
	defer h.Stop()

	a := NewClient("a", "r1")
	b := NewClient("b", "r1")
	h.Register(a)
	h.Register(b)

	h.Broadcast([]byte("hello"))
	assertReceives(t, a, "hello")
	assertReceives(t, b, "hello")
}

func TestHubUnregisterStopsDelivery(t *testing.T) {
	h := newHub("r1")
	go h.run()
	defer h.Stop()

	a := NewClient("a", "r1")
	b := NewClient("b", "r1")
	h.Register(a)
	h.Register(b)

	h.Unregister(a)
	if got := h.Count(); got != 1 {
		t.Fatalf("count = %d, want 1", got)
	}

	h.Broadcast([]byte("x"))
	assertReceives(t, b, "x")

	// a's mailbox was closed on unregister
	if _, ok := <-a.send; ok {
		t.Fatal("expected a.send to be closed after unregister")
	}
}

func TestHubSlowClientDropped(t *testing.T) {
	h := newHub("r1")
	go h.run()
	defer h.Stop()

	slow := NewClient("slow", "r1")
	h.Register(slow)

	// slow never reads; fill its buffer exactly to capacity
	for i := 0; i < sendBuffer; i++ {
		h.Broadcast([]byte("fill"))
	}
	if got := h.Count(); got != 1 {
		t.Fatalf("count before overflow = %d, want 1", got)
	}

	// the next message cannot fit; the hub must drop slow, not block
	h.Broadcast([]byte("overflow"))
	if got := h.Count(); got != 0 {
		t.Fatalf("count after overflow = %d, want 0 (slow should be dropped)", got)
	}

	// the hub still works after dropping a client
	fresh := NewClient("fresh", "r1")
	h.Register(fresh)
	h.Broadcast([]byte("hi"))
	assertReceives(t, fresh, "hi")
}

func TestHubManagerLifecycle(t *testing.T) {
	m := NewHubManager()

	h1 := m.GetOrCreate("r1")
	h2 := m.GetOrCreate("r1")
	if h1 != h2 {
		t.Fatal("GetOrCreate returned different hubs for the same room")
	}
	if _, ok := m.Get("r1"); !ok {
		t.Fatal("Get should find an existing room")
	}

	m.Remove("r1")
	if _, ok := m.Get("r1"); ok {
		t.Fatal("Get should not find a removed room")
	}

	// after removal a fresh hub is created
	h3 := m.GetOrCreate("r1")
	if h3 == h1 {
		t.Fatal("expected a fresh hub after removal")
	}
	m.Remove("r1")
}