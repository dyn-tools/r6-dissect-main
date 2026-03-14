package dissect

import (
	"math"
	"testing"
)

func TestMovementMacroSequenceScoreRewardsConsistentSteps(t *testing.T) {
	stats := []movementMacroWindowStat{
		{window: movementMacroWindow{startMs: 0, dx: 1084}, mean: 0, span: 1},
		{window: movementMacroWindow{startMs: 1000, dx: 1084}, mean: -45, span: 1},
		{window: movementMacroWindow{startMs: 2000, dx: 1084}, mean: -90, span: 1},
	}
	score := movementMacroSequenceScore(stats, func(window movementMacroWindow) float64 {
		return -signFloat64(window.dx)
	})
	if score <= 0 {
		t.Fatalf("expected positive score for consistent right-turn steps, got %f", score)
	}
}

func TestMovementMacroDistinctLevelScorePenalizesCollapsedLevels(t *testing.T) {
	collapsed := []movementMacroWindowStat{
		{window: movementMacroWindow{startMs: 0}, mean: 39, span: 0},
		{window: movementMacroWindow{startMs: 1000}, mean: 39.1, span: 0},
		{window: movementMacroWindow{startMs: 2000}, mean: 39.05, span: 0},
	}
	score := movementMacroDistinctLevelScore(collapsed, 3, 5)
	if score >= 0 {
		t.Fatalf("expected negative score for collapsed levels, got %f", score)
	}
}

func TestMovementMacroDistinctLevelScoreRewardsMultipleLevels(t *testing.T) {
	levels := []movementMacroWindowStat{
		{window: movementMacroWindow{startMs: 0}, mean: -80, span: 1},
		{window: movementMacroWindow{startMs: 1000}, mean: -40, span: 1},
		{window: movementMacroWindow{startMs: 2000}, mean: 0, span: 1},
		{window: movementMacroWindow{startMs: 3000}, mean: 40, span: 1},
		{window: movementMacroWindow{startMs: 4000}, mean: 80, span: 1},
	}
	score := movementMacroDistinctLevelScore(levels, 3, 5)
	if score < 0 {
		t.Fatalf("expected non-negative score for multiple distinct levels, got %f", score)
	}
}

func TestMovementMacroSpanConsistencyScoreRewardsStableSpans(t *testing.T) {
	stable := []movementMacroWindowStat{
		{span: 42},
		{span: 45},
		{span: 44},
		{span: 43},
	}
	noisy := []movementMacroWindowStat{
		{span: 0},
		{span: 180},
		{span: 38},
		{span: 120},
	}
	if movementMacroSpanConsistencyScore(stable, 160) <= movementMacroSpanConsistencyScore(noisy, 160) {
		t.Fatalf("expected stable spans to score better than noisy spans")
	}
}

func TestMovementMacroSuppressionPenaltyPunishesJitter(t *testing.T) {
	stable := []movementMacroWindowStat{
		{span: 0.5, slope: 0.2},
		{span: 1.0, slope: 0.1},
	}
	jittery := []movementMacroWindowStat{
		{span: 40, slope: 12},
		{span: 75, slope: -18},
	}
	if movementMacroSuppressionPenalty(jittery, 12) <= movementMacroSuppressionPenalty(stable, 12) {
		t.Fatalf("expected jittery windows to incur higher suppression penalty")
	}
}

func TestMovementMacroExpectedYawDegreesUsesCalibration(t *testing.T) {
	degrees := movementMacroExpectedYawDegrees("yaw", 1084, 8670)
	if degrees < 44 || degrees > 46 {
		t.Fatalf("expected 1084 dots at 8670/full turn to be about 45 degrees, got %f", degrees)
	}
}

func TestMovementMacroExpectedYawDeltaScoreRewardsCloseMatch(t *testing.T) {
	window := movementMacroWindow{kind: "yaw", expectedYawDegrees: 45}
	closeScore := movementMacroExpectedYawDeltaScore(window, -44)
	farScore := movementMacroExpectedYawDeltaScore(window, -140)
	if closeScore <= farScore {
		t.Fatalf("expected close yaw delta match to score better, close=%f far=%f", closeScore, farScore)
	}
}

func TestMovementMacroExpectedYawSpanScoreRewardsCloseSpan(t *testing.T) {
	window := movementMacroWindow{kind: "yaw", expectedYawDegrees: 45}
	closeScore := movementMacroExpectedYawSpanScore(window, 46)
	farScore := movementMacroExpectedYawSpanScore(window, 208)
	if closeScore <= farScore {
		t.Fatalf("expected close yaw span match to score better, close=%f far=%f", closeScore, farScore)
	}
}

func TestMovementMacroAffineNormalizeYawTimelineRescalesObservedLevels(t *testing.T) {
	times := []float64{10.0, 9.8, 9.6, 9.4, 9.2, 9.0, 8.8, 8.6}
	track := MovementTrack{
		Samples: make([]MovementSample, len(times)),
	}
	for i := range times {
		track.Samples[i].TimeInSeconds = &times[i]
	}
	windows := []movementMacroWindow{
		{kind: "yaw", startMs: 0, endMs: 250, expectedYawDegrees: 45},
		{kind: "yaw", startMs: 400, endMs: 650, expectedYawDegrees: 45},
		{kind: "yaw", startMs: 800, endMs: 1050, expectedYawDegrees: -45},
		{kind: "yaw", startMs: 1200, endMs: 1450, expectedYawDegrees: -45},
	}
	timeline := map[int]float64{
		0: 90,
		1: 88,
		2: 180,
		3: 178,
		4: 90,
		5: 88,
		6: 0,
		7: -2,
	}
	normalized, ok := movementMacroAffineNormalizeYawTimeline(track, windows, timeline, 0)
	if !ok {
		t.Fatalf("expected affine normalization to succeed")
	}
	firstValues := movementMacroWindowValues(track, normalized, windows[0], 0)
	secondValues := movementMacroWindowValues(track, normalized, windows[1], 0)
	if math.Abs(movementMacroMean(firstValues)-(-45)) > 10 {
		t.Fatalf("expected first normalized yaw window to be near -45deg, got %f", movementMacroMean(firstValues))
	}
	if math.Abs(movementMacroMean(secondValues)-(-90)) > 10 {
		t.Fatalf("expected second normalized yaw window to be near -90deg, got %f", movementMacroMean(secondValues))
	}
}

func TestMovementMacroCommandSequenceFitScoreRewardsMatchingPattern(t *testing.T) {
	stats := []movementMacroWindowStat{
		{window: movementMacroWindow{startMs: 0, dx: 1084}, mean: -45},
		{window: movementMacroWindow{startMs: 1000, dx: 1084}, mean: -90},
		{window: movementMacroWindow{startMs: 2000, dx: -1084}, mean: -45},
		{window: movementMacroWindow{startMs: 3000, dx: -1084}, mean: 0},
	}
	score := movementMacroCommandSequenceFitScore(stats, func(window movementMacroWindow) float64 {
		return -window.dx
	})
	if score <= 0 {
		t.Fatalf("expected positive score for matching command sequence, got %f", score)
	}
}

func TestMovementMacroCommandSequenceFitScorePenalizesFlatPattern(t *testing.T) {
	stats := []movementMacroWindowStat{
		{window: movementMacroWindow{startMs: 0, dx: 1084}, mean: 90},
		{window: movementMacroWindow{startMs: 1000, dx: 1084}, mean: 90.2},
		{window: movementMacroWindow{startMs: 2000, dx: -1084}, mean: 90.1},
		{window: movementMacroWindow{startMs: 3000, dx: -1084}, mean: 90.3},
	}
	score := movementMacroCommandSequenceFitScore(stats, func(window movementMacroWindow) float64 {
		return -window.dx
	})
	if score >= 0 {
		t.Fatalf("expected negative score for flat non-matching command sequence, got %f", score)
	}
}

func TestMovementMacroShortlistStreamsPitchKeepsMultipleFamilies(t *testing.T) {
	streams := make([]movementDirectionStream, 0, 220)
	for index := 0; index < 180; index++ {
		streams = append(streams, movementDirectionStream{
			source:       "quat-window-pitch:test",
			propID:       "quat",
			actorID:      "a",
			vectorOffset: index,
			samples:      make([]movementDirectionAngleSample, 20),
		})
	}
	streams = append(streams, movementDirectionStream{
		source:       "late-s16-actor-fused-delta-integrated",
		propID:       "scalar",
		actorID:      "b",
		vectorOffset: 72,
		samples:      make([]movementDirectionAngleSample, 30),
	})
	shortlisted := movementMacroShortlistStreams(streams, "pitch")
	foundScalar := false
	for _, stream := range shortlisted {
		if stream.propID == "scalar" {
			foundScalar = true
			break
		}
	}
	if !foundScalar {
		t.Fatalf("expected pitch shortlist to keep at least one late scalar family candidate")
	}
}

func TestMovementMacroShortlistStreamsYawKeepsSingleActorLatePairs(t *testing.T) {
	streams := make([]movementDirectionStream, 0, 220)
	for index := 0; index < 180; index++ {
		streams = append(streams, movementDirectionStream{
			source:       "late-s16-delta-integrated-scalar-pair-double-actor-fused",
			propID:       "fused",
			actorID:      "fused",
			vectorOffset: index,
			samples:      make([]movementDirectionAngleSample, 40),
		})
	}
	streams = append(streams, movementDirectionStream{
		source:       "late-s16-delta-integrated-scalar-pair-double",
		propID:       "single",
		actorID:      "actor-a",
		vectorOffset: 46,
		samples:      make([]movementDirectionAngleSample, 32),
	})
	shortlisted := movementMacroShortlistStreams(streams, "yaw")
	foundSingle := false
	for _, stream := range shortlisted {
		if stream.propID == "single" && stream.actorID == "actor-a" {
			foundSingle = true
			break
		}
	}
	if !foundSingle {
		t.Fatalf("expected yaw shortlist to keep at least one single-actor late pair candidate")
	}
}

func TestMovementMacroStabilizePitchTimelineKeepsContinuousBranch(t *testing.T) {
	t0 := 10.0
	t1 := 9.0
	t2 := 8.0
	t3 := 7.0
	track := MovementTrack{
		Samples: []MovementSample{
			{TimeInSeconds: &t0},
			{TimeInSeconds: &t1},
			{TimeInSeconds: &t2},
			{TimeInSeconds: &t3},
		},
	}
	windows := []movementMacroWindow{
		{kind: "yaw", startMs: 0, endMs: 1500},
		{kind: "pitch", startMs: 1500, endMs: 4000},
	}
	timeline := map[int]float64{
		0: -80,
		1: 78,
		2: 76,
		3: -74,
	}
	stable := movementMacroStabilizePitchTimeline(track, windows, timeline)
	if stable[1] >= 0 || stable[2] >= 0 {
		t.Fatalf("expected stabilizer to keep the negative pitch branch, got %+v", stable)
	}
}

func TestMovementMacroJointRefineYawPitchReducesCrossTalk(t *testing.T) {
	times := []float64{10.0, 9.75, 9.5, 9.25, 9.0, 8.75, 8.5, 8.25}
	track := MovementTrack{
		Samples: make([]MovementSample, len(times)),
	}
	for i := range times {
		track.Samples[i].TimeInSeconds = &times[i]
	}
	windows := []movementMacroWindow{
		{kind: "yaw", startMs: 0, endMs: 750},
		{kind: "pitch", startMs: 1000, endMs: 1750},
	}
	yawTruth := []float64{-15, -30, -45, -60, -60, -60, -60, -60}
	pitchTruth := []float64{0, 0, 0, 0, 10, 20, 30, 40}
	yawTimeline := map[int]float64{}
	pitchTimeline := map[int]float64{}
	for i := range yawTruth {
		yawTimeline[i] = movementWrapDegrees(yawTruth[i] + (0.5 * pitchTruth[i]))
		pitchTimeline[i] = movementFoldPitchDegrees(pitchTruth[i] + (0.25 * yawTruth[i]))
	}
	origYawLeak, _ := movementMacroAverageWindowSpanByKind(track, windows, yawTimeline, "pitch")
	origPitchLeak, _ := movementMacroAverageWindowSpanByKind(track, windows, pitchTimeline, "yaw")
	origYawSignal, _ := movementMacroAverageWindowSpanByKind(track, windows, yawTimeline, "yaw")
	origPitchSignal, _ := movementMacroAverageWindowSpanByKind(track, windows, pitchTimeline, "pitch")
	refinedYaw, refinedPitch := movementMacroJointRefineYawPitch(track, windows, yawTimeline, pitchTimeline)
	newYawLeak, _ := movementMacroAverageWindowSpanByKind(track, windows, refinedYaw, "pitch")
	newPitchLeak, _ := movementMacroAverageWindowSpanByKind(track, windows, refinedPitch, "yaw")
	newYawSignal, _ := movementMacroAverageWindowSpanByKind(track, windows, refinedYaw, "yaw")
	newPitchSignal, _ := movementMacroAverageWindowSpanByKind(track, windows, refinedPitch, "pitch")
	if newYawLeak >= origYawLeak {
		t.Fatalf("expected yaw leak to decrease, before=%f after=%f", origYawLeak, newYawLeak)
	}
	if newPitchLeak >= origPitchLeak {
		t.Fatalf("expected pitch leak to decrease, before=%f after=%f", origPitchLeak, newPitchLeak)
	}
	if newYawSignal < origYawSignal*0.75 {
		t.Fatalf("expected yaw signal to stay mostly intact, before=%f after=%f", origYawSignal, newYawSignal)
	}
	if newPitchSignal < origPitchSignal*0.75 {
		t.Fatalf("expected pitch signal to stay mostly intact, before=%f after=%f", origPitchSignal, newPitchSignal)
	}
}

func TestMovementMacroStabilizeYawTimelineFlattensPitchWindows(t *testing.T) {
	times := []float64{10.0, 9.75, 9.5, 9.25, 9.0, 8.75, 8.5, 8.25}
	track := MovementTrack{
		Samples: make([]MovementSample, len(times)),
	}
	for i := range times {
		track.Samples[i].TimeInSeconds = &times[i]
	}
	windows := []movementMacroWindow{
		{kind: "yaw", startMs: 0, endMs: 750},
		{kind: "pitch", startMs: 1000, endMs: 1750},
	}
	timeline := map[int]float64{
		0: -15,
		1: -30,
		2: -45,
		3: -60,
		4: -58,
		5: -42,
		6: -63,
		7: -47,
	}
	before, _ := movementMacroAverageWindowSpanByKind(track, windows, timeline, "pitch")
	afterTimeline := movementMacroStabilizeYawTimeline(track, windows, timeline)
	after, _ := movementMacroAverageWindowSpanByKind(track, windows, afterTimeline, "pitch")
	yawSignalBefore, _ := movementMacroAverageWindowSpanByKind(track, windows, timeline, "yaw")
	yawSignalAfter, _ := movementMacroAverageWindowSpanByKind(track, windows, afterTimeline, "yaw")
	if after >= before {
		t.Fatalf("expected pitch-window yaw span to shrink, before=%f after=%f", before, after)
	}
	if yawSignalAfter < yawSignalBefore*0.9 {
		t.Fatalf("expected yaw signal to remain mostly intact, before=%f after=%f", yawSignalBefore, yawSignalAfter)
	}
}

func TestMovementMacroStabilizePitchTimelineFlattensYawWindows(t *testing.T) {
	times := []float64{10.0, 9.75, 9.5, 9.25, 9.0, 8.75, 8.5, 8.25}
	track := MovementTrack{
		Samples: make([]MovementSample, len(times)),
	}
	for i := range times {
		track.Samples[i].TimeInSeconds = &times[i]
	}
	windows := []movementMacroWindow{
		{kind: "yaw", startMs: 0, endMs: 750},
		{kind: "pitch", startMs: 1000, endMs: 1750},
	}
	timeline := map[int]float64{
		0: -10,
		1: 12,
		2: -9,
		3: 11,
		4: 5,
		5: 15,
		6: 25,
		7: 35,
	}
	before, _ := movementMacroAverageWindowSpanByKind(track, windows, timeline, "yaw")
	afterTimeline := movementMacroStabilizePitchTimeline(track, windows, timeline)
	after, _ := movementMacroAverageWindowSpanByKind(track, windows, afterTimeline, "yaw")
	pitchSignalBefore, _ := movementMacroAverageWindowSpanByKind(track, windows, timeline, "pitch")
	pitchSignalAfter, _ := movementMacroAverageWindowSpanByKind(track, windows, afterTimeline, "pitch")
	if after >= before {
		t.Fatalf("expected yaw-window pitch span to shrink, before=%f after=%f", before, after)
	}
	if pitchSignalAfter < pitchSignalBefore*0.9 {
		t.Fatalf("expected pitch signal to remain mostly intact, before=%f after=%f", pitchSignalBefore, pitchSignalAfter)
	}
}

func TestMovementNormalizeMacroWindowsMarksWalkingOverlap(t *testing.T) {
	windows := movementNormalizeMacroWindows([]movementMacroCommand{
		{Type: "holdkey", Keys: []string{"w"}, At: "2000ms", Duration: "2000ms"},
		{Type: "mousemove", At: "2400ms", Duration: "260ms", DX: 1084},
		{Type: "mousemove", At: "3200ms", Duration: "260ms", DY: -480},
	})
	foundMove := false
	foundMoveYaw := false
	foundMovePitch := false
	for _, window := range windows {
		switch window.kind {
		case "move":
			foundMove = true
		case "move+yaw":
			foundMoveYaw = true
		case "move+pitch":
			foundMovePitch = true
		}
	}
	if !foundMove || !foundMoveYaw || !foundMovePitch {
		t.Fatalf("expected move, move+yaw, and move+pitch windows, got %+v", windows)
	}
}

func TestMovementMacroAverageWindowSpanByKindMatchesMovePrefixedKinds(t *testing.T) {
	t0 := 10.0
	t1 := 9.75
	t2 := 9.5
	track := MovementTrack{
		Samples: []MovementSample{
			{TimeInSeconds: &t0},
			{TimeInSeconds: &t1},
			{TimeInSeconds: &t2},
		},
	}
	windows := []movementMacroWindow{
		{kind: "move+yaw", startMs: 0, endMs: 750},
	}
	timeline := map[int]float64{
		0: 0,
		1: -30,
		2: -60,
	}
	span, count := movementMacroAverageWindowSpanByKind(track, windows, timeline, "yaw")
	if count != 1 || span <= 0 {
		t.Fatalf("expected move+yaw window to count as yaw, got span=%f count=%d", span, count)
	}
}

func TestMovementMacroStreamAllowedIncludesBruteforceFamilies(t *testing.T) {
	streams := []movementDirectionStream{
		{source: "vector"},
		{source: "target-pos-xy"},
		{source: "quat-window:wxyz"},
		{source: "late-s16"},
	}
	for _, stream := range streams {
		if !movementMacroStreamAllowed(stream) {
			t.Fatalf("expected source %q to be allowed for macro brute-force search", stream.source)
		}
	}
}

func TestMovementMacroUniqueDirectionStreamsDeduplicatesExactCopies(t *testing.T) {
	streams := []movementDirectionStream{
		{source: "late-s16", propID: "aa", actorID: "bb", vectorOffset: 42, axisA: "x", axisB: "y"},
		{source: "late-s16", propID: "aa", actorID: "bb", vectorOffset: 42, axisA: "x", axisB: "y"},
		{source: "quat-window:wxyz", propID: "aa", actorID: "bb", vectorOffset: 46, axisA: "+o46", axisB: "+o48"},
	}
	unique := movementMacroUniqueDirectionStreams(streams)
	if len(unique) != 2 {
		t.Fatalf("expected 2 unique streams, got %d", len(unique))
	}
}

func TestMovementMacroHasAxisWindowsRecognizesYawAndPitchFamilies(t *testing.T) {
	windows := []movementMacroWindow{
		{kind: "yaw"},
		{kind: "move+yaw"},
		{kind: "combined"},
		{kind: "move"},
	}
	if !movementMacroHasAxisWindows(windows, "yaw") {
		t.Fatalf("expected yaw windows to be detected")
	}
	if !movementMacroHasAxisWindows(windows, "pitch") {
		t.Fatalf("expected combined windows to count for pitch detection")
	}
	if movementMacroHasAxisWindows([]movementMacroWindow{{kind: "move"}, {kind: "idle"}}, "pitch") {
		t.Fatalf("expected move/idle only windows to not count as pitch")
	}
}

func TestMovementMergeMacroSearchPrefersMacroTimeline(t *testing.T) {
	base := MovementTrack{ActorID: "actor-a"}
	direction := movementDirectionSearchResult{
		timelineByActor: map[string]map[int]float64{
			"actor-a": {0: 10, 1: 20, 2: 30, 3: 40},
		},
	}
	macro := movementMacroSearchResult{
		summary: &MovementMacroSearch{},
		timelineByActor: map[string]map[int]float64{
			"actor-a": {0: 91, 1: 92},
		},
	}
	merged := movementMergeMacroSearch(direction, macro, []MovementTrack{base})
	if got := merged.timelineByActor["actor-a"][0]; got != 91 {
		t.Fatalf("expected macro timeline to override generic direction timeline, got %f", got)
	}
	if len(merged.timelineByActor["actor-a"]) != 2 {
		t.Fatalf("expected macro timeline length to be preserved, got %d", len(merged.timelineByActor["actor-a"]))
	}
}

func TestMovementMergeMacroSearchClearsGenericAxesWhenMacroHasNoReplacement(t *testing.T) {
	base := MovementTrack{ActorID: "actor-a"}
	direction := movementDirectionSearchResult{
		timelineByActor: map[string]map[int]float64{
			"actor-a": {0: 10},
		},
		derivedByActor: map[string]map[int]float64{
			"actor-a": {0: 20},
		},
		pitchTimelineByActor: map[string]map[int]float64{
			"actor-a": {0: 30},
		},
	}
	macro := movementMacroSearchResult{
		summary: &MovementMacroSearch{},
	}
	merged := movementMergeMacroSearch(direction, macro, []MovementTrack{base})
	if _, ok := merged.timelineByActor["actor-a"]; ok {
		t.Fatalf("expected generic yaw timeline to be cleared when macro has no replacement")
	}
	if _, ok := merged.derivedByActor["actor-a"]; ok {
		t.Fatalf("expected generic derived yaw timeline to be cleared when macro has no replacement")
	}
	if _, ok := merged.pitchTimelineByActor["actor-a"]; ok {
		t.Fatalf("expected generic pitch timeline to be cleared when macro has no replacement")
	}
}

func TestMovementMacroLockYawTimelineAppliesCumulativeSteps(t *testing.T) {
	times := []float64{10.0, 9.95, 9.85, 9.75, 9.65, 9.55, 9.45, 9.35}
	track := MovementTrack{Samples: make([]MovementSample, len(times))}
	for i := range times {
		track.Samples[i].TimeInSeconds = &times[i]
	}
	windows := []movementMacroWindow{
		{kind: "idle", startMs: 0, endMs: 75},
		{kind: "yaw", startMs: 100, endMs: 350, expectedYawDegrees: 45},
		{kind: "idle", startMs: 400, endMs: 475},
		{kind: "yaw", startMs: 500, endMs: 750, expectedYawDegrees: 45},
	}
	timeline := map[int]float64{
		0: 140,
		1: 141,
		2: 142,
		3: 143,
		4: 144,
		5: 145,
		6: 146,
		7: 147,
	}
	locked, ok := movementMacroLockYawTimeline(track, windows, timeline)
	if !ok {
		t.Fatalf("expected macro yaw lock to succeed")
	}
	if math.Abs(locked[0]-141) > 5 {
		t.Fatalf("expected initial baseline near 141, got %f", locked[0])
	}
	if locked[4] >= locked[2] {
		t.Fatalf("expected first yaw window to step downward, got %+v", locked)
	}
	if math.Abs(movementWrapDegrees(locked[7]-locked[6])-(-45)) > 8 {
		t.Fatalf("expected second yaw step to be near -45deg, got %+v", locked)
	}
	firstSpan := movementHeadingSequenceSpan([]float64{locked[2], locked[3], locked[4]})
	secondSpan := movementHeadingSequenceSpan([]float64{locked[6], locked[7]})
	if math.Abs(firstSpan-45) > 6 {
		t.Fatalf("expected first locked yaw span to be near 45deg, got %f", firstSpan)
	}
	if math.Abs(secondSpan-45) > 6 {
		t.Fatalf("expected second locked yaw span to be near 45deg, got %f", secondSpan)
	}
}

func TestMovementMacroWindowProgressFromTimelinePreservesRawShape(t *testing.T) {
	indexes := []int{10, 11, 12, 13}
	timeline := map[int]float64{
		10: 100,
		11: 95,
		12: 70,
		13: 55,
	}
	progresses, ok := movementMacroWindowProgressFromTimeline(indexes, timeline)
	if !ok {
		t.Fatalf("expected progress extraction to succeed")
	}
	if len(progresses) != len(indexes) {
		t.Fatalf("expected %d progresses, got %d", len(indexes), len(progresses))
	}
	if !(progresses[1] < progresses[2] && progresses[2] < progresses[3]) {
		t.Fatalf("expected monotonic raw-shape progress, got %+v", progresses)
	}
	if progresses[0] != 0 || progresses[len(progresses)-1] != 1 {
		t.Fatalf("expected progress to start at 0 and end at 1, got %+v", progresses)
	}
}
