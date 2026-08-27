package mux

import "testing"

func TestFollowsTurnCoversEverythingAboutTheWork(t *testing.T) {
	// Text typed into a turn already under way is a steer. Sent to the subscription that
	// merely owns the chat it lands nowhere and the words are lost, so anything about the
	// turn has to reach the subscription running it.
	for _, method := range []string{
		"turn/start",
		"turn/steer",
		"turn/interrupt",
		"thread/goal/set",
		"thread/goal/get",
		"thread/goal/clear",
		"thread/inject_items",
	} {
		if !followsTurn(method) {
			t.Fatalf("%s is about the turn and must follow it", method)
		}
	}

	// Reading a chat stays with the subscription that can show it: only the one that has
	// been reading the chat's history has its turns to hand.
	for _, method := range []string{
		"thread/read",
		"thread/turns/list",
		"thread/items/list",
		"thread/list",
		"thread/name/set",
		"account/read",
		"",
	} {
		if followsTurn(method) {
			t.Fatalf("%s is about the chat, not its turn", method)
		}
	}
}
