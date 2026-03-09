package dissect

import (
	"math"
	"strings"
	"testing"
)

func TestSelectBlenderCameraTrackDefaultsToRecordingPlayer(t *testing.T) {
	data := MovementOutput{
		Header: Header{
			RecordingPlayerID: 42,
			Players: []Player{
				{ID: 7, Username: "handyy"},
				{ID: 42, Username: "kds"},
			},
		},
		PrimaryTracks: []MovementTrack{
			{ActorID: "actor-a", Label: "P01", PlayerNameGuess: "handyy"},
			{ActorID: "actor-b", Label: "P02", PlayerNameGuess: "kds"},
		},
	}

	track, err := selectBlenderCameraTrack(data, "")
	if err != nil {
		t.Fatalf("expected default track selection to succeed: %v", err)
	}
	if track.ActorID != "actor-b" {
		t.Fatalf("expected recording player track, got %+v", track)
	}
}

func TestSelectBlenderCameraTrackMatchesSelectors(t *testing.T) {
	data := MovementOutput{
		PrimaryTracks: []MovementTrack{{
			ActorID:         "00112233aabbccdd",
			Label:           "P01",
			PlayerNameGuess: "kds",
		}},
	}

	selectors := []string{"kds", "P01", "00112233aabbccdd", "kds_P01"}
	for _, selector := range selectors {
		track, err := selectBlenderCameraTrack(data, selector)
		if err != nil {
			t.Fatalf("selector %q failed: %v", selector, err)
		}
		if track.ActorID != "00112233aabbccdd" {
			t.Fatalf("selector %q returned wrong track: %+v", selector, track)
		}
	}
}

func TestBlenderCameraEulerUsesWrappedDegrees(t *testing.T) {
	sample := MovementSample{
		Rotation:        &Vector3{X: 10, Y: 20, Z: 30},
		RotationDegrees: &Vector3{X: 15, Y: 25, Z: 35},
	}

	got := blenderCameraEuler(sample, normalizeBlenderCameraOptions(BlenderCameraOptions{}))
	if math.Abs(got.X-degreesToRadians(15)) > 0.0001 {
		t.Fatalf("expected wrapped pitch from rotationDegrees, got %f", got.X)
	}
	if math.Abs(got.Y-degreesToRadians(25)) > 0.0001 {
		t.Fatalf("expected wrapped roll from rotationDegrees, got %f", got.Y)
	}
	if math.Abs(got.Z-degreesToRadians(35)) > 0.0001 {
		t.Fatalf("expected wrapped yaw from rotationDegrees, got %f", got.Z)
	}
}

func TestChooseBlenderCameraOutputPrefersDenserUsernameTrack(t *testing.T) {
	header := Header{Players: []Player{{Username: "noa"}, {Username: "kds"}}}
	baseline := MovementOutput{
		Header:         header,
		PositionPropID: "baseline-pos",
		PrimaryTracks: []MovementTrack{
			{ActorID: "a", PlayerNameGuess: "noa", PositionSamples: 80, RotationSamples: 40, Samples: make([]MovementSample, 120)},
		},
	}
	denser := MovementOutput{
		Header:         header,
		PositionPropID: "denser-pos",
		PrimaryTracks: []MovementTrack{
			{ActorID: "b", PlayerNameGuess: "noa", PositionSamples: 500, RotationSamples: 250, Samples: make([]MovementSample, 750)},
		},
	}

	data, track, err := chooseBlenderCameraOutput([]MovementOutput{baseline, denser}, "noa")
	if err != nil {
		t.Fatalf("expected output selection to succeed: %v", err)
	}
	if data.PositionPropID != "denser-pos" || track.ActorID != "b" {
		t.Fatalf("expected denser username track to win, got data=%+v track=%+v", data, track)
	}
}

func TestChooseBlenderCameraOutputPrefersResolvedRecordingPlayerTrack(t *testing.T) {
	header := Header{
		RecordingPlayerID: 42,
		Players: []Player{
			{ID: 42, Username: "dyn4mic"},
			{ID: 7, Username: "other"},
		},
	}
	unresolved := MovementOutput{
		Header:         header,
		PositionPropID: "unresolved-pos",
		RotationPropID: "unresolved-rot",
		PrimaryTracks: []MovementTrack{
			{ActorID: "a", PlayerNameGuess: "other", PositionSamples: 400, RotationSamples: 200, Samples: make([]MovementSample, 600)},
		},
	}
	resolved := MovementOutput{
		Header:         header,
		PositionPropID: "resolved-pos",
		RotationPropID: "resolved-rot",
		PrimaryTracks: []MovementTrack{
			{ActorID: "b", PlayerNameGuess: "dyn4mic", PositionSamples: 80, RotationSamples: 60, Samples: make([]MovementSample, 140)},
		},
	}

	data, track, err := chooseBlenderCameraOutput([]MovementOutput{unresolved, resolved}, "")
	if err != nil {
		t.Fatalf("expected recording player output selection to succeed: %v", err)
	}
	if data.PositionPropID != "resolved-pos" || track.ActorID != "b" {
		t.Fatalf("expected resolved recording player track to win, got data=%+v track=%+v", data, track)
	}
}

func TestBlenderShouldScanCandidatePropsOnlyForRecordingPlayerOrExplicitUsername(t *testing.T) {
	header := Header{Players: []Player{{Username: "noa"}, {Username: "kds"}}}
	if !blenderShouldScanCandidateProps(header, "") {
		t.Fatal("expected empty selector to allow candidate scan")
	}
	if !blenderShouldScanCandidateProps(header, "kds") {
		t.Fatal("expected explicit username selector to allow candidate scan")
	}
	if blenderShouldScanCandidateProps(header, "P01") {
		t.Fatal("did not expect label selector to trigger candidate scan")
	}
}

func TestBlenderCameraTrackScorePenalizesOriginLikeTrack(t *testing.T) {
	originLike := MovementTrack{
		PositionSamples: 120,
		RotationSamples: 30,
		Samples:         make([]MovementSample, 150),
		FirstPosition:   &Vector3{X: 0.2, Y: -0.3, Z: 0.8},
	}
	worldSpace := MovementTrack{
		PositionSamples: 120,
		RotationSamples: 30,
		Samples:         make([]MovementSample, 150),
		FirstPosition:   &Vector3{X: -35, Y: -110, Z: 4},
	}

	if blenderCameraTrackScore(worldSpace) <= blenderCameraTrackScore(originLike) {
		t.Fatalf("expected world-space track to beat origin-like track")
	}
}

func TestRenderBlenderCameraScriptIncludesMetadataAndKeyframes(t *testing.T) {
	timeValue := 12.5
	track := MovementTrack{
		ActorID:         "actor-b",
		Label:           "P02",
		PlayerNameGuess: "kds",
		Samples: []MovementSample{
			{
				Offset:          10,
				TimeInSeconds:   &timeValue,
				Position:        &Vector3{X: 1, Y: 2, Z: 3},
				Rotation:        &Vector3{X: 0.1, Y: 0.2, Z: 0.3},
				RotationDegrees: &Vector3{X: 10, Y: 20, Z: 30},
			},
			{
				Offset:          20,
				Position:        &Vector3{X: 4, Y: 5, Z: 6},
				Rotation:        &Vector3{X: 0.4, Y: 0.5, Z: 0.6},
				RotationDegrees: &Vector3{X: 40, Y: 50, Z: 60},
			},
		},
	}
	options := normalizeBlenderCameraOptions(BlenderCameraOptions{FPS: 30, FrameStep: 2, Scale: 0.5})
	data := MovementOutput{PositionPropID: "pos-prop", RotationPropID: "rot-prop"}

	script := renderBlenderCameraScript(data, track, options)

	expectedSnippets := []string{
		"import bpy",
		"CAMERA_NAME = \"R6Cam_kds_P02\"",
		"scene.render.fps = FPS",
		"camera.rotation_mode = 'XYZ'",
		"camera.location = (0.5, 1, 1.5)",
		"camera.keyframe_insert(data_path='location', frame=3)",
		"keyframe_prop(camera, 'r6_sample_offset', 10, 1)",
		"keyframe_prop(camera, 'r6_time_seconds', 12.5, 1)",
		"camera['r6_position_prop'] = POSITION_PROP_ID",
		"scene.frame_end = 3",
	}
	for _, snippet := range expectedSnippets {
		if !strings.Contains(script, snippet) {
			t.Fatalf("expected script to contain %q\n%s", snippet, script)
		}
	}
}
