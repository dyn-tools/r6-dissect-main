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

	got := blenderCameraEuler(sample, normalizeBlenderCameraOptions(BlenderCameraOptions{}), nil)
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

func TestBlenderCameraEulerPrefersViewingHeadingForYaw(t *testing.T) {
	viewHeading := 135.0
	sample := MovementSample{
		RotationDegrees:         &Vector3{X: 15, Y: 25, Z: 35},
		ViewingDirectionDegrees: &viewHeading,
	}
	headingRadians := degreesToRadians(viewHeading)

	got := blenderCameraEuler(sample, normalizeBlenderCameraOptions(BlenderCameraOptions{}), &headingRadians)
	if math.Abs(got.X-degreesToRadians(15)) > 0.0001 {
		t.Fatalf("expected wrapped pitch from rotationDegrees, got %f", got.X)
	}
	if math.Abs(got.Y-degreesToRadians(25)) > 0.0001 {
		t.Fatalf("expected wrapped roll from rotationDegrees, got %f", got.Y)
	}
	if math.Abs(got.Z-degreesToRadians(135)) > 0.0001 {
		t.Fatalf("expected yaw from recovered heading, got %f", got.Z)
	}
}

func TestBlenderCameraEulerSupportsLevelCameraOffsets(t *testing.T) {
	viewHeading := 135.0
	sample := MovementSample{
		RotationDegrees:         &Vector3{X: 15, Y: 25, Z: 35},
		ViewingDirectionDegrees: &viewHeading,
	}
	headingRadians := degreesToRadians(viewHeading)
	options := normalizeBlenderCameraOptions(BlenderCameraOptions{
		LevelCamera:    true,
		PitchOffsetDeg: 90,
		YawOffsetDeg:   -90,
	})

	got := blenderCameraEuler(sample, options, &headingRadians)
	if math.Abs(got.X-degreesToRadians(90)) > 0.0001 {
		t.Fatalf("expected leveled pitch with base offset, got %f", got.X)
	}
	if math.Abs(got.Y) > 0.0001 {
		t.Fatalf("expected roll to be zeroed in level mode, got %f", got.Y)
	}
	if math.Abs(got.Z-degreesToRadians(45)) > 0.0001 {
		t.Fatalf("expected yaw offset to apply to recovered heading, got %f", got.Z)
	}
}

func TestBlenderCameraEulerPrefersRecoveredPitchWhenAvailable(t *testing.T) {
	viewHeading := 90.0
	viewPitch := -35.0
	sample := MovementSample{
		RotationDegrees:         &Vector3{X: 15, Y: 25, Z: 35},
		ViewingDirectionDegrees: &viewHeading,
		ViewPitchDegrees:        &viewPitch,
	}
	headingRadians := degreesToRadians(viewHeading)

	got := blenderCameraEuler(sample, normalizeBlenderCameraOptions(BlenderCameraOptions{}), &headingRadians)
	if math.Abs(got.X-degreesToRadians(viewPitch)) > 0.0001 {
		t.Fatalf("expected pitch from recovered view pitch, got %f", got.X)
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
	viewHeadingA := 170.0
	viewHeadingB := -170.0
	track := MovementTrack{
		ActorID:         "actor-b",
		Label:           "P02",
		PlayerNameGuess: "kds",
		Samples: []MovementSample{
			{
				Offset:                  10,
				TimeInSeconds:           &timeValue,
				Position:                &Vector3{X: 1, Y: 2, Z: 3},
				Rotation:                &Vector3{X: 0.1, Y: 0.2, Z: 0.3},
				RotationDegrees:         &Vector3{X: 10, Y: 20, Z: 30},
				ViewingDirectionDegrees: &viewHeadingA,
			},
			{
				Offset:                  20,
				Position:                &Vector3{X: 4, Y: 5, Z: 6},
				Rotation:                &Vector3{X: 0.4, Y: 0.5, Z: 0.6},
				RotationDegrees:         &Vector3{X: 40, Y: 50, Z: 60},
				ViewingDirectionDegrees: &viewHeadingB,
			},
		},
	}
	options := normalizeBlenderCameraOptions(BlenderCameraOptions{FPS: 30, FrameStep: 2, Scale: 0.5})
	data := MovementOutput{PositionPropID: "pos-prop", RotationPropID: "rot-prop"}

	script := renderBlenderCameraScript(data, track, options)

	expectedSnippets := []string{
		"import bpy",
		"CAMERA_NAME = \"R6Cam_kds_P02\"",
		"RIG_NAME = \"R6Rig_kds_P02\"",
		"BOUNDS_NAME = \"R6Bounds_kds_P02\"",
		"scene.render.fps = FPS",
		"camera.rotation_mode = 'XYZ'",
		"def create_bounds_object(name, bounds_min, bounds_max, collection):",
		"bounds = create_bounds_object(BOUNDS_NAME, BOUNDS_MIN, BOUNDS_MAX, collection)",
		"rig = bpy.data.objects.new(RIG_NAME, None)",
		"camera.parent = rig",
		"camera.rotation_euler = (math.radians(90), 0, 0)",
		"rig.location = (0.5, 1, 1.5)",
		"rig.rotation_euler = (0, 0, -2.96705972839036)",
		"rig.keyframe_insert(data_path='location', frame=3)",
		"keyframe_prop(rig, 'r6_sample_offset', 10, 1)",
		"keyframe_prop(rig, 'r6_view_heading_deg', 170, 1)",
		"keyframe_prop(rig, 'r6_view_heading_continuous_deg', 190, 3)",
		"keyframe_prop(rig, 'r6_view_heading_smoothed_deg', -170, 1)",
		"keyframe_prop(rig, 'r6_time_seconds', 12.5, 1)",
		"rig['r6_position_prop'] = POSITION_PROP_ID",
		"scene.frame_end = 3",
	}
	for _, snippet := range expectedSnippets {
		if !strings.Contains(script, snippet) {
			t.Fatalf("expected script to contain %q\n%s", snippet, script)
		}
	}
}

func TestBlenderSampleFramesUsesRecoveredTime(t *testing.T) {
	t0 := 10.0
	t1 := 9.5
	t2 := 9.0
	samples := []MovementSample{
		{TimeInSeconds: &t0},
		{TimeInSeconds: &t1},
		{TimeInSeconds: &t2},
	}
	frames := blenderSampleFrames(samples, 30, 1)
	expected := []int{1, 16, 31}
	for i, want := range expected {
		if frames[i] != want {
			t.Fatalf("frame %d: expected %d, got %d", i, want, frames[i])
		}
	}
}

func TestBlenderSmoothScalarTimelineSuppressesSingleSampleSpike(t *testing.T) {
	makePtr := func(v float64) *float64 { return &v }
	values := []*float64{
		makePtr(0),
		makePtr(0),
		makePtr(60),
		makePtr(0),
		makePtr(0),
	}
	smoothed := blenderSmoothScalarTimeline(values, 1, 0.5, false)
	if smoothed[2] == nil {
		t.Fatal("expected smoothed center value")
	}
	if math.Abs(*smoothed[2]) > 20 {
		t.Fatalf("expected spike to be reduced substantially, got %f", *smoothed[2])
	}
}

func TestBlenderStableViewingHeadingContinuousClampsWildPrefix(t *testing.T) {
	views := []float64{96, 36, -24, -84, -144, -12, -72}
	samples := make([]MovementSample, len(views))
	for index, view := range views {
		viewCopy := view
		samples[index].ViewingDirectionDegrees = &viewCopy
		samples[index].RotationDegrees = &Vector3{Z: 42.5}
	}
	samples[5].Position = &Vector3{X: 1}
	samples[6].Position = &Vector3{X: 2}

	values := blenderStableViewingHeadingContinuous(samples, blenderStableStartIndex(samples))
	for index := 0; index < 5; index++ {
		if values[index] == nil {
			t.Fatalf("expected clamped prefix value at %d", index)
		}
		if math.Abs(*values[index]-(-12)) > 0.001 {
			t.Fatalf("expected prefix to clamp to first stable heading, got %f at %d", *values[index], index)
		}
	}
}

func TestBlenderBaselinePitchRollUsesStableSample(t *testing.T) {
	samples := []MovementSample{
		{
			RotationDegrees:         &Vector3{X: 10, Y: 20, Z: 30},
			ViewingDirectionDegrees: float64Ptr(120),
		},
		{
			Position:                &Vector3{X: 1},
			RotationDegrees:         &Vector3{X: 15, Y: 25, Z: 35},
			ViewingDirectionDegrees: float64Ptr(135),
		},
	}

	headings := blenderStableViewingHeadingContinuous(samples, 1)
	pitch, roll := blenderBaselinePitchRoll(samples, normalizeBlenderCameraOptions(BlenderCameraOptions{}), headings, nil, 1)
	if math.Abs(pitch-degreesToRadians(15)) > 0.0001 {
		t.Fatalf("expected baseline pitch from stable sample, got %f", pitch)
	}
	if math.Abs(roll-degreesToRadians(25)) > 0.0001 {
		t.Fatalf("expected baseline roll from stable sample, got %f", roll)
	}
}

func TestBlenderTrackWithDerivedTimelinesClearsRequestedAxes(t *testing.T) {
	heading := 45.0
	pitch := -20.0
	track := MovementTrack{
		Samples: []MovementSample{
			{ViewingDirectionDegrees: &heading, ViewPitchDegrees: &pitch},
			{ViewingDirectionDegrees: &heading, ViewPitchDegrees: &pitch},
		},
	}
	derived := blenderTrackWithDerivedTimelines(track, map[int]float64{1: 90}, nil, true, false)
	if derived.Samples[0].ViewingDirectionDegrees != nil {
		t.Fatal("expected heading on untouched sample to be cleared")
	}
	if derived.Samples[1].ViewingDirectionDegrees == nil || math.Abs(*derived.Samples[1].ViewingDirectionDegrees-90) > 0.001 {
		t.Fatalf("expected derived heading on sample 1, got %+v", derived.Samples[1].ViewingDirectionDegrees)
	}
	if derived.Samples[0].ViewPitchDegrees == nil || math.Abs(*derived.Samples[0].ViewPitchDegrees-pitch) > 0.001 {
		t.Fatalf("expected pitch to remain untouched, got %+v", derived.Samples[0].ViewPitchDegrees)
	}
}

func TestRenderBlenderCameraScriptVariantsIncludesCandidateMetadata(t *testing.T) {
	viewHeading := 30.0
	viewPitch := -10.0
	timeValue := 5.0
	track := MovementTrack{
		ActorID:         "actor-b",
		Label:           "P02",
		PlayerNameGuess: "kds",
		Samples: []MovementSample{
			{
				Offset:                  10,
				TimeInSeconds:           &timeValue,
				Position:                &Vector3{X: 1, Y: 2, Z: 3},
				RotationDegrees:         &Vector3{X: 10, Y: 20, Z: 30},
				ViewingDirectionDegrees: &viewHeading,
				ViewPitchDegrees:        &viewPitch,
			},
		},
	}
	variants := []blenderCameraVariant{
		{
			Track:         track,
			VariantKind:   "selected",
			VariantLabel:  "Selected",
			PrimaryCamera: true,
		},
		{
			Track:        blenderTrackWithDerivedTimelines(track, map[int]float64{0: 90}, nil, true, false),
			NameSuffix:   "Yaw01_test",
			VariantKind:  "yaw-candidate",
			VariantLabel: "Yaw Candidate 1",
			Candidate: &MovementMacroTimelineCandidate{
				Source:       "late-s16-integrated",
				PropID:       "00000000479807f000000000",
				ActorID:      "actor-b",
				VectorOffset: 46,
				Alignment:    "offset",
				Transform:    "neg",
				Score:        12.5,
			},
		},
	}
	script := renderBlenderCameraScriptVariants(MovementOutput{
		PositionPropID: "pos-prop",
		RotationPropID: "rot-prop",
	}, variants, normalizeBlenderCameraOptions(BlenderCameraOptions{FPS: 60}))

	expectedSnippets := []string{
		"Imported R6 compare camera variants into Blender",
		"rig['r6_variant_kind'] = \"selected\"",
		"rig['r6_variant_kind'] = \"yaw-candidate\"",
		"rig['r6_variant_label'] = \"Yaw Candidate 1\"",
		"rig['r6_candidate_source'] = \"late-s16-integrated\"",
		"rig['r6_candidate_prop'] = \"00000000479807f000000000\"",
		"rig['r6_candidate_vector_offset'] = 46",
		"scene.camera = camera",
	}
	for _, snippet := range expectedSnippets {
		if !strings.Contains(script, snippet) {
			t.Fatalf("expected compare script to contain %q\n%s", snippet, script)
		}
	}
}

func float64Ptr(value float64) *float64 {
	return &value
}
