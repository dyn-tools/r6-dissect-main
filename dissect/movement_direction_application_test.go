package dissect

import "testing"

func TestMovementApplyDerivedViewingDirectionsPrefersProjectedTimeline(t *testing.T) {
	ordered := []MovementTrack{{
		ActorID: "actor-a",
		Samples: []MovementSample{{Offset: 1}, {Offset: 2}, {Offset: 3}},
	}}
	primary := []MovementTrack{{
		ActorID: "actor-a",
		Samples: []MovementSample{{Offset: 1}, {Offset: 2}, {Offset: 3}},
	}}

	derived := map[int]float64{0: 15, 1: 25, 2: 35}
	timeline := map[int]float64{0: 90, 1: 95, 2: 100}

	ordered, primary = movementApplyDerivedViewingDirections(ordered, primary, movementDirectionSearchResult{
		derivedByActor:  map[string]map[int]float64{"actor-a": derived},
		timelineByActor: map[string]map[int]float64{"actor-a": timeline},
	})

	for i, expected := range []float64{90, 95, 100} {
		got := ordered[0].Samples[i].ViewingDirectionDegrees
		if got == nil || *got != expected {
			t.Fatalf("ordered sample %d expected projected timeline heading %f, got %v", i, expected, got)
		}
		gotPrimary := primary[0].Samples[i].ViewingDirectionDegrees
		if gotPrimary == nil || *gotPrimary != expected {
			t.Fatalf("primary sample %d expected projected timeline heading %f, got %v", i, expected, gotPrimary)
		}
	}
}

func TestMovementApplyDerivedViewingDirectionsUsesDerivedForGapsOnly(t *testing.T) {
	ordered := []MovementTrack{{
		ActorID: "actor-a",
		Samples: []MovementSample{{Offset: 1}, {Offset: 2}, {Offset: 3}},
	}}
	primary := []MovementTrack{{
		ActorID: "actor-a",
		Samples: []MovementSample{{Offset: 1}, {Offset: 2}, {Offset: 3}},
	}}

	derived := map[int]float64{0: 10, 1: 20, 2: 30}
	timeline := map[int]float64{1: 200}

	ordered, primary = movementApplyDerivedViewingDirections(ordered, primary, movementDirectionSearchResult{
		derivedByActor:  map[string]map[int]float64{"actor-a": derived},
		timelineByActor: map[string]map[int]float64{"actor-a": timeline},
	})

	expected := []float64{10, 200, 30}
	for i, want := range expected {
		got := ordered[0].Samples[i].ViewingDirectionDegrees
		if got == nil || *got != want {
			t.Fatalf("ordered sample %d expected %f, got %v", i, want, got)
		}
		gotPrimary := primary[0].Samples[i].ViewingDirectionDegrees
		if gotPrimary == nil || *gotPrimary != want {
			t.Fatalf("primary sample %d expected %f, got %v", i, want, gotPrimary)
		}
	}
}
