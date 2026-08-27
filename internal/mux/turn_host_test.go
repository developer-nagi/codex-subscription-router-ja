package mux

import "testing"

func newTurnHostTestMultiplexer() *Multiplexer {
	return &Multiplexer{turnHosts: make(map[string]string)}
}

func TestTurnHostRemembersWhereAChatsWorkMoved(t *testing.T) {
	multiplexer := newTurnHostTestMultiplexer()
	if _, ok := multiplexer.turnHost("t1"); ok {
		t.Fatal("a chat with no history of running out has no separate host")
	}

	multiplexer.rememberTurnHost("t1", "secondary")
	host, ok := multiplexer.turnHost("t1")
	if !ok || host != "secondary" {
		t.Fatalf("host = %q ok=%v, want the subscription that took the work over", host, ok)
	}

	// The chat itself never moves, so reading it is unaffected: only this record says
	// where its next turn should run.
	multiplexer.forgetTurnHost("t1")
	if _, ok := multiplexer.turnHost("t1"); ok {
		t.Fatal("once the host is unusable the chat's own subscription is tried again")
	}
}

func TestRememberTurnHostIgnoresAChatWithoutAnId(t *testing.T) {
	multiplexer := newTurnHostTestMultiplexer()
	multiplexer.rememberTurnHost("", "secondary")
	if len(multiplexer.turnHosts) != 0 {
		t.Fatalf("recorded %d hosts, want none", len(multiplexer.turnHosts))
	}
}
