package dissect

import "testing"

func TestHeaderRecordingPlayerFallsBackToProfileID(t *testing.T) {
	header := Header{
		RecordingPlayerID:  42,
		RecordingProfileID: "profile-dyn4mic",
		Players: []Player{
			{ID: 7, ProfileID: "profile-other", Username: "other"},
			{ID: 0, ProfileID: "profile-dyn4mic", Username: "dyn4mic"},
		},
	}

	player := header.RecordingPlayer()
	if player.Username != "dyn4mic" {
		t.Fatalf("expected profile fallback to resolve dyn4mic, got %+v", player)
	}
}
