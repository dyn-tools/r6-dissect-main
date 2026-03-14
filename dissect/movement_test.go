package dissect

import (
	"encoding/binary"
	"encoding/hex"
	"math"
	"testing"
)

func TestMovementDataMergesPositionAndRotation(t *testing.T) {
	actor := []byte{0, 0, 0, 0, 0, 0, 0, 0}
	buf := make([]byte, 0)
	buf = append(buf, movementTimePacket(90)...)
	buf = append(buf, movementRecord(actor, movementPositionProp, 1, 2, 3)...)
	buf = append(buf, movementRecord(actor, movementRotationProp, 4, 5, 6)...)
	buf = append(buf, movementTimePacket(89)...)
	buf = append(buf, movementRecord(actor, movementPositionProp, 7, 8, 9)...)

	r := Reader{
		b:      buf,
		Header: Header{CodeVersion: Y10S3_1},
	}

	out := r.MovementData()
	if len(out.Tracks) != 1 {
		t.Fatalf("expected 1 track, got %d", len(out.Tracks))
	}
	track := out.Tracks[0]
	if track.PositionSamples != 2 {
		t.Fatalf("expected 2 position samples, got %d", track.PositionSamples)
	}
	if track.RotationSamples != 1 {
		t.Fatalf("expected 1 rotation sample, got %d", track.RotationSamples)
	}
	if len(track.Samples) != 3 {
		t.Fatalf("expected 3 merged samples, got %d", len(track.Samples))
	}
	if track.Samples[0].Time != "1:30" || track.Samples[0].TimeInSeconds == nil || *track.Samples[0].TimeInSeconds != 90 {
		t.Fatalf("unexpected first timestamp: %+v", track.Samples[0])
	}
	if track.Samples[0].Rotation != nil {
		t.Fatalf("first sample should not have rotation yet: %+v", track.Samples[0])
	}
	if track.Samples[1].Changed != "rotation" {
		t.Fatalf("expected second sample to be a rotation update, got %q", track.Samples[1].Changed)
	}
	if track.Samples[1].Position == nil || track.Samples[1].Rotation == nil {
		t.Fatalf("second sample should include both position and rotation: %+v", track.Samples[1])
	}
	if track.Samples[1].RotationDegrees == nil {
		t.Fatalf("second sample should include derived rotationDegrees: %+v", track.Samples[1])
	}
	if track.Samples[2].Time != "1:29" || track.Samples[2].TimeInSeconds == nil || *track.Samples[2].TimeInSeconds != 89 {
		t.Fatalf("unexpected second timestamp: %+v", track.Samples[2])
	}
	if track.Samples[2].Rotation == nil || !float32Equal(track.Samples[2].Rotation.Z, 6) {
		t.Fatalf("expected latest rotation to carry forward, got %+v", track.Samples[2])
	}
	if track.Samples[2].Position == nil || !float32Equal(track.Samples[2].Position.X, 7) {
		t.Fatalf("expected updated position to be carried, got %+v", track.Samples[2])
	}
	if track.FirstOffset != len(movementTimePacket(90))+len(actor) {
		t.Fatalf("unexpected first offset: %d", track.FirstOffset)
	}
	if track.LastOffset != len(movementTimePacket(90))+len(movementRecord(actor, movementPositionProp, 1, 2, 3))+len(movementRecord(actor, movementRotationProp, 4, 5, 6))+len(movementTimePacket(89))+len(actor) {
		t.Fatalf("unexpected last offset: %d", track.LastOffset)
	}
	if track.FirstPosition == nil || !float32Equal(track.FirstPosition.Z, 3) {
		t.Fatalf("expected first position metadata, got %+v", track.FirstPosition)
	}
	if track.LastPosition == nil || !float32Equal(track.LastPosition.X, 7) {
		t.Fatalf("expected last position metadata, got %+v", track.LastPosition)
	}
	if track.FirstRotation == nil || !float32Equal(track.FirstRotation.Y, 5) {
		t.Fatalf("expected first rotation metadata, got %+v", track.FirstRotation)
	}
	if track.LastRotation == nil || !float32Equal(track.LastRotation.Z, 6) {
		t.Fatalf("expected last rotation metadata, got %+v", track.LastRotation)
	}
	if track.Bounds == nil || !float32Equal(track.Bounds.Min.X, 1) || !float32Equal(track.Bounds.Max.Z, 9) {
		t.Fatalf("expected movement bounds, got %+v", track.Bounds)
	}
	if math.Abs(track.Distance-math.Sqrt(108)) > 0.0001 {
		t.Fatalf("expected travel distance across position updates, got %f", track.Distance)
	}
}

func TestMovementRecordAtUsesStableZeroActorPrefix(t *testing.T) {
	prefix := []byte{0x60, 0x73, 0x85, 0xFE, 0x40, 0x04, 0x0C, 0x00}
	buf := append([]byte{}, prefix...)
	buf = append(buf, make([]byte, 8)...)
	buf = append(buf, movementPositionProp...)
	buf = append(buf, 0x8E, 0x00, 0x00, 0x00)
	buf = append(buf, movementRecordMarker...)
	buf = append(buf, 0xC0, 0x01)
	buf = appendFloat32(buf, 1)
	buf = appendFloat32(buf, 2)
	buf = appendFloat32(buf, 3)
	buf = append(buf, movementRecordTail...)

	actor, changed, vec, ok := movementRecordAt(buf, 16, map[string]int{string(prefix): movementZeroActorPrefixThreshold})
	if !ok {
		t.Fatal("expected movement record to parse")
	}
	if changed != "position" || !float32Equal(vec.Z, 3) {
		t.Fatalf("unexpected parsed record: changed=%s vec=%+v", changed, vec)
	}
	if string(actor) != string(prefix) {
		t.Fatalf("expected zero actor to use stable prefix, got %x want %x", actor, prefix)
	}
}

func TestMovementRecordAtKeepsZeroActorWithoutStablePrefix(t *testing.T) {
	prefix := []byte{0x60, 0x73, 0x85, 0xFE, 0x40, 0x04, 0x0C, 0x00}
	buf := append([]byte{}, prefix...)
	buf = append(buf, make([]byte, 8)...)
	buf = append(buf, movementPositionProp...)
	buf = append(buf, 0x8E, 0x00, 0x00, 0x00)
	buf = append(buf, movementRecordMarker...)
	buf = append(buf, 0xC0, 0x01)
	buf = appendFloat32(buf, 1)
	buf = appendFloat32(buf, 2)
	buf = appendFloat32(buf, 3)
	buf = append(buf, movementRecordTail...)

	actor, _, _, ok := movementRecordAt(buf, 16, map[string]int{string(prefix): movementZeroActorPrefixThreshold - 1})
	if !ok {
		t.Fatal("expected movement record to parse")
	}
	if !movementActorKeyIsZero(actor) {
		t.Fatalf("expected zero actor to remain zero without stable prefix, got %x", actor)
	}
}
func TestMovementDataPrimaryTracks(t *testing.T) {
	actorA := []byte{1, 0, 0, 0, 0, 0, 0, 0}
	actorB := []byte{2, 0, 0, 0, 0, 0, 0, 0}
	junk := []byte{3, 0, 0, 0, 0, 0, 0, 0}
	buf := make([]byte, 0)
	for i := 0; i < 22; i++ {
		buf = append(buf, movementRecord(actorA, movementPositionProp, float32(i), 1, 2)...)
	}
	for i := 0; i < 21; i++ {
		buf = append(buf, movementRecord(actorB, movementPositionProp, float32(i), 3, 4)...)
	}
	for i := 0; i < 5; i++ {
		buf = append(buf, movementRecord(junk, movementPositionProp, float32(i), 5, 6)...)
	}

	r := Reader{
		b: buf,
		Header: Header{
			CodeVersion: Y10S3_1,
			Players: []Player{
				{Username: "a"},
				{Username: "b"},
			},
		},
	}

	out := r.MovementData()
	if len(out.PrimaryTracks) != 2 {
		t.Fatalf("expected 2 primary tracks, got %d", len(out.PrimaryTracks))
	}
	if !out.Tracks[0].LikelyPlayer || !out.Tracks[1].LikelyPlayer {
		t.Fatalf("expected top tracks to be marked likely players: %+v", out.Tracks[:2])
	}
	if out.Tracks[2].LikelyPlayer {
		t.Fatalf("expected junk track to remain unmarked: %+v", out.Tracks[2])
	}
	if out.PrimaryTracks[0].ActorID != out.Tracks[0].ActorID || out.PrimaryTracks[1].ActorID != out.Tracks[1].ActorID {
		t.Fatalf("primary track order mismatch: %+v vs %+v", out.PrimaryTracks, out.Tracks[:2])
	}
	if out.PrimaryTracks[0].Label != "P01" || out.PrimaryTracks[1].Label != "P02" {
		t.Fatalf("expected primary labels, got %+v", out.PrimaryTracks)
	}
}

func TestMovementDataPrimaryTrackGrouping(t *testing.T) {
	buf := make([]byte, 0)
	for actor := 0; actor < 5; actor++ {
		id := []byte{byte(actor + 1), 0, 0, 0, 0, 0, 0, 0}
		for step := 0; step < 20; step++ {
			buf = append(buf, movementRecord(id, movementPositionProp, float32(actor), float32(step)*0.05, 1)...)
		}
	}
	for actor := 0; actor < 5; actor++ {
		id := []byte{byte(actor + 21), 0, 0, 0, 0, 0, 0, 0}
		for step := 0; step < 20; step++ {
			buf = append(buf, movementRecord(id, movementPositionProp, 100+float32(actor*4), float32(step), 2)...)
		}
	}

	r := Reader{
		b: buf,
		Header: Header{
			CodeVersion: Y10S3_1,
			Teams: [2]Team{
				{Role: Attack},
				{Role: Defense},
			},
			Players: []Player{
				{Username: "a1", TeamIndex: 0},
				{Username: "a2", TeamIndex: 0},
				{Username: "a3", TeamIndex: 0},
				{Username: "a4", TeamIndex: 0},
				{Username: "a5", TeamIndex: 0},
				{Username: "d1", TeamIndex: 1},
				{Username: "d2", TeamIndex: 1},
				{Username: "d3", TeamIndex: 1},
				{Username: "d4", TeamIndex: 1},
				{Username: "d5", TeamIndex: 1},
			},
		},
	}

	out := r.MovementData()
	if len(out.PrimaryTracks) != 10 {
		t.Fatalf("expected 10 primary tracks, got %d", len(out.PrimaryTracks))
	}
	attackCount := 0
	defenseCount := 0
	for _, track := range out.PrimaryTracks {
		if track.StartGroup == 0 {
			t.Fatalf("expected start group assignment, got %+v", track)
		}
		if track.TeamRoleGuess == Attack {
			attackCount++
		}
		if track.TeamRoleGuess == Defense {
			defenseCount++
		}
	}
	if attackCount != 5 || defenseCount != 5 {
		t.Fatalf("expected 5 attack and 5 defense guesses, got attack=%d defense=%d", attackCount, defenseCount)
	}
}

func TestGuessMovementTrackPlayers(t *testing.T) {
	tracks := []MovementTrack{
		{
			ActorID:       "atk-live",
			TeamRoleGuess: Attack,
			FirstOffset:   10,
			LastOffset:    220,
			Samples: []MovementSample{
				{Offset: 90, Position: &Vector3{X: 0, Y: 0, Z: 0}},
				{Offset: 100, Position: &Vector3{X: 1, Y: 0, Z: 0}},
			},
		},
		{
			ActorID:       "atk-dead",
			TeamRoleGuess: Attack,
			FirstOffset:   10,
			LastOffset:    130,
			Samples: []MovementSample{
				{Offset: 90, Position: &Vector3{X: 9, Y: 0, Z: 0}},
				{Offset: 120, Position: &Vector3{X: 10, Y: 0, Z: 0}},
			},
		},
		{
			ActorID:       "def-dead",
			TeamRoleGuess: Defense,
			FirstOffset:   10,
			LastOffset:    110,
			Samples: []MovementSample{
				{Offset: 90, Position: &Vector3{X: 2, Y: 0, Z: 0}},
				{Offset: 100, Position: &Vector3{X: 2, Y: 0, Z: 0}},
			},
		},
		{
			ActorID:       "def-live",
			TeamRoleGuess: Defense,
			FirstOffset:   10,
			LastOffset:    220,
			Samples: []MovementSample{
				{Offset: 90, Position: &Vector3{X: 11, Y: 0, Z: 0}},
				{Offset: 120, Position: &Vector3{X: 11, Y: 0, Z: 0}},
			},
		},
	}
	header := Header{
		Teams: [2]Team{{Role: Attack}, {Role: Defense}},
		Players: []Player{
			{Username: "A1", TeamIndex: 0, Operator: Ash, Spawn: "Spawn A"},
			{Username: "A2", TeamIndex: 0, Operator: Buck, Spawn: "Spawn B"},
			{Username: "D1", TeamIndex: 1, Operator: Smoke, Spawn: "Site"},
			{Username: "D2", TeamIndex: 1, Operator: Bandit, Spawn: "Site"},
		},
	}
	feedback := []MatchUpdate{
		{Type: Kill, Username: "A1", Target: "D2", offset: 100},
		{Type: Kill, Username: "D1", Target: "A2", offset: 120},
	}

	guessMovementTrackPlayers(header, feedback, tracks)

	if tracks[0].PlayerNameGuess != "A1" || tracks[0].PlayerGuessSource != "kill-proximity" {
		t.Fatalf("expected live attack track to map to A1 via kill proximity, got %+v", tracks[0])
	}
	if tracks[1].PlayerNameGuess != "A2" || tracks[1].PlayerGuessSource != "death-order" {
		t.Fatalf("expected dead attack track to map to A2 via death order, got %+v", tracks[1])
	}
	if tracks[2].PlayerNameGuess != "D2" || tracks[2].PlayerGuessSource != "death-order" {
		t.Fatalf("expected dead defense track to map to D2 via death order, got %+v", tracks[2])
	}
	if tracks[3].PlayerNameGuess != "D1" || tracks[3].PlayerGuessSource != "kill-proximity" {
		t.Fatalf("expected live defense track to map to D1 via kill proximity, got %+v", tracks[3])
	}
	if tracks[0].OperatorGuess != Ash || tracks[3].OperatorGuess != Smoke {
		t.Fatalf("expected operator guesses to be copied across, got atk=%v def=%v", tracks[0].OperatorGuess, tracks[3].OperatorGuess)
	}
}
func TestMovementDataDiscoversNonDefaultPropIDs(t *testing.T) {
	actor := []byte{1, 0, 0, 0, 0, 0, 0, 0}
	positionProp := []byte{0x00, 0x00, 0x00, 0x00, 0x44, 0x33, 0x22, 0x11, 0x00, 0x00, 0x00, 0x00}
	rotationProp := []byte{0x00, 0x00, 0x00, 0x00, 0x88, 0x77, 0x66, 0x55, 0x10, 0x00, 0x00, 0x00}
	buf := make([]byte, 0)
	buf = append(buf, movementTimePacket(90)...)
	buf = append(buf, movementRecord(actor, positionProp, 1, 2, 3)...)
	buf = append(buf, movementRecord(actor, rotationProp, 4, 5, 6)...)
	buf = append(buf, movementRecord(actor, positionProp, 7, 8, 9)...)

	r := Reader{
		b: buf,
		Header: Header{
			CodeVersion: Y10S3_1,
			Players:     []Player{{Username: "a"}},
		},
	}

	out := r.MovementData()
	if out.PositionPropID != hex.EncodeToString(positionProp) {
		t.Fatalf("expected discovered position prop %x, got %s", positionProp, out.PositionPropID)
	}
	if out.RotationPropID != hex.EncodeToString(rotationProp) {
		t.Fatalf("expected discovered rotation prop %x, got %s", rotationProp, out.RotationPropID)
	}
	if len(out.Tracks) != 1 || out.Tracks[0].PositionSamples != 2 || out.Tracks[0].RotationSamples != 1 {
		t.Fatalf("unexpected discovered track output: %+v", out.Tracks)
	}
}

func TestMovementDataIncludesUsagePercentages(t *testing.T) {
	actor := []byte{1, 0, 0, 0, 0, 0, 0, 0}
	positionProp := []byte{0x00, 0x00, 0x00, 0x00, 0x44, 0x33, 0x22, 0x11, 0x00, 0x00, 0x00, 0x00}
	rotationProp := []byte{0x00, 0x00, 0x00, 0x00, 0x88, 0x77, 0x66, 0x55, 0x10, 0x00, 0x00, 0x00}
	extraProp := []byte{0x00, 0x00, 0x00, 0x00, 0x99, 0x77, 0x66, 0x55, 0x10, 0x00, 0x00, 0x00}
	buf := make([]byte, 0)
	buf = append(buf, movementRecord(actor, positionProp, 1, 2, 3)...)
	buf = append(buf, movementRecord(actor, rotationProp, 4, 5, 6)...)
	buf = append(buf, movementRecord(actor, positionProp, 7, 8, 9)...)
	buf = append(buf, movementRecord(actor, extraProp, 10, 11, 12)...)

	r := Reader{
		b: buf,
		Header: Header{
			CodeVersion: Y10S3_1,
			Players:     []Player{{Username: "a"}},
		},
	}

	out := r.MovementData()
	if out.Usage == nil {
		t.Fatal("expected usage block in movement output")
	}
	if out.Usage.TotalCandidatePackets != 4 {
		t.Fatalf("expected 4 candidate packets, got %+v", out.Usage)
	}
	if out.Usage.SelectedPropPackets != 3 {
		t.Fatalf("expected 3 selected prop packets, got %+v", out.Usage)
	}
	if out.Usage.ExportedSamples != 3 || out.Usage.PrimarySamples != 3 {
		t.Fatalf("expected 3 exported and primary samples, got %+v", out.Usage)
	}
	if math.Abs(out.Usage.ReplayUsagePercent-75) > 0.0001 {
		t.Fatalf("expected replay usage 75%%, got %+v", out.Usage)
	}
	if math.Abs(out.Usage.SelectedPropUsagePercent-100) > 0.0001 {
		t.Fatalf("expected selected prop usage 100%%, got %+v", out.Usage)
	}
}

func TestMovementDataOptionsRespectsPropOverrides(t *testing.T) {
	actor := []byte{1, 0, 0, 0, 0, 0, 0, 0}
	altPositionProp := []byte{0x00, 0x00, 0x00, 0x00, 0x44, 0x33, 0x22, 0x11, 0x00, 0x00, 0x00, 0x00}
	altRotationProp := []byte{0x00, 0x00, 0x00, 0x00, 0x88, 0x77, 0x66, 0x55, 0x10, 0x00, 0x00, 0x00}
	buf := make([]byte, 0)
	buf = append(buf, movementRecord(actor, altPositionProp, 1, 2, 3)...)
	buf = append(buf, movementRecord(actor, altRotationProp, 4, 5, 6)...)
	buf = append(buf, movementRecord(actor, altPositionProp, 7, 8, 9)...)
	buf = append(buf, movementRecord(actor, movementPositionProp, 10, 11, 12)...)
	buf = append(buf, movementRecord(actor, movementRotationProp, 13, 14, 15)...)

	r := Reader{
		b: buf,
		Header: Header{
			CodeVersion: Y10S3_1,
			Players:     []Player{{Username: "a"}},
		},
	}

	out := r.MovementDataOptions(MovementOptions{
		PositionPropID: hex.EncodeToString(movementPositionProp),
		RotationPropID: hex.EncodeToString(movementRotationProp),
	})
	if out.PositionPropID != hex.EncodeToString(movementPositionProp) {
		t.Fatalf("expected overridden position prop %x, got %s", movementPositionProp, out.PositionPropID)
	}
	if out.RotationPropID != hex.EncodeToString(movementRotationProp) {
		t.Fatalf("expected overridden rotation prop %x, got %s", movementRotationProp, out.RotationPropID)
	}
	if len(out.Tracks) != 1 || out.Tracks[0].PositionSamples != 1 || out.Tracks[0].RotationSamples != 1 {
		t.Fatalf("expected override to parse only the default prop pair, got %+v", out.Tracks)
	}
	if out.Tracks[0].LastPosition == nil || !float32Equal(out.Tracks[0].LastPosition.X, 10) {
		t.Fatalf("expected overridden position sample, got %+v", out.Tracks[0])
	}
	if out.Usage == nil || out.Usage.SelectedPropPackets != 2 || out.Usage.ExportedSamples != 2 {
		t.Fatalf("expected usage to follow overridden props, got %+v", out.Usage)
	}
}

func TestRadiansToWrappedDegrees(t *testing.T) {
	cases := []struct {
		name  string
		input float32
		want  float32
	}{
		{name: "identity", input: float32(math.Pi / 2), want: 90},
		{name: "positive wrap", input: float32((10 * math.Pi) + 0.5), want: float32((0.5 * 180) / math.Pi)},
		{name: "negative wrap", input: float32((-8 * math.Pi) - 0.25), want: float32((-0.25 * 180) / math.Pi)},
	}

	for _, tc := range cases {
		if got := radiansToWrappedDegrees(tc.input); !float32Equal(got, tc.want) {
			t.Fatalf("%s: expected %.4f, got %.4f", tc.name, tc.want, got)
		}
	}
}
func TestMovementDataIgnoresNearZeroPropIDs(t *testing.T) {
	actorA := []byte{1, 0, 0, 0, 0, 0, 0, 0}
	actorB := []byte{2, 0, 0, 0, 0, 0, 0, 0}
	falsePositionProp := []byte{0x00, 0x00, 0x00, 0x00, 0x91, 0x33, 0x22, 0x11, 0x00, 0x00, 0x00, 0x00}
	falseRotationProp := []byte{0x00, 0x00, 0x00, 0x00, 0x92, 0x33, 0x22, 0x11, 0x00, 0x00, 0x00, 0x00}
	realPositionProp := []byte{0x00, 0x00, 0x00, 0x00, 0x93, 0x33, 0x22, 0x11, 0x00, 0x00, 0x00, 0x00}
	realRotationProp := []byte{0x00, 0x00, 0x00, 0x00, 0x94, 0x33, 0x22, 0x11, 0x00, 0x00, 0x00, 0x00}
	buf := make([]byte, 0)
	for i := 0; i < 25; i++ {
		buf = append(buf, movementRecord(actorA, falsePositionProp, 0.0001, 0.0002, 0.0003)...)
		buf = append(buf, movementRecord(actorB, falsePositionProp, 0.0001, 0.0002, 0.0003)...)
		buf = append(buf, movementRecord(actorA, falseRotationProp, 0.0004, 0.0005, 0.0006)...)
	}
	for i := 0; i < 20; i++ {
		buf = append(buf, movementRecord(actorA, realPositionProp, float32(i), float32(i)+1, float32(i)+2)...)
		buf = append(buf, movementRecord(actorA, realRotationProp, float32(i)+3, float32(i)+4, float32(i)+5)...)
	}

	r := Reader{
		b: buf,
		Header: Header{
			CodeVersion: Y10S3_1,
			Players:     []Player{{Username: "a"}},
		},
	}

	out := r.MovementData()
	if out.PositionPropID != hex.EncodeToString(realPositionProp) {
		t.Fatalf("expected real position prop %x, got %s", realPositionProp, out.PositionPropID)
	}
	if out.RotationPropID != hex.EncodeToString(realRotationProp) {
		t.Fatalf("expected real rotation prop %x, got %s", realRotationProp, out.RotationPropID)
	}
}
func TestMergeSoloRotationTracks(t *testing.T) {
	tracks := []MovementTrack{
		{
			ActorID:         "pos",
			PositionSamples: 1,
			RotationSamples: 1,
			Samples: []MovementSample{
				{Offset: 10, Changed: "position", Position: &Vector3{X: 1, Y: 2, Z: 3}},
				{Offset: 20, Changed: "rotation", Position: &Vector3{X: 1, Y: 2, Z: 3}, Rotation: &Vector3{X: 0.1, Y: 0.2, Z: 0.3}},
			},
		},
		{
			ActorID:         "rot",
			RotationSamples: 2,
			Samples: []MovementSample{
				{Offset: 30, Changed: "rotation", Rotation: &Vector3{X: 0.4, Y: 0.5, Z: 0.6}},
				{Offset: 40, Changed: "rotation", Rotation: &Vector3{X: 0.7, Y: 0.8, Z: 0.9}},
			},
		},
	}

	merged := mergeSoloRotationTracks(Header{Players: []Player{{Username: "dyn4mic"}}}, tracks)
	if len(merged) != 1 {
		t.Fatalf("expected a single merged track, got %d", len(merged))
	}
	track := merged[0]
	if track.PositionSamples != 1 || track.RotationSamples != 3 {
		t.Fatalf("unexpected merged sample counts: %+v", track)
	}
	if len(track.Samples) != 4 || track.LastRotation == nil || !float32Equal(track.LastRotation.Z, 0.9) {
		t.Fatalf("expected merged samples to preserve latest rotation, got %+v", track)
	}
	if track.Samples[3].Position == nil || !float32Equal(track.Samples[3].Position.X, 1) {
		t.Fatalf("expected merged samples to carry forward position, got %+v", track.Samples[3])
	}
}

func TestMovementDataY11SinglePropCarriesPositionAndRotation(t *testing.T) {
	actor := []byte{1, 0, 0, 0, 0, 0, 0, 0}
	prop := []byte{0x00, 0x00, 0x00, 0x00, 0x57, 0xE4, 0x05, 0xF0, 0x00, 0x00, 0x00, 0x00}
	buf := make([]byte, 0)
	buf = append(buf, movementY11PositionRecord(actor, prop, 45.3, -23.8058, 0.0135)...)
	buf = append(buf, movementY11QuaternionRecord(actor, prop, 45)...)
	buf = append(buf, movementY11QuaternionRecord(actor, prop, 90)...)

	r := Reader{
		b: buf,
		Header: Header{
			CodeVersion: Y11S1Alpha3,
			Players:     []Player{{Username: "dyn4mic"}},
		},
	}

	out := r.MovementData()
	if out.PositionPropID != hex.EncodeToString(prop) {
		t.Fatalf("expected discovered Y11 position prop %x, got %s", prop, out.PositionPropID)
	}
	if out.RotationPropID != hex.EncodeToString(prop) {
		t.Fatalf("expected discovered Y11 rotation prop %x, got %s", prop, out.RotationPropID)
	}
	if len(out.PrimaryTracks) != 1 {
		t.Fatalf("expected 1 primary track, got %d", len(out.PrimaryTracks))
	}
	track := out.PrimaryTracks[0]
	if track.PositionSamples != 1 {
		t.Fatalf("expected 1 position sample, got %d", track.PositionSamples)
	}
	if track.RotationSamples != 2 {
		t.Fatalf("expected 2 rotation samples, got %d", track.RotationSamples)
	}
	if track.PlayerNameGuess != "dyn4mic" || track.PlayerGuessSource != "single-player" {
		t.Fatalf("expected single-player labeling, got %+v", track)
	}
	if track.FirstPosition == nil || !float32Equal(track.FirstPosition.X, 45.3) {
		t.Fatalf("expected first position to come from the Y11 primary vector, got %+v", track.FirstPosition)
	}
	if track.LastRotation == nil || !float32Equal(radiansToWrappedDegrees(track.LastRotation.Z), 90) {
		t.Fatalf("expected quaternion-derived yaw to reach 90 degrees, got %+v", track.LastRotation)
	}
	if len(track.Samples) != 3 || track.Samples[1].Rotation == nil || track.Samples[1].RotationDegrees == nil {
		t.Fatalf("expected merged Y11 samples to carry rotation, got %+v", track.Samples)
	}
	if out.Usage == nil {
		t.Fatal("expected usage block for Y11 export")
	}
	if out.Usage.SelectedPropPackets != 3 || out.Usage.PositionPropPackets != 3 || out.Usage.RotationPropPackets != 3 {
		t.Fatalf("expected same Y11 prop to count each packet once for selected usage, got %+v", out.Usage)
	}
	if out.Usage.ExportedSamples != 3 || math.Abs(out.Usage.SelectedPropUsagePercent-100) > 0.0001 {
		t.Fatalf("expected full Y11 selected-prop usage, got %+v", out.Usage)
	}
}

func TestMovementY11CandidatePositionFallsBackToSecondary(t *testing.T) {
	candidate := movementCandidate{
		prop:         []byte{0x00, 0x00, 0x00, 0x00, 0x57, 0xE4, 0x05, 0xF0, 0x00, 0x00, 0x00, 0x00},
		actor:        []byte{1, 0, 0, 0, 0, 0, 0, 0},
		tailZero:     true,
		primary:      Vector3{X: 5875893300000, Y: 0, Z: 0},
		secondary:    Vector3{X: 45.3, Y: -23.8, Z: 1.35},
		hasSecondary: true,
	}

	value, ok := movementY11CandidatePosition(candidate)
	if !ok {
		t.Fatal("expected secondary Y11 vector to be accepted when primary is out of bounds")
	}
	if !float32Equal(value.X, 45.3) || !float32Equal(value.Y, -23.8) || !float32Equal(value.Z, 1.35) {
		t.Fatalf("unexpected fallback position: %+v", value)
	}
	if !movementY11CandidateIsPosition(candidate) {
		t.Fatalf("expected candidate to count as a Y11 position update: %+v", candidate)
	}
}

func TestMovementRecordAtWithPropsY11DoesNotTreatRotationPacketsAsPositionWhenPropsDiffer(t *testing.T) {
	actor := []byte{1, 0, 0, 0, 0, 0, 0, 0}
	positionProp := []byte{0x00, 0x00, 0x00, 0x00, 0x57, 0xE4, 0x05, 0xF0, 0x00, 0x00, 0x00, 0x00}
	rotationProp := []byte{0x00, 0x00, 0x00, 0x00, 0x58, 0xE4, 0x05, 0xF0, 0x00, 0x00, 0x00, 0x00}

	buf := movementY11QuaternionRecord(actor, positionProp, 90)
	gotActor, changed, _, ok := movementRecordAtWithProps(Y11S1Alpha3, buf, 8, positionProp, rotationProp, nil)
	if ok {
		t.Fatalf("expected quaternion packet on position prop to be ignored, got actor=%x changed=%s", gotActor, changed)
	}
}

func TestMovementY11CandidateQuaternionFallsBackToImplicitSecondaryXYZ(t *testing.T) {
	candidate := movementCandidate{
		prop:         []byte{0x00, 0x00, 0x00, 0x00, 0x82, 0x09, 0x06, 0xF0, 0x00, 0x00, 0x00, 0x00},
		actor:        []byte{1, 0, 0, 0, 0, 0, 0, 0},
		tailZero:     false,
		primary:      Vector3{X: 0, Y: 0, Z: 0},
		secondary:    Vector3{X: 0, Y: 0, Z: float32(math.Sin(math.Pi / 4))},
		hasSecondary: true,
	}

	if !movementY11CandidateIsRotation(candidate) {
		t.Fatalf("expected implicit secondary quaternion xyz to count as rotation: %+v", candidate)
	}
	quat, ok := movementY11CandidateQuaternion(candidate)
	if !ok {
		t.Fatalf("expected implicit secondary quaternion xyz to decode: %+v", candidate)
	}
	if !float32Equal(quat.W, float32(math.Cos(math.Pi/4))) {
		t.Fatalf("expected implicit quaternion W reconstruction, got %+v", quat)
	}
	rotation := movementQuaternionToEuler(quat)
	if !float32Equal(radiansToWrappedDegrees(rotation.Z), 90) {
		t.Fatalf("expected implicit quaternion to decode to 90 yaw, got %+v", rotation)
	}
}

func TestMovementNormalizedPackedQuaternion(t *testing.T) {
	quat, ok := movementNormalizedPackedQuaternion(movementQuaternion{
		Z: float32(math.Sin(math.Pi / 4)),
		W: float32(math.Cos(math.Pi / 4)),
	})
	if !ok {
		t.Fatal("expected packed quaternion to normalize")
	}
	rotation := movementQuaternionToEuler(quat)
	if !float32Equal(radiansToWrappedDegrees(rotation.Z), 90) {
		t.Fatalf("expected normalized packed quaternion to keep 90 yaw, got %+v", rotation)
	}
	if _, ok := movementNormalizedPackedQuaternion(movementQuaternion{X: 4, Y: 4, Z: 4, W: 4}); ok {
		t.Fatal("expected oversized packed quaternion to be rejected")
	}
}

func TestMovementTrackQualityPositionScorePrefersDensePositionStream(t *testing.T) {
	positionHeavy := movementTrackQuality{
		longTracks:     12,
		balancedTracks: 4,
		topMerged:      2064,
		topPosition:    1920,
		topRotation:    144,
	}
	balancedButSparse := movementTrackQuality{
		longTracks:     15,
		balancedTracks: 12,
		topMerged:      816,
		topPosition:    498,
		topRotation:    318,
	}

	if got, want := movementTrackQualityPositionScore(10, positionHeavy), movementTrackQualityPositionScore(10, balancedButSparse); got <= want {
		t.Fatalf("expected dense position stream to win position scoring, got dense=%f sparse=%f", got, want)
	}
	if got, want := movementTrackQualityScore(10, positionHeavy), movementTrackQualityScore(10, balancedButSparse); got <= want {
		t.Fatalf("expected dense mixed pair to win overall scoring, got dense=%f sparse=%f", got, want)
	}
}

func TestMovementY11RankRotationChoicesUsesBestPositionPairing(t *testing.T) {
	qualities := map[string]movementTrackQuality{
		"posA:rotSelf": {
			topRotation: 4,
			topMerged:   8,
		},
		"posB:rotSelf": {
			topRotation: 6,
			topMerged:   12,
		},
		"posA:rotDense": {
			topRotation: 18,
			topMerged:   20,
		},
		"posB:rotDense": {
			topRotation:    640,
			topMerged:      780,
			balancedTracks: 1,
		},
	}
	choices := movementY11RankRotationChoices(1, []string{"posA", "posB"}, []string{"rotSelf", "rotDense"}, func(positionID string, rotationID string) movementTrackQuality {
		return qualities[positionID+":"+rotationID]
	}, 2)
	if len(choices) != 2 {
		t.Fatalf("expected two rotation choices, got %d", len(choices))
	}
	if got, want := choices[0], "rotDense"; got != want {
		t.Fatalf("expected dense rotation prop to win when paired with best position, got %q want %q", got, want)
	}
}

func TestMovementDirectionTargetVectorStreamsDeriveHeadingFromPlayerPosition(t *testing.T) {
	actor := []byte{1, 0, 0, 0, 0, 0, 0, 0}
	prop := []byte{0x00, 0x00, 0x02, 0x00, 0x0C, 0x0C, 0x03, 0xF0, 0x00, 0x00, 0x00, 0x00}
	buf := make([]byte, 0)
	samples := []struct {
		position Vector3
		target   Vector3
	}{
		{position: Vector3{X: 10, Y: 5}, target: Vector3{X: 20, Y: 5}},
		{position: Vector3{X: 10, Y: 10}, target: Vector3{X: 10, Y: 20}},
		{position: Vector3{X: 10, Y: 5}, target: Vector3{X: 20, Y: 5}},
		{position: Vector3{X: 10, Y: 10}, target: Vector3{X: 10, Y: 20}},
		{position: Vector3{X: 10, Y: 5}, target: Vector3{X: 20, Y: 5}},
		{position: Vector3{X: 10, Y: 10}, target: Vector3{X: 10, Y: 20}},
		{position: Vector3{X: 10, Y: 5}, target: Vector3{X: 20, Y: 5}},
		{position: Vector3{X: 10, Y: 10}, target: Vector3{X: 10, Y: 20}},
	}
	base := MovementTrack{
		ActorID: hex.EncodeToString(actor),
		Samples: make([]MovementSample, 0, len(samples)),
	}
	for index, sample := range samples {
		rec := movementY11RecordBase(actor, prop)
		binary.LittleEndian.PutUint32(rec[30:34], math.Float32bits(sample.target.X))
		binary.LittleEndian.PutUint32(rec[34:38], math.Float32bits(sample.target.Y))
		binary.LittleEndian.PutUint32(rec[38:42], math.Float32bits(sample.target.Z))
		offset := len(buf)
		buf = append(buf, rec...)
		base.Samples = append(base.Samples, MovementSample{Offset: offset, Position: &samples[index].position})
	}
	streams := movementDirectionTargetVectorStreams(
		Header{CodeVersion: Y11S1Alpha3, Players: []Player{{Username: "dyn4mic"}}},
		buf,
		0,
		nil,
		map[string]bool{hex.EncodeToString(prop): true},
		nil,
		base,
	)
	var target *movementDirectionStream
	for index := range streams {
		if streams[index].source == "target-pos-xy" && streams[index].actorID == hex.EncodeToString(actor) {
			target = &streams[index]
			break
		}
	}
	if target == nil {
		t.Fatal("expected target-pos-xy stream")
	}
	if len(target.samples) != 8 {
		t.Fatalf("expected 8 target samples, got %d", len(target.samples))
	}
	if got := movementWrapDegrees(target.samples[0].angle); math.Abs(got-0) > 0.001 {
		t.Fatalf("expected first target heading 0deg, got %f", got)
	}
	if got := movementWrapDegrees(target.samples[1].angle); math.Abs(got-90) > 0.001 {
		t.Fatalf("expected second target heading 90deg, got %f", got)
	}
}

func TestMovementDirectionFinalizeSoloScalarStreamsFusesBeforeIntegrating(t *testing.T) {
	streams := []movementDirectionStream{
		{
			source:       "s16",
			propID:       "prop",
			actorID:      "actor-a",
			vectorOffset: 26,
			axisA:        "s16",
			samples: []movementDirectionAngleSample{
				{offset: 0, angle: 10},
				{offset: 10, angle: 20},
			},
		},
		{
			source:       "s16",
			propID:       "prop",
			actorID:      "actor-b",
			vectorOffset: 26,
			axisA:        "s16",
			samples: []movementDirectionAngleSample{
				{offset: 20, angle: 20},
				{offset: 30, angle: 30},
			},
		},
	}
	out := movementDirectionFinalizeSoloScalarStreams(streams, 2)
	foundRawFused := false
	foundDeltaFused := false
	for _, stream := range out {
		if stream.source == "s16-actor-fused" {
			foundRawFused = true
			if len(stream.samples) != 4 {
				t.Fatalf("expected raw fused stream to keep 4 samples, got %d", len(stream.samples))
			}
		}
		if stream.source == "s16-actor-fused-delta-integrated" {
			foundDeltaFused = true
			if len(stream.samples) != 4 {
				t.Fatalf("expected fused delta-integrated stream to keep 4 samples, got %d", len(stream.samples))
			}
			if got := stream.samples[len(stream.samples)-1].angle; math.Abs(got-20) > 0.001 {
				t.Fatalf("expected final fused delta angle 20, got %f", got)
			}
		}
	}
	if !foundRawFused {
		t.Fatal("expected raw actor-fused stream")
	}
	if !foundDeltaFused {
		t.Fatal("expected actor-fused delta-integrated stream")
	}
}

func TestMovementDirectionPropFusedDeltaStreamsMergeAcrossActors(t *testing.T) {
	streams := []movementDirectionStream{
		{
			source:       "s16",
			propID:       "prop",
			actorID:      "actor-a",
			vectorOffset: 26,
			axisA:        "s16",
			samples: []movementDirectionAngleSample{
				{offset: 0, angle: 2},
				{offset: 10, angle: 3},
			},
		},
		{
			source:       "s16",
			propID:       "prop",
			actorID:      "actor-b",
			vectorOffset: 26,
			axisA:        "s16",
			samples: []movementDirectionAngleSample{
				{offset: 20, angle: 4},
				{offset: 30, angle: 5},
			},
		},
	}
	out := movementDirectionPropFusedDeltaStreams(streams, 2)
	foundRaw := false
	foundDelta := false
	for _, stream := range out {
		if stream.source == "s16-prop-fused" {
			foundRaw = true
			if len(stream.samples) != 4 {
				t.Fatalf("expected prop-fused raw stream to keep 4 samples, got %d", len(stream.samples))
			}
		}
		if stream.source == "s16-prop-fused-delta-integrated" {
			foundDelta = true
			if len(stream.samples) != 4 {
				t.Fatalf("expected prop-fused delta stream to keep 4 samples, got %d", len(stream.samples))
			}
			if got := stream.samples[len(stream.samples)-1].angle; math.Abs(got-3) > 0.001 {
				t.Fatalf("expected final prop-fused delta angle 3, got %f", got)
			}
		}
	}
	if !foundRaw {
		t.Fatal("expected prop-fused raw stream")
	}
	if !foundDelta {
		t.Fatal("expected prop-fused delta-integrated stream")
	}
}

func TestMovementTrackEffectivePositionSamplesPenalizesStaticAnchors(t *testing.T) {
	static := MovementTrack{PositionSamples: 250, Distance: 0.2}
	moving := MovementTrack{PositionSamples: 250, Distance: 12}

	if got := movementTrackEffectivePositionSamples(static); got != 0 {
		t.Fatalf("expected static dense track to be ignored, got %d", got)
	}
	if got := movementTrackEffectivePositionSamples(moving); got != 250 {
		t.Fatalf("expected moving track to keep its position samples, got %d", got)
	}
}

func TestMovementTrackLooksOriginLike(t *testing.T) {
	if !movementTrackLooksOriginLike(MovementTrack{FirstPosition: &Vector3{X: 0.3, Y: -0.2, Z: 1.1}}) {
		t.Fatal("expected near-origin track to be penalized")
	}
	if movementTrackLooksOriginLike(MovementTrack{FirstPosition: &Vector3{X: -35, Y: -110, Z: 4}}) {
		t.Fatal("did not expect world-space track to look origin-like")
	}
}

func TestMovementBestSameActorDirectionCandidatePrefersMatchingActor(t *testing.T) {
	candidates := []MovementDirectionCandidate{
		{
			ActorID:             "other",
			SampleCount:         50,
			MeanErrorDegrees:    12,
			MedianErrorDegrees:  12,
			MeanCosineAgreement: 0.95,
		},
		{
			ActorID:             "target",
			Source:              "f32-rad",
			SampleCount:         38,
			MeanErrorDegrees:    14,
			MedianErrorDegrees:  15,
			MeanCosineAgreement: 0.96,
		},
		{
			ActorID:             "target",
			Source:              "u16",
			SampleCount:         20,
			MeanErrorDegrees:    8,
			MedianErrorDegrees:  8,
			MeanCosineAgreement: 0.99,
		},
	}
	best := movementBestSameActorDirectionCandidate(candidates, "target", 8)
	if best == nil {
		t.Fatal("expected a best same-actor candidate")
	}
	if best.ActorID != "target" {
		t.Fatalf("unexpected best same-actor candidate: %#v", best)
	}
	if !best.MakesSense {
		t.Fatal("expected selected same-actor candidate to be marked as sensible")
	}
}

func TestMovementDirectionFloat64StreamsFindSoloScalarCandidates(t *testing.T) {
	actor := []byte{1, 0, 0, 0, 0, 0, 0, 0}
	prop := []byte{0x00, 0x00, 0x02, 0x00, 0x73, 0x0E, 0x03, 0xF0, 0x00, 0x00, 0x00, 0x00}
	buf := make([]byte, 0)
	for i := 0; i < 8; i++ {
		rec := movementY11RecordBase(actor, prop)
		binary.LittleEndian.PutUint64(rec[30:38], math.Float64bits(0.25+float64(i)*0.05))
		buf = append(buf, rec...)
	}

	streams := movementDirectionFloat64Streams(
		Header{CodeVersion: Y11S1Alpha3, Players: []Player{{Username: "dyn4mic"}}},
		buf,
		8,
		nil,
		map[string]bool{hex.EncodeToString(prop): true},
		nil,
	)

	found := false
	for _, stream := range streams {
		if stream.source == "f64-rad" && stream.propID == hex.EncodeToString(prop) && stream.actorID == hex.EncodeToString(actor) && stream.vectorOffset == 22 && len(stream.samples) == 8 {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected solo f64 scalar stream at relative offset 22, got %+v", streams)
	}
}

func TestMovementDirectionFloat32PairStreamsFindSoloPairCandidates(t *testing.T) {
	actor := []byte{1, 0, 0, 0, 0, 0, 0, 0}
	prop := []byte{0x00, 0x00, 0x02, 0x00, 0x0C, 0x0C, 0x03, 0xF0, 0x00, 0x00, 0x00, 0x00}
	buf := make([]byte, 0)
	for i := 0; i < 8; i++ {
		rec := movementY11RecordBase(actor, prop)
		angle := float32((15 + float64(i)*10) * math.Pi / 180)
		binary.LittleEndian.PutUint32(rec[30:34], math.Float32bits(float32(math.Cos(float64(angle)))))
		binary.LittleEndian.PutUint32(rec[34:38], math.Float32bits(float32(math.Sin(float64(angle)))))
		buf = append(buf, rec...)
	}

	streams := movementDirectionFloat32PairStreams(
		Header{CodeVersion: Y11S1Alpha3, Players: []Player{{Username: "dyn4mic"}}},
		buf,
		8,
		nil,
		map[string]bool{hex.EncodeToString(prop): true},
		nil,
	)

	found := false
	for _, stream := range streams {
		if stream.source == "f32-pair" && stream.propID == hex.EncodeToString(prop) && stream.actorID == hex.EncodeToString(actor) && stream.vectorOffset == 22 && len(stream.samples) == 8 {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected solo f32 pair stream at relative offset 22, got %+v", streams)
	}
}

func TestMovementDirectionTargetedAlignedWordDeltaStreamsFindsInt32Lane(t *testing.T) {
	actor := []byte{1, 0, 0, 0, 0, 0, 0, 0}
	prop := []byte{0x00, 0x00, 0x02, 0x00, 0x0C, 0x0C, 0x03, 0xF0, 0x00, 0x00, 0x00, 0x00}
	buf := make([]byte, 0)
	for i := 0; i < 9; i++ {
		rec := movementY11RecordBase(actor, prop)
		binary.LittleEndian.PutUint32(rec[30:34], uint32(int32(1000+(i*25))))
		buf = append(buf, rec...)
	}

	streams := movementDirectionTargetedAlignedWordDeltaStreams(
		Header{CodeVersion: Y11S1Alpha3, Players: []Player{{Username: "dyn4mic"}}},
		buf,
		8,
		nil,
		map[string]bool{hex.EncodeToString(prop): true},
		nil,
	)

	found := false
	for _, stream := range streams {
		if stream.source == "i32-word-delta-integrated" && stream.propID == hex.EncodeToString(prop) && stream.actorID == hex.EncodeToString(actor) && stream.vectorOffset == 22 && len(stream.samples) == 8 {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected aligned-word int32 delta stream at relative offset 22, got %+v", streams)
	}
}

func TestMovementDirectionPropProbeForPropGroupsLaneActors(t *testing.T) {
	streams := []movementDirectionStream{
		{
			source:       "s16-neg-integrated",
			propID:       "dense",
			actorID:      "b",
			vectorOffset: 26,
			axisA:        "yaw",
			samples: []movementDirectionAngleSample{
				{offset: 40, angle: 10},
				{offset: 60, angle: 20},
			},
		},
		{
			source:       "s16-neg-integrated",
			propID:       "dense",
			actorID:      "a",
			vectorOffset: 26,
			axisA:        "yaw",
			samples: []movementDirectionAngleSample{
				{offset: 10, angle: 0},
				{offset: 20, angle: 5},
				{offset: 30, angle: 8},
			},
		},
	}
	candidates := []MovementDirectionCandidate{
		{
			Source:              "s16-neg-integrated",
			PropID:              "dense",
			ActorID:             "a",
			VectorOffset:        26,
			Plane:               "yaw/",
			SampleCount:         3,
			MeanErrorDegrees:    12,
			MeanCosineAgreement: 0.95,
		},
	}

	probe := movementDirectionPropProbeForProp(streams, candidates, "dense")
	if probe == nil {
		t.Fatal("expected prop probe")
	}
	if len(probe.LaneGroups) != 1 {
		t.Fatalf("expected 1 lane group, got %d", len(probe.LaneGroups))
	}
	group := probe.LaneGroups[0]
	if group.TotalSamples != 5 || group.ActorCount != 2 {
		t.Fatalf("unexpected lane group summary: %+v", group)
	}
	if group.BestCandidate == nil || group.BestCandidate.ActorID != "a" {
		t.Fatalf("expected candidate to attach to lane group, got %+v", group.BestCandidate)
	}
	if len(probe.ActorOrder) != 2 || probe.ActorOrder[0].ActorID != "a" || probe.ActorOrder[1].GapFromPreviousOffset != 10 {
		t.Fatalf("unexpected actor order: %+v", probe.ActorOrder)
	}
}

func TestMovementBestDenseFallbackDirectionCandidatePrefersCoverage(t *testing.T) {
	movementSamples := make([]movementDirectionAngleSample, 0, 120)
	for index := 0; index < 120; index++ {
		movementSamples = append(movementSamples, movementDirectionAngleSample{
			sampleIndex: index,
			offset:      index * 100,
			angle:       movementWrapDegrees(-90 + (float64(index) * 1.5)),
		})
	}
	densePredictions := make([]movementDirectionPrediction, 0, 24)
	for index := 0; index < len(movementSamples); index += 5 {
		densePredictions = append(densePredictions, movementDirectionPrediction{
			sampleIndex:     index,
			offset:          movementSamples[index].offset,
			movementDegrees: movementSamples[index].angle,
			viewingDegrees:  movementSamples[index].angle - 18,
		})
	}
	anchorPredictions := make([]movementDirectionPrediction, 0, 8)
	for _, index := range []int{20, 25, 30, 35, 40, 45, 50, 55} {
		anchorPredictions = append(anchorPredictions, movementDirectionPrediction{
			sampleIndex:     index,
			offset:          movementSamples[index].offset,
			movementDegrees: movementSamples[index].angle,
			viewingDegrees:  movementSamples[index].angle + 4,
		})
	}
	denseCandidate := MovementDirectionCandidate{
		Source:              "dense-stream",
		PropID:              "dense-prop",
		ActorID:             "dense-actor",
		VectorOffset:        26,
		Plane:               "dense/",
		Alignment:           "offset",
		SampleCount:         len(densePredictions),
		MeanCosineAgreement: 0.55,
		MeanErrorDegrees:    48,
		MedianErrorDegrees:  38,
	}
	anchorCandidate := MovementDirectionCandidate{
		Source:              "anchor-stream",
		PropID:              "anchor-prop",
		ActorID:             "anchor-actor",
		VectorOffset:        30,
		Plane:               "anchor/",
		Alignment:           "offset",
		SampleCount:         len(anchorPredictions),
		MeanCosineAgreement: 0.98,
		MeanErrorDegrees:    6,
		MedianErrorDegrees:  5,
		MakesSense:          true,
	}
	evals := []movementDirectionCandidateEval{
		{
			candidate:  denseCandidate,
			prediction: densePredictions,
			packetKeys: map[string]bool{
				"dense|1": true,
			},
		},
		{
			candidate:  anchorCandidate,
			prediction: anchorPredictions,
			packetKeys: map[string]bool{
				"anchor|1": true,
			},
		},
	}

	track, derived, packets, actors := movementBestDenseFallbackDirectionCandidate(movementSamples, evals, &denseCandidate, &anchorCandidate)
	if track == nil {
		t.Fatal("expected dense fallback track")
	}
	if track.CoveragePercent < 15 {
		t.Fatalf("expected dense fallback coverage gain, got %+v", *track)
	}
	if track.SampleCount != len(derived) {
		t.Fatalf("expected derived map to match sample count, track=%d derived=%d", track.SampleCount, len(derived))
	}
	if !actors["dense-actor"] || !actors["anchor-actor"] {
		t.Fatalf("expected both dense and anchor actors in fallback, got %+v", actors)
	}
	if !packets["dense|1"] || !packets["anchor|1"] {
		t.Fatalf("expected merged packet keys, got %+v", packets)
	}
	if derived[20] < movementSamples[20].angle-10 || derived[20] > movementSamples[20].angle+10 {
		t.Fatalf("expected anchor alignment near movement angle, got %f want around %f", derived[20], movementSamples[20].angle)
	}
}

func TestMovementBestDenseLocalFragmentDirectionCandidateStitchesRawFragments(t *testing.T) {
	movementSamples := make([]movementDirectionAngleSample, 0, 96)
	for index := 0; index < 96; index++ {
		movementSamples = append(movementSamples, movementDirectionAngleSample{
			sampleIndex: index,
			offset:      index * 100,
			angle:       movementWrapDegrees(-120 + (float64(index) * 2.5)),
		})
	}
	buildPredictions := func(start int, end int, delta float64) []movementDirectionPrediction {
		out := make([]movementDirectionPrediction, 0, end-start)
		for index := start; index < end; index++ {
			out = append(out, movementDirectionPrediction{
				sampleIndex:     index,
				offset:          movementSamples[index].offset,
				movementDegrees: movementSamples[index].angle,
				viewingDegrees:  movementWrapDegrees(movementSamples[index].angle + delta),
			})
		}
		return out
	}
	bestDense := MovementDirectionCandidate{
		Source:              "s16-neg-integrated",
		PropID:              "dense-prop",
		ActorID:             "a",
		VectorOffset:        23,
		Plane:               "s16/",
		Alignment:           "offset",
		SampleCount:         32,
		MeanCosineAgreement: 0.94,
		MeanErrorDegrees:    12,
		MedianErrorDegrees:  10,
		MakesSense:          true,
	}
	second := MovementDirectionCandidate{
		Source:              "s16-neg-integrated",
		PropID:              "dense-prop",
		ActorID:             "b",
		VectorOffset:        24,
		Plane:               "s16/",
		Alignment:           "offset",
		SampleCount:         32,
		MeanCosineAgreement: 0.91,
		MeanErrorDegrees:    14,
		MedianErrorDegrees:  12,
		MakesSense:          true,
	}
	noisy := MovementDirectionCandidate{
		Source:              "s16-neg-integrated",
		PropID:              "dense-prop",
		ActorID:             "c",
		VectorOffset:        25,
		Plane:               "s16/",
		Alignment:           "offset",
		SampleCount:         96,
		MeanCosineAgreement: 0.35,
		MeanErrorDegrees:    58,
		MedianErrorDegrees:  55,
	}
	evals := []movementDirectionCandidateEval{
		{
			candidate:  bestDense,
			prediction: buildPredictions(0, 40, 6),
			packetKeys: map[string]bool{"a": true},
		},
		{
			candidate:  second,
			prediction: buildPredictions(40, 96, 5),
			packetKeys: map[string]bool{"b": true},
		},
		{
			candidate:  noisy,
			prediction: buildPredictions(0, 96, 48),
			packetKeys: map[string]bool{"c": true},
		},
	}
	track, derived, packets, actors := movementBestDenseLocalFragmentDirectionCandidate(movementSamples, evals, &bestDense, nil)
	if track == nil {
		t.Fatal("expected dense local fragment track")
	}
	if track.CoveragePercent < 90 {
		t.Fatalf("expected near-full dense local coverage, got %+v", *track)
	}
	if track.MeanCosineAgreement < 0.95 {
		t.Fatalf("expected strong dense local agreement, got %+v", *track)
	}
	if !actors["a"] || !actors["b"] || actors["c"] {
		t.Fatalf("expected only good fragment actors, got %+v", actors)
	}
	if !packets["a"] || !packets["b"] || packets["c"] {
		t.Fatalf("expected packet keys from stitched fragments only, got %+v", packets)
	}
	if len(derived) != track.SampleCount {
		t.Fatalf("expected derived count to match track sample count, got derived=%d track=%d", len(derived), track.SampleCount)
	}
}

func TestMovementDirectionProjectCandidateTimelineProjectsBeyondMovementSamples(t *testing.T) {
	stream := movementDirectionStream{
		source:       "late-f32-deg-delta-integrated-scalar-pair-double",
		propID:       "prop",
		actorID:      "actor",
		vectorOffset: 46,
		samples: []movementDirectionAngleSample{
			{offset: 100, angle: 10},
			{offset: 200, angle: 20},
			{offset: 300, angle: 30},
			{offset: 400, angle: 40},
		},
	}
	candidate := MovementDirectionCandidate{
		Source:               stream.source,
		PropID:               stream.propID,
		ActorID:              stream.actorID,
		VectorOffset:         stream.vectorOffset,
		Plane:                "+o46/+o50",
		Alignment:            "offset",
		SampleCount:          3,
		AngleScale:           1,
		HeadingOffsetDegrees: 5,
	}
	evals := []movementDirectionCandidateEval{
		{
			candidate: candidate,
			stream:    stream,
		},
	}
	track := MovementTrack{
		ActorID: "actor",
		Samples: []MovementSample{
			{Offset: 100},
			{Offset: 150},
			{Offset: 200},
			{Offset: 250},
			{Offset: 300},
			{Offset: 350},
			{Offset: 400},
		},
	}
	timeline, ok := movementDirectionProjectCandidateTimeline(track, evals, candidate)
	if !ok {
		t.Fatal("expected timeline projection")
	}
	if len(timeline) != len(track.Samples) {
		t.Fatalf("expected full projected timeline, got %d samples", len(timeline))
	}
	if got := timeline[0]; math.Abs(got-15) > 0.001 {
		t.Fatalf("unexpected first projected angle: %f", got)
	}
	if got := timeline[3]; math.Abs(got-30) > 0.001 {
		t.Fatalf("unexpected mid projected angle: %f", got)
	}
	if got := timeline[6]; math.Abs(got-45) > 0.001 {
		t.Fatalf("unexpected last projected angle: %f", got)
	}
}

func TestMovementDirectionAnchorMinPredictionsAllowsShortDenseFragments(t *testing.T) {
	if got := movementDirectionAnchorMinPredictions(32, 24); got != 24 {
		t.Fatalf("expected sparse anchor threshold to stay at 24, got %d", got)
	}
	if got := movementDirectionAnchorMinPredictions(120, 24); got != 90 {
		t.Fatalf("expected larger anchors to scale up threshold, got %d", got)
	}
}

func TestMovementBestMotionHeadingFallbackBuildsFullCoverageTrack(t *testing.T) {
	movementSamples := make([]movementDirectionAngleSample, 0, 64)
	for index := 0; index < 64; index++ {
		movementSamples = append(movementSamples, movementDirectionAngleSample{
			sampleIndex: index,
			offset:      index * 100,
			angle:       movementWrapDegrees(-90 + (float64(index) * 3)),
		})
	}
	anchorPredictions := make([]movementDirectionPrediction, 0, 8)
	for _, index := range []int{8, 12, 16, 20, 24, 28, 32, 36} {
		anchorPredictions = append(anchorPredictions, movementDirectionPrediction{
			sampleIndex:     index,
			offset:          movementSamples[index].offset,
			movementDegrees: movementSamples[index].angle,
			viewingDegrees:  movementSamples[index].angle + 4,
		})
	}
	anchor := MovementDirectionCandidate{
		Source:              "anchor",
		PropID:              "anchor-prop",
		ActorID:             "anchor-actor",
		VectorOffset:        30,
		Plane:               "anchor/",
		Alignment:           "offset",
		SampleCount:         len(anchorPredictions),
		MeanErrorDegrees:    4,
		MedianErrorDegrees:  4,
		MeanCosineAgreement: 0.99,
		MakesSense:          true,
	}
	evals := []movementDirectionCandidateEval{
		{
			candidate:  anchor,
			prediction: anchorPredictions,
			packetKeys: map[string]bool{"anchor": true},
		},
	}
	track, derived := movementBestMotionHeadingFallback(movementSamples, evals, &anchor)
	if track == nil {
		t.Fatal("expected motion fallback track")
	}
	if track.Source != "movement-smoothed-fallback" {
		t.Fatalf("unexpected fallback source: %+v", *track)
	}
	if track.SampleCount != len(movementSamples) || track.CoveragePercent != 100 {
		t.Fatalf("expected full coverage fallback, got %+v", *track)
	}
	if len(derived) != len(movementSamples) {
		t.Fatalf("expected derived headings for all movement samples, got %d", len(derived))
	}
}

func TestMovementDirectionFloat64PairStreamsFindSoloPairCandidates(t *testing.T) {
	actor := []byte{1, 0, 0, 0, 0, 0, 0, 0}
	prop := []byte{0x00, 0x00, 0x00, 0x00, 0x0C, 0x0C, 0x03, 0xF0, 0x00, 0x00, 0x00, 0x00}
	buf := make([]byte, 0)
	for i := 0; i < 8; i++ {
		rec := movementY11RecordBase(actor, prop)
		angle := (15 + float64(i)*10) * math.Pi / 180
		binary.LittleEndian.PutUint64(rec[30:38], math.Float64bits(math.Cos(angle)))
		binary.LittleEndian.PutUint64(rec[38:46], math.Float64bits(math.Sin(angle)))
		buf = append(buf, rec...)
	}

	streams := movementDirectionFloat64PairStreams(
		Header{CodeVersion: Y11S1Alpha3, Players: []Player{{Username: "dyn4mic"}}},
		buf,
		8,
		nil,
		map[string]bool{hex.EncodeToString(prop): true},
		nil,
	)

	found := false
	for _, stream := range streams {
		if stream.source == "f64-pair" && stream.propID == hex.EncodeToString(prop) && stream.actorID == hex.EncodeToString(actor) && stream.vectorOffset == 22 && len(stream.samples) == 8 {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected solo f64 pair stream at relative offset 22, got %+v", streams)
	}
}

func TestMovementDirectionDeltaIntegratedSamplesUnwrapsTurns(t *testing.T) {
	samples := []movementDirectionAngleSample{
		{offset: 10, angle: 170},
		{offset: 20, angle: 175},
		{offset: 30, angle: -179},
		{offset: 40, angle: -170},
	}

	got := movementDirectionDeltaIntegratedSamples(samples)
	if len(got) != 4 {
		t.Fatalf("expected 4 samples, got %d", len(got))
	}
	want := []float64{0, 5, 11, 20}
	for i := range want {
		if math.Abs(got[i].angle-want[i]) > 0.0001 {
			t.Fatalf("unexpected cumulative angle at %d: got=%f want=%f", i, got[i].angle, want[i])
		}
	}
}

func TestMovementDirectionAnchorStreamShortlistKeepsDensePropAndActor(t *testing.T) {
	streams := []movementDirectionStream{
		{source: "s16-neg-integrated", propID: "dense", actorID: "other", vectorOffset: 26, axisA: "x", samples: make([]movementDirectionAngleSample, 120)},
		{source: "s16-neg-delta-integrated", propID: "dense", actorID: "other", vectorOffset: 27, axisA: "x", samples: make([]movementDirectionAngleSample, 90)},
		{source: "f64-pair-double", propID: "anchor", actorID: "target", vectorOffset: 31, axisA: "x", samples: make([]movementDirectionAngleSample, 35)},
		{source: "s16", propID: "skip", actorID: "skip", vectorOffset: 10, axisA: "x", samples: make([]movementDirectionAngleSample, 200)},
	}

	got := movementDirectionAnchorStreamShortlist(streams, "target", "dense", 8)
	if len(got) != 3 {
		t.Fatalf("expected 3 shortlisted streams, got %d", len(got))
	}
	if got[0].actorID != "target" {
		t.Fatalf("expected same-actor stream to be prioritized first, got %+v", got[0])
	}
	foundDeltaDense := false
	for _, stream := range got {
		if stream.propID == "dense" && stream.source == "s16-neg-delta-integrated" {
			foundDeltaDense = true
			break
		}
	}
	if !foundDeltaDense {
		t.Fatalf("expected dense delta-integrated stream in shortlist, got %+v", got)
	}
}

func TestMovementFuseActorSplitDirectionStreamsStitchesIntegratedFragments(t *testing.T) {
	streams := []movementDirectionStream{
		{
			source:       "s16-neg-integrated",
			propID:       "dense",
			actorID:      "a",
			vectorOffset: 30,
			axisA:        "x",
			samples: []movementDirectionAngleSample{
				{offset: 10, angle: 0},
				{offset: 20, angle: 10},
				{offset: 30, angle: 20},
			},
		},
		{
			source:       "s16-neg-integrated",
			propID:       "dense",
			actorID:      "b",
			vectorOffset: 30,
			axisA:        "x",
			samples: []movementDirectionAngleSample{
				{offset: 40, angle: 5},
				{offset: 50, angle: 15},
				{offset: 60, angle: 25},
			},
		},
	}

	got := movementFuseActorSplitDirectionStreams(streams, 2)
	if len(got) != 3 {
		t.Fatalf("expected original 2 streams plus 1 fused stream, got %d", len(got))
	}
	fused := got[2]
	if fused.source != "s16-neg-integrated-actor-fused" || fused.actorID != "fused" {
		t.Fatalf("unexpected fused stream metadata: %+v", fused)
	}
	if len(fused.samples) != 6 {
		t.Fatalf("expected 6 fused samples, got %d", len(fused.samples))
	}
	if math.Abs(fused.samples[3].angle-20) > 0.0001 || math.Abs(fused.samples[5].angle-40) > 0.0001 {
		t.Fatalf("expected shifted continuation angles, got %+v", fused.samples)
	}
}

func TestMovementRecoveredClockEntriesTrimTrailingNonPositive(t *testing.T) {
	seconds5 := uint32(5)
	seconds4 := uint32(4)
	seconds3 := uint32(3)
	seconds2 := uint32(2)
	seconds1 := uint32(1)
	millis5 := uint32(5000)
	millis4 := uint32(4000)
	millis3 := uint32(3000)
	millis2 := uint32(2000)
	millis1 := uint32(1000)
	buf := make([]byte, 0)
	buf = append(buf, movementClockEntryPacket(nil, nil)...)
	buf = append(buf, movementClockEntryPacket(&seconds5, &millis5)...)
	buf = append(buf, movementClockEntryPacket(&seconds4, &millis4)...)
	buf = append(buf, movementClockEntryPacket(&seconds3, &millis3)...)
	buf = append(buf, movementClockEntryPacket(&seconds2, &millis2)...)
	buf = append(buf, movementClockEntryPacket(&seconds1, &millis1)...)
	buf = append(buf, movementClockEntryPacket(nil, nil)...)

	entries, anchors, trimmedLeading, trimmedTrailing, ok := movementRecoveredClockEntries(buf)
	if !ok {
		t.Fatal("expected recovered tail clock entries")
	}
	if len(anchors) != 5 {
		t.Fatalf("expected 5 anchors, got %d", len(anchors))
	}
	if trimmedLeading != 0 || trimmedTrailing != 1 {
		t.Fatalf("expected one trailing trim, got leading=%d trailing=%d", trimmedLeading, trimmedTrailing)
	}
	if len(entries) != 6 {
		t.Fatalf("expected 6 trimmed entries, got %d", len(entries))
	}
	if entries[0].milliseconds != 6000 || entries[len(entries)-1].milliseconds != 1000 {
		t.Fatalf("unexpected trimmed clock range: first=%d last=%d", entries[0].milliseconds, entries[len(entries)-1].milliseconds)
	}
}

func TestMovementDataY11AttachesRecoveredTailClock(t *testing.T) {
	actor := []byte{1, 0, 0, 0, 0, 0, 0, 0}
	prop := []byte{0x00, 0x00, 0x00, 0x00, 0x57, 0xE4, 0x05, 0xF0, 0x00, 0x00, 0x00, 0x00}
	buf := make([]byte, 0)
	buf = append(buf, movementY11PositionRecord(actor, prop, 45.3, -23.8058, 0.0135)...)
	buf = append(buf, movementY11QuaternionRecord(actor, prop, 45)...)
	buf = append(buf, movementY11QuaternionRecord(actor, prop, 90)...)
	buf = append(buf, movementClockEntryPacket(nil, nil)...)
	for second := 18; second >= 1; second-- {
		seconds := uint32(second)
		milliseconds := uint32(second * 1000)
		buf = append(buf, movementClockEntryPacket(&seconds, &milliseconds)...)
	}
	buf = append(buf, movementClockEntryPacket(nil, nil)...)

	r := Reader{
		b: buf,
		Header: Header{
			CodeVersion: Y11S1Alpha3,
			Players:     []Player{{Username: "dyn4mic"}},
		},
	}

	out := r.MovementData()
	if out.Clock == nil {
		t.Fatal("expected recovered clock block on Y11 export")
	}
	if out.Clock.Source != "tail-index-ms" || out.Clock.TrimmedTrailing != 1 || !out.Clock.MonotonicSamples {
		t.Fatalf("unexpected recovered clock metadata: %+v", out.Clock)
	}
	if len(out.PrimaryTracks) != 1 || len(out.PrimaryTracks[0].Samples) != 3 {
		t.Fatalf("expected one timed primary track, got %+v", out.PrimaryTracks)
	}
	first := out.PrimaryTracks[0].Samples[0]
	last := out.PrimaryTracks[0].Samples[len(out.PrimaryTracks[0].Samples)-1]
	if first.TimeInSeconds == nil || last.TimeInSeconds == nil {
		t.Fatalf("expected sample-level recovered times, got first=%+v last=%+v", first, last)
	}
	if *first.TimeInSeconds <= *last.TimeInSeconds {
		t.Fatalf("expected descending remaining-time samples, got first=%f last=%f", *first.TimeInSeconds, *last.TimeInSeconds)
	}
}

func TestMovementRotationHeadingHybridModelHandlesMixedSoloPackets(t *testing.T) {
	var hybrid movementRotationHeadingModel
	found := false
	for _, model := range movementRotationHeadingModels() {
		if model.name == "hybrid-z-or-implicit-default" {
			hybrid = model
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected hybrid rotation heading model")
	}
	direct, ok := hybrid.decode(Vector3{X: 0, Y: 0, Z: float32(120 * math.Pi / 180)})
	if !ok || math.Abs(direct-120) > 0.5 {
		t.Fatalf("expected direct-radian decode near 120 degrees, got %f ok=%v", direct, ok)
	}
	quatLike, ok := hybrid.decode(Vector3{X: 0.29548135, Y: 0.18343286, Z: 0.75073767})
	if !ok || math.Abs(quatLike-101.55) > 1.0 {
		t.Fatalf("expected quaternion-like decode near 101.55 degrees, got %f ok=%v", quatLike, ok)
	}
}

func TestMovementApplyDerivedViewingDirectionsKeepsGoodDerivedTimeline(t *testing.T) {
	actorID := "solo"
	track := MovementTrack{
		ActorID: actorID,
		Samples: []MovementSample{
			{RotationDegrees: &Vector3{Z: -120}},
			{RotationDegrees: &Vector3{Z: -40}},
			{RotationDegrees: &Vector3{Z: 30}},
			{RotationDegrees: &Vector3{Z: 100}},
		},
	}
	derived := map[int]float64{0: -10, 1: 15, 2: 45, 3: 90}
	ordered, _ := movementApplyDerivedViewingDirections([]MovementTrack{track}, []MovementTrack{{ActorID: actorID}}, movementDirectionSearchResult{derivedByActor: map[string]map[int]float64{actorID: derived}}, nil)
	for index, want := range []float64{-10, 15, 45, 90} {
		got := ordered[0].Samples[index].ViewingDirectionDegrees
		if got == nil || math.Abs(*got-want) > 0.001 {
			t.Fatalf("expected derived heading at index %d, got %+v want %f", index, got, want)
		}
	}
}

func movementY11PositionRecord(actor []byte, prop []byte, x, y, z float32) []byte {
	rec := movementY11RecordBase(actor, prop)
	binary.LittleEndian.PutUint32(rec[30:34], math.Float32bits(x))
	binary.LittleEndian.PutUint32(rec[34:38], math.Float32bits(y))
	binary.LittleEndian.PutUint32(rec[38:42], math.Float32bits(z))
	return rec
}

func movementY11QuaternionRecord(actor []byte, prop []byte, yawDegrees float64) []byte {
	rec := movementY11RecordBase(actor, prop)
	halfYaw := float32((yawDegrees * math.Pi / 180) / 2)
	binary.LittleEndian.PutUint32(rec[30:34], math.Float32bits(0))
	binary.LittleEndian.PutUint32(rec[34:38], math.Float32bits(0))
	binary.LittleEndian.PutUint32(rec[38:42], math.Float32bits(float32(math.Sin(float64(halfYaw)))))
	binary.LittleEndian.PutUint32(rec[42:46], math.Float32bits(float32(math.Cos(float64(halfYaw)))))
	return rec
}

func movementY11RecordBase(actor []byte, prop []byte) []byte {
	rec := make([]byte, 51)
	copy(rec[0:8], actor)
	copy(rec[8:20], prop)
	copy(rec[20:24], []byte{0x8E, 0x00, 0x00, 0x00})
	copy(rec[24:28], movementRecordMarker)
	copy(rec[28:30], []byte{0xC0, 0x01})
	return rec
}

func movementTimePacket(seconds uint32) []byte {
	pkt := append([]byte{}, movementTimePattern...)
	pkt = append(pkt, 0x04)
	payload := make([]byte, 4)
	binary.LittleEndian.PutUint32(payload, seconds)
	return append(pkt, payload...)
}

func movementClockEntryPacket(seconds *uint32, milliseconds *uint32) []byte {
	pkt := append([]byte{}, movementClockEntryPattern...)
	pkt = append(pkt, 0xAA, 0xBB, 0xCC)
	if seconds != nil {
		pkt = append(pkt, movementClockSecondsPattern...)
		pkt = appendUint32(pkt, *seconds)
	}
	if milliseconds != nil {
		pkt = append(pkt, movementClockMillisPattern...)
		pkt = appendUint32(pkt, *milliseconds)
	}
	pkt = append(pkt, 0x00, 0x01, 0x02)
	return pkt
}

func movementRecord(actor []byte, prop []byte, x, y, z float32) []byte {
	rec := append([]byte{}, actor...)
	rec = append(rec, prop...)
	rec = append(rec, 0x8E, 0x00, 0x00, 0x00)
	rec = append(rec, movementRecordMarker...)
	rec = append(rec, 0xC0, 0x01)
	rec = appendFloat32(rec, x)
	rec = appendFloat32(rec, y)
	rec = appendFloat32(rec, z)
	rec = append(rec, movementRecordTail...)
	return rec
}

func appendFloat32(dst []byte, value float32) []byte {
	buf := make([]byte, 4)
	binary.LittleEndian.PutUint32(buf, math.Float32bits(value))
	return append(dst, buf...)
}

func appendUint32(dst []byte, value uint32) []byte {
	buf := make([]byte, 4)
	binary.LittleEndian.PutUint32(buf, value)
	return append(dst, buf...)
}

func floatPtr(value float64) *float64 {
	return &value
}

func float32Equal(a, b float32) bool {
	const epsilon = 0.0001
	return math.Abs(float64(a-b)) < epsilon
}

func TestMovementFoldPitchDegrees(t *testing.T) {
	cases := []struct {
		input float64
		want  float64
	}{
		{0, 0},
		{45, 45},
		{120, 60},
		{170, 10},
		{-120, -60},
		{-170, -10},
	}
	for _, tc := range cases {
		got := movementFoldPitchDegrees(tc.input)
		if math.Abs(got-tc.want) > 0.0001 {
			t.Fatalf("fold(%f)=%f want %f", tc.input, got, tc.want)
		}
	}
}

func TestMovementGuessViewPitchAxisPrefersResponsiveAxis(t *testing.T) {
	track := MovementTrack{
		Samples: []MovementSample{
			{RotationDegrees: &Vector3{X: 10, Y: 10, Z: 42}, ViewingDirectionDegrees: floatPtr(0)},
			{RotationDegrees: &Vector3{X: 10, Y: 10, Z: 20}, ViewingDirectionDegrees: floatPtr(0)},
			{RotationDegrees: &Vector3{X: 10, Y: 10, Z: -10}, ViewingDirectionDegrees: floatPtr(0), Position: &Vector3{X: 0}},
			{RotationDegrees: &Vector3{X: 10, Y: 10, Z: -35}, ViewingDirectionDegrees: floatPtr(0), Position: &Vector3{X: 0}},
			{RotationDegrees: &Vector3{X: 10, Y: 10, Z: 50}, ViewingDirectionDegrees: floatPtr(0), Position: &Vector3{X: 0}},
			{RotationDegrees: &Vector3{X: 10, Y: 10, Z: 85}, ViewingDirectionDegrees: floatPtr(0), Position: &Vector3{X: 0}},
			{RotationDegrees: &Vector3{X: 10, Y: 10, Z: 42}, ViewingDirectionDegrees: floatPtr(0)},
			{RotationDegrees: &Vector3{X: 10, Y: 10, Z: 42}, ViewingDirectionDegrees: floatPtr(0)},
		},
	}

	got := movementGuessViewPitchAxis(track)
	if got != "z" {
		t.Fatalf("expected z to win as pitch axis, got %q", got)
	}
}

func TestMovementDirectionProjectedPitchTimelineScorePrefersPitchLikeSignal(t *testing.T) {
	track := MovementTrack{Samples: make([]MovementSample, 10)}
	yawTimeline := map[int]float64{
		0: 0,
		1: 12,
		2: 24,
		3: 24,
		4: 24,
		5: 24,
		6: 24,
		7: 24,
		8: 24,
		9: 24,
	}
	stationary := map[int]bool{
		0: true,
		1: true,
		2: true,
		3: true,
		4: true,
		5: true,
		6: true,
		7: true,
		8: true,
		9: true,
	}
	pitchLike := map[int]float64{
		0: 0,
		1: 0.4,
		2: 0.8,
		3: 8,
		4: 16,
		5: 24,
		6: 32,
		7: 40,
		8: 48,
		9: 56,
	}
	yawLeaky := map[int]float64{
		0: 0,
		1: 12,
		2: 24,
		3: 24.5,
		4: 25,
		5: 25.5,
		6: 26,
		7: 26.5,
		8: 27,
		9: 27.5,
	}

	pitchScore := movementDirectionProjectedPitchTimelineScore(track, yawTimeline, pitchLike, stationary)
	leakyScore := movementDirectionProjectedPitchTimelineScore(track, yawTimeline, yawLeaky, stationary)
	if pitchScore <= leakyScore {
		t.Fatalf("expected pitch-like score %f to beat leaky score %f", pitchScore, leakyScore)
	}
}

func TestMovementDirectionPitchAlignmentsForStreamIncludesTimedFallbacks(t *testing.T) {
	stream := movementDirectionStream{
		propID:  "prop",
		actorID: "actor",
		samples: []movementDirectionAngleSample{
			{hasTime: true},
			{hasTime: true},
			{hasTime: true},
			{hasTime: true},
			{hasTime: true},
			{hasTime: true},
			{hasTime: true},
			{hasTime: true},
		},
	}
	bestDense := &MovementDirectionCandidate{
		PropID:    "prop",
		ActorID:   "actor",
		Alignment: "progress",
		ByteShift: 17,
	}

	alignments := movementDirectionPitchAlignmentsForStream(stream, bestDense, nil)
	hasDense := false
	hasTimed := false
	for _, alignment := range alignments {
		if alignment.Alignment == "progress" && alignment.ByteShift == 17 {
			hasDense = true
		}
		if alignment.Alignment == "time" && alignment.TimeShiftMilliseconds == 0 {
			hasTimed = true
		}
	}
	if !hasDense {
		t.Fatal("expected pitch search to keep best-dense alignment")
	}
	if !hasTimed {
		t.Fatal("expected pitch search to include timed fallback alignment")
	}
}
