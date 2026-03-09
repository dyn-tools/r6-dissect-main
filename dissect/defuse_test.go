package dissect

import "testing"

func TestReadDefuserTimerIgnoresCompleteWithoutKnownPlayer(t *testing.T) {
	r := &Reader{
		b:                      append([]byte{4, '0', '.', '0', '0'}, make([]byte, 39)...),
		lastDefuserPlayerIndex: -1,
	}

	if err := readDefuserTimer(r); err != nil {
		t.Fatalf("readDefuserTimer(): expected no error, got %v", err)
	}
	if len(r.MatchFeedback) != 0 {
		t.Fatalf("expected no match feedback, got %d entries", len(r.MatchFeedback))
	}
}
