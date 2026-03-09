package dissect

import "testing"

func TestWinningTeamFromScoreDelta(t *testing.T) {
	tests := []struct {
		name   string
		teams  [2]Team
		want   int
		wantOK bool
	}{
		{
			name: "team 0 score increased by one",
			teams: [2]Team{
				{StartingScore: 1, Score: 2},
				{StartingScore: 2, Score: 2},
			},
			want:   0,
			wantOK: true,
		},
		{
			name: "team 1 score increased by one",
			teams: [2]Team{
				{StartingScore: 2, Score: 2},
				{StartingScore: 2, Score: 3},
			},
			want:   1,
			wantOK: true,
		},
		{
			name: "invalid zeroed final header scores",
			teams: [2]Team{
				{StartingScore: 4, Score: 0},
				{StartingScore: 2, Score: 0},
			},
			want:   -1,
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := winningTeamFromScoreDelta(tt.teams)
			if got != tt.want || ok != tt.wantOK {
				t.Fatalf("winningTeamFromScoreDelta() = (%d, %t), want (%d, %t)", got, ok, tt.want, tt.wantOK)
			}
		})
	}
}

func TestRoundEndY9S4DoesNotFlipWinnerOnPlantFromLosingTeam(t *testing.T) {
	r := &Reader{
		Header: Header{
			CodeVersion: Y9S4,
			Teams: [2]Team{
				{StartingScore: 1, Score: 2, Role: Attack},
				{StartingScore: 2, Score: 2, Role: Defense},
			},
			Players: []Player{
				{Username: "attacker", TeamIndex: 0},
				{Username: "defender", TeamIndex: 1},
			},
		},
		MatchFeedback: []MatchUpdate{
			{Type: DefuserPlantComplete, Username: "defender"},
		},
	}

	r.roundEnd()

	if !r.Header.Teams[0].Won {
		t.Fatal("expected team 0 to remain the winner")
	}
	if r.Header.Teams[1].Won {
		t.Fatal("expected team 1 to remain a loser")
	}
	if r.Header.Teams[1].WinCondition != "" {
		t.Fatalf("expected team 1 win condition to stay empty, got %q", r.Header.Teams[1].WinCondition)
	}
}
