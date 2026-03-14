package dissect

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

type BlenderCameraOptions struct {
	MovementOptions
	Track             string
	FPS               int
	FrameStep         int
	Scale             float64
	RotationMode      string
	YawAxis           string
	PitchAxis         string
	RollAxis          string
	YawSign           float64
	PitchSign         float64
	RollSign          float64
	LevelCamera       bool
	YawOffsetDeg      float64
	PitchOffsetDeg    float64
	RollOffsetDeg     float64
	CompareCandidates bool
	CandidateLimit    int
}

func (r *Reader) BlenderCameraScriptOptions(options BlenderCameraOptions) (string, error) {
	normalized := normalizeBlenderCameraOptions(options)
	outputs, err := r.blenderCameraMovementOutputs(normalized)
	if err != nil {
		return "", err
	}
	data, track, err := chooseBlenderCameraOutput(outputs, normalized.Track)
	if err != nil {
		return "", err
	}
	variants := r.blenderCameraVariants(data, track, normalized)
	if len(variants) <= 1 {
		return renderBlenderCameraScript(data, track, normalized), nil
	}
	return renderBlenderCameraScriptVariants(data, variants, normalized), nil
}

type blenderCameraVariant struct {
	Track         MovementTrack
	NameSuffix    string
	VariantKind   string
	VariantLabel  string
	PrimaryCamera bool
	Candidate     *MovementMacroTimelineCandidate
}

func (r *Reader) blenderCameraMovementOutputs(options BlenderCameraOptions) ([]MovementOutput, error) {
	base, err := r.MovementDataWithPlayersOptions(options.MovementOptions)
	if err != nil {
		return nil, err
	}
	outputs := []MovementOutput{base}
	if options.PositionPropID != "" || options.RotationPropID != "" || !blenderShouldScanCandidateProps(base.Header, options.Track) || blenderHasObviousCameraTrack(base, options.Track) {
		return outputs, nil
	}
	seen := map[string]bool{base.PositionPropID + ":" + base.RotationPropID: true}
	addOutput := func(positionPropID string, rotationPropID string) {
		key := positionPropID + ":" + rotationPropID
		if seen[key] {
			return
		}
		seen[key] = true
		trialOptions := options.MovementOptions
		trialOptions.PositionPropID = positionPropID
		trialOptions.RotationPropID = rotationPropID
		trialData, err := r.MovementDataWithPlayersOptions(trialOptions)
		if err != nil {
			return
		}
		outputs = append(outputs, trialData)
	}
	for i, candidate := range base.PositionPropCandidates {
		if i >= 3 {
			break
		}
		if candidate.PropID == "" || candidate.PropID == base.PositionPropID {
			continue
		}
		addOutput(candidate.PropID, base.RotationPropID)
	}
	for i, candidate := range base.RotationPropCandidates {
		if i >= 3 {
			break
		}
		if candidate.PropID == "" || candidate.PropID == base.RotationPropID {
			continue
		}
		addOutput(base.PositionPropID, candidate.PropID)
	}
	return outputs, nil
}

func blenderHasObviousCameraTrack(data MovementOutput, selector string) bool {
	tracks := data.PrimaryTracks
	if len(tracks) == 0 {
		tracks = data.Tracks
	}
	if len(tracks) <= 1 {
		return true
	}
	selector = strings.TrimSpace(selector)
	if selector != "" {
		_, err := selectBlenderCameraTrackWithFallback(data, selector, false)
		return err == nil
	}
	recording := data.Header.RecordingPlayer()
	if recording.Username == "" {
		return false
	}
	resolved := 0
	for _, track := range tracks {
		if strings.EqualFold(track.PlayerNameGuess, recording.Username) {
			resolved++
		}
	}
	return resolved == 1
}

func chooseBlenderCameraOutput(outputs []MovementOutput, selector string) (MovementOutput, MovementTrack, error) {
	if len(outputs) == 0 {
		return MovementOutput{}, MovementTrack{}, fmt.Errorf("no movement outputs available")
	}
	requireResolvedRecording := strings.TrimSpace(selector) == "" && outputs[0].Header.RecordingPlayer().Username != ""
	bestData, bestTrack, found, err := chooseBlenderCameraOutputPass(outputs, selector, !requireResolvedRecording)
	if err == nil && found {
		return bestData, bestTrack, nil
	}
	if requireResolvedRecording {
		bestData, bestTrack, found, err = chooseBlenderCameraOutputPass(outputs, selector, true)
		if err == nil && found {
			return bestData, bestTrack, nil
		}
	}
	if err != nil {
		return MovementOutput{}, MovementTrack{}, err
	}
	return MovementOutput{}, MovementTrack{}, fmt.Errorf("no camera track found")
}

func blenderShouldScanCandidateProps(header Header, selector string) bool {
	selector = strings.TrimSpace(selector)
	if selector == "" {
		return true
	}
	for _, player := range header.Players {
		if strings.EqualFold(player.Username, selector) {
			return true
		}
	}
	return false
}

func blenderCameraTrackScore(track MovementTrack) int {
	score := (len(track.Samples) * 1000) + (track.PositionSamples * 20) + (track.RotationSamples * 50)
	if track.PositionSamples == 0 {
		score -= 100000
	}
	if track.RotationSamples == 0 {
		score -= 50000
	}
	if movementTrackLooksOriginLike(track) {
		score -= 250000
	}
	return score
}

func normalizeBlenderCameraOptions(options BlenderCameraOptions) BlenderCameraOptions {
	options.Track = strings.TrimSpace(options.Track)
	if options.FPS <= 0 {
		options.FPS = 60
	}
	if options.FrameStep <= 0 {
		options.FrameStep = 1
	}
	if options.Scale == 0 {
		options.Scale = 1
	}
	options.RotationMode = strings.ToLower(strings.TrimSpace(options.RotationMode))
	if options.RotationMode != "raw" {
		options.RotationMode = "wrapped"
	}
	options.YawAxis = normalizeBlenderAxis(options.YawAxis, "z")
	options.PitchAxis = normalizeBlenderAxis(options.PitchAxis, "x")
	options.RollAxis = normalizeBlenderAxis(options.RollAxis, "y")
	options.YawSign = normalizeBlenderSign(options.YawSign)
	options.PitchSign = normalizeBlenderSign(options.PitchSign)
	options.RollSign = normalizeBlenderSign(options.RollSign)
	if options.CandidateLimit <= 0 {
		options.CandidateLimit = 3
	}
	return options
}

func normalizeBlenderAxis(value string, fallback string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "x", "y", "z":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return fallback
	}
}

func normalizeBlenderSign(value float64) float64 {
	if value == 0 {
		return 1
	}
	return value
}

func selectBlenderCameraTrack(data MovementOutput, selector string) (MovementTrack, error) {
	return selectBlenderCameraTrackWithFallback(data, selector, true)
}

func selectBlenderCameraTrackWithFallback(data MovementOutput, selector string, allowFallback bool) (MovementTrack, error) {
	tracks := data.PrimaryTracks
	if len(tracks) == 0 {
		tracks = data.Tracks
	}
	if len(tracks) == 0 {
		return MovementTrack{}, fmt.Errorf("no movement tracks available")
	}
	selector = strings.TrimSpace(selector)
	if selector == "" {
		recording := data.Header.RecordingPlayer()
		if recording.Username != "" {
			for _, track := range tracks {
				if strings.EqualFold(track.PlayerNameGuess, recording.Username) {
					return track, nil
				}
			}
			if !allowFallback {
				return MovementTrack{}, fmt.Errorf("recording player %q not resolved in current output", recording.Username)
			}
		}
		if !allowFallback {
			return MovementTrack{}, fmt.Errorf("no automatic camera track found")
		}
		return tracks[0], nil
	}
	normalized := strings.ToLower(selector)
	for _, track := range tracks {
		candidates := []string{
			track.ActorID,
			track.Label,
			track.PlayerNameGuess,
			blenderTrackDisplayName(track),
		}
		for _, candidate := range candidates {
			if candidate == "" {
				continue
			}
			candidateLower := strings.ToLower(candidate)
			if candidateLower == normalized || strings.HasPrefix(candidateLower, normalized) {
				return track, nil
			}
		}
	}
	available := make([]string, 0, len(tracks))
	for _, track := range tracks {
		available = append(available, fmt.Sprintf("%s=%s", blenderTrackDisplayName(track), track.ActorID))
	}
	return MovementTrack{}, fmt.Errorf("track %q not found; available: %s", selector, strings.Join(available, ", "))
}

func chooseBlenderCameraOutputPass(outputs []MovementOutput, selector string, allowFallback bool) (MovementOutput, MovementTrack, bool, error) {
	var bestData MovementOutput
	var bestTrack MovementTrack
	bestScore := 0
	found := false
	var lastErr error
	for _, output := range outputs {
		track, err := selectBlenderCameraTrackWithFallback(output, selector, allowFallback)
		if err != nil {
			lastErr = err
			continue
		}
		score := blenderCameraTrackScore(track)
		if !found || score > bestScore {
			bestData = output
			bestTrack = track
			bestScore = score
			found = true
		}
	}
	if found {
		return bestData, bestTrack, true, nil
	}
	return bestData, bestTrack, false, lastErr
}

func (r *Reader) blenderCameraVariants(data MovementOutput, track MovementTrack, options BlenderCameraOptions) []blenderCameraVariant {
	variants := []blenderCameraVariant{{
		Track:         track,
		VariantKind:   "selected",
		VariantLabel:  "Selected",
		PrimaryCamera: true,
	}}
	if !options.CompareCandidates || options.CandidateLimit <= 0 || data.MacroSearch == nil || len(track.Samples) == 0 {
		return variants
	}
	streams := r.blenderMacroCandidateStreams(data, track)
	if len(streams) == 0 {
		return variants
	}
	variants = append(variants, blenderCameraVariant{
		Track:        blenderTrackWithDerivedTimelines(track, nil, nil, true, true),
		NameSuffix:   "RawEntity",
		VariantKind:  "raw-entity",
		VariantLabel: "Raw Entity Rotation",
	})
	for index, candidate := range blenderMacroUniqueCandidates(data.MacroSearch.YawCandidates, data.MacroSearch.BestYaw, options.CandidateLimit) {
		timeline, ok := blenderMacroCandidateTimeline(track, candidate, "yaw", streams)
		if !ok {
			continue
		}
		variants = append(variants, blenderCameraVariant{
			Track:        blenderTrackWithDerivedTimelines(track, timeline, nil, true, false),
			NameSuffix:   blenderMacroCandidateSuffix("Yaw", index, candidate),
			VariantKind:  "yaw-candidate",
			VariantLabel: "Yaw Candidate " + strconv.Itoa(index+1),
			Candidate:    &candidate,
		})
	}
	for index, candidate := range blenderMacroUniqueCandidates(data.MacroSearch.PitchCandidates, data.MacroSearch.BestPitch, options.CandidateLimit) {
		timeline, ok := blenderMacroCandidateTimeline(track, candidate, "pitch", streams)
		if !ok {
			continue
		}
		variants = append(variants, blenderCameraVariant{
			Track:        blenderTrackWithDerivedTimelines(track, nil, timeline, false, true),
			NameSuffix:   blenderMacroCandidateSuffix("Pitch", index, candidate),
			VariantKind:  "pitch-candidate",
			VariantLabel: "Pitch Candidate " + strconv.Itoa(index+1),
			Candidate:    &candidate,
		})
	}
	return variants
}

func (r *Reader) blenderMacroCandidateStreams(data MovementOutput, track MovementTrack) []movementDirectionStream {
	zeroActorPrefixes := countMovementZeroActorPrefixes(r.Header.CodeVersion, r.b, r.offset)
	allowedProps := movementDirectionPropAllowlist(r.Header, r.b, r.offset, zeroActorPrefixes, track.ActorID)
	clockEntries, _, _, _, _ := movementRecoveredClockEntries(r.b)
	minSamples := movementDirectionMinSampleCount(r.Header)
	streams := movementDirectionTargetedLateOffsetStreams(r.Header, r.b, r.offset, zeroActorPrefixes, allowedProps, clockEntries)
	streams = append(streams, movementDirectionQuaternionStreams(r.Header, r.b, r.offset, zeroActorPrefixes, allowedProps, clockEntries)...)
	streams = append(streams, movementDirectionQuaternionPitchStreams(r.Header, r.b, r.offset, zeroActorPrefixes, allowedProps, clockEntries)...)
	streams = append(streams, movementDirectionTargetedAlignedWordDeltaStreams(r.Header, r.b, r.offset, zeroActorPrefixes, allowedProps, clockEntries)...)
	streams = append(streams, movementDirectionTargetedScalarPairStreams(streams, allowedProps, minSamples)...)
	props := map[string]bool{}
	if data.MacroSearch != nil {
		if data.MacroSearch.BestYaw != nil {
			props[normalizeMovementPropID(data.MacroSearch.BestYaw.PropID)] = true
		}
		if data.MacroSearch.BestPitch != nil {
			props[normalizeMovementPropID(data.MacroSearch.BestPitch.PropID)] = true
		}
		for _, candidate := range data.MacroSearch.YawCandidates {
			props[normalizeMovementPropID(candidate.PropID)] = true
		}
		for _, candidate := range data.MacroSearch.PitchCandidates {
			props[normalizeMovementPropID(candidate.PropID)] = true
		}
	}
	for propID := range props {
		if propID == "" {
			continue
		}
		streams = append(streams, movementMacroTriadPitchStreamsForProp(streams, propID, minSamples)...)
	}
	return streams
}

func blenderMacroUniqueCandidates(candidates []MovementMacroTimelineCandidate, best *MovementMacroTimelineCandidate, limit int) []MovementMacroTimelineCandidate {
	seen := map[string]bool{}
	bestKey := ""
	if best != nil {
		bestKey = blenderMacroCandidateFamilyKey(*best)
	}
	unique := make([]MovementMacroTimelineCandidate, 0, limit)
	for _, candidate := range candidates {
		key := blenderMacroCandidateFamilyKey(candidate)
		if key == "" || key == bestKey || seen[key] {
			continue
		}
		seen[key] = true
		unique = append(unique, candidate)
		if len(unique) >= limit {
			break
		}
	}
	return unique
}

func blenderMacroCandidateFamilyKey(candidate MovementMacroTimelineCandidate) string {
	source := strings.TrimSpace(candidate.Source)
	source = strings.ReplaceAll(source, "-actor-fused", "")
	source = strings.ReplaceAll(source, "actor-fused-", "")
	source = strings.ReplaceAll(source, "-neg", "")
	source = strings.ReplaceAll(source, "neg-", "")
	canonicalAxis := func(axis string) string {
		axis = strings.TrimSpace(axis)
		axis = strings.TrimPrefix(axis, "+")
		axis = strings.TrimPrefix(axis, "-")
		return axis
	}
	return strings.Join([]string{
		source,
		normalizeMovementPropID(candidate.PropID),
		strconv.Itoa(candidate.VectorOffset),
		canonicalAxis(candidate.AxisA),
		canonicalAxis(candidate.AxisB),
		strings.TrimSpace(candidate.Alignment),
		strings.TrimSpace(candidate.Transform),
	}, "|")
}

func blenderMacroCandidateSuffix(prefix string, index int, candidate MovementMacroTimelineCandidate) string {
	propID := normalizeMovementPropID(candidate.PropID)
	if len(propID) > 8 {
		propID = propID[:8]
	}
	source := sanitizeBlenderName(blenderMacroShortSource(candidate.Source))
	return fmt.Sprintf("%s%02d_%s_%s_O%d", prefix, index+1, source, propID, candidate.VectorOffset)
}

func blenderMacroShortSource(source string) string {
	source = strings.TrimSpace(source)
	switch {
	case strings.Contains(source, "scalar-pair"):
		return "scalar_pair"
	case strings.Contains(source, "word-hi"):
		return "word_hi"
	case strings.Contains(source, "word-lo"):
		return "word_lo"
	case strings.Contains(source, "quat"):
		return "quat"
	case strings.Contains(source, "late-s16"):
		return "late_s16"
	case strings.Contains(source, "late-u16"):
		return "late_u16"
	default:
		return source
	}
}

func blenderMacroCandidateTimeline(track MovementTrack, candidate MovementMacroTimelineCandidate, kind string, streams []movementDirectionStream) (map[int]float64, bool) {
	stream, ok := blenderFindMacroCandidateStream(streams, candidate)
	if !ok {
		return nil, false
	}
	transform, ok := blenderMacroTransformByName(kind, candidate.Transform)
	if !ok {
		return nil, false
	}
	timeline, ok := movementDirectionProjectStreamTimelineWithAlignment(track, stream, MovementDirectionCandidate{
		Alignment:             candidate.Alignment,
		ByteShift:             candidate.ByteShift,
		TimeShiftMilliseconds: candidate.TimeShiftMilliseconds,
	}, transform)
	if !ok {
		return nil, false
	}
	if candidate.GlobalShiftMilliseconds != 0 {
		timeline = movementMacroShiftTimelineByMilliseconds(track, timeline, candidate.GlobalShiftMilliseconds)
	}
	return timeline, len(timeline) > 0
}

func blenderFindMacroCandidateStream(streams []movementDirectionStream, candidate MovementMacroTimelineCandidate) (movementDirectionStream, bool) {
	for _, stream := range streams {
		if strings.TrimSpace(candidate.Source) != "" && stream.source != candidate.Source {
			continue
		}
		if normalizeMovementPropID(candidate.PropID) != "" && normalizeMovementPropID(stream.propID) != normalizeMovementPropID(candidate.PropID) {
			continue
		}
		if strings.TrimSpace(candidate.ActorID) != "" && stream.actorID != candidate.ActorID {
			continue
		}
		if stream.vectorOffset != candidate.VectorOffset {
			continue
		}
		if strings.TrimSpace(candidate.AxisA) != "" && stream.axisA != candidate.AxisA {
			continue
		}
		if strings.TrimSpace(candidate.AxisB) != "" && stream.axisB != candidate.AxisB {
			continue
		}
		return stream, true
	}
	return movementDirectionStream{}, false
}

func blenderMacroTransformByName(kind string, name string) (func(float64) float64, bool) {
	for _, transform := range movementMacroTransforms(kind) {
		if transform.name == name {
			return transform.fn, true
		}
	}
	return nil, false
}

func blenderTrackWithDerivedTimelines(track MovementTrack, heading map[int]float64, pitch map[int]float64, clearHeading bool, clearPitch bool) MovementTrack {
	cloned := track
	cloned.Samples = make([]MovementSample, len(track.Samples))
	copy(cloned.Samples, track.Samples)
	for index := range cloned.Samples {
		if clearHeading {
			cloned.Samples[index].ViewingDirectionDegrees = nil
		}
		if clearPitch {
			cloned.Samples[index].ViewPitchDegrees = nil
		}
	}
	for sampleIndex, value := range heading {
		if sampleIndex < 0 || sampleIndex >= len(cloned.Samples) {
			continue
		}
		copyValue := value
		cloned.Samples[sampleIndex].ViewingDirectionDegrees = &copyValue
	}
	for sampleIndex, value := range pitch {
		if sampleIndex < 0 || sampleIndex >= len(cloned.Samples) {
			continue
		}
		copyValue := value
		cloned.Samples[sampleIndex].ViewPitchDegrees = &copyValue
	}
	return cloned
}

func renderBlenderCameraScript(data MovementOutput, track MovementTrack, options BlenderCameraOptions) string {
	cameraBaseName := sanitizeBlenderName(blenderTrackDisplayName(track))
	cameraName := "R6Cam_" + cameraBaseName
	rigName := "R6Rig_" + cameraBaseName
	boundsName := "R6Bounds_" + cameraBaseName
	collectionName := "R6 Replay Cameras"
	bounds := blenderTrackBounds(track)
	stableIndex := blenderStableStartIndex(track.Samples)
	rawViewHeadingContinuous := blenderStableViewingHeadingContinuous(track.Samples, stableIndex)
	rawViewHeadingWrapped := blenderWrappedViewingHeadingFromContinuous(rawViewHeadingContinuous)
	rawViewPitch := blenderViewPitchDegrees(track.Samples)
	cameraViewHeadingContinuous := blenderSmoothScalarTimeline(rawViewHeadingContinuous, 2, 0.45, true)
	cameraViewHeadingWrapped := blenderWrappedViewingHeadingFromContinuous(cameraViewHeadingContinuous)
	cameraViewPitch := blenderSmoothScalarTimeline(rawViewPitch, 2, 0.35, false)
	frames := blenderSampleFrames(track.Samples, options.FPS, options.FrameStep)
	baselinePitch, baselineRoll := blenderBaselinePitchRoll(track.Samples, options, cameraViewHeadingContinuous, cameraViewPitch, stableIndex)
	var builder strings.Builder
	builder.WriteString("import bpy\n")
	builder.WriteString("import math\n\n")
	fmt.Fprintf(&builder, "COLLECTION_NAME = %q\n", collectionName)
	fmt.Fprintf(&builder, "CAMERA_NAME = %q\n", cameraName)
	fmt.Fprintf(&builder, "RIG_NAME = %q\n", rigName)
	fmt.Fprintf(&builder, "BOUNDS_NAME = %q\n", boundsName)
	fmt.Fprintf(&builder, "TRACK_NAME = %q\n", blenderTrackDisplayName(track))
	fmt.Fprintf(&builder, "TRACK_LABEL = %q\n", track.Label)
	fmt.Fprintf(&builder, "PLAYER_GUESS = %q\n", track.PlayerNameGuess)
	fmt.Fprintf(&builder, "ACTOR_ID = %q\n", track.ActorID)
	fmt.Fprintf(&builder, "POSITION_PROP_ID = %q\n", data.PositionPropID)
	fmt.Fprintf(&builder, "ROTATION_PROP_ID = %q\n", data.RotationPropID)
	fmt.Fprintf(&builder, "ROTATION_MODE = %q\n", options.RotationMode)
	fmt.Fprintf(&builder, "LEVEL_CAMERA = %t\n", options.LevelCamera)
	fmt.Fprintf(&builder, "YAW_OFFSET_DEG = %s\n", blenderFloatString(options.YawOffsetDeg))
	fmt.Fprintf(&builder, "PITCH_OFFSET_DEG = %s\n", blenderFloatString(options.PitchOffsetDeg))
	fmt.Fprintf(&builder, "ROLL_OFFSET_DEG = %s\n", blenderFloatString(options.RollOffsetDeg))
	fmt.Fprintf(&builder, "FPS = %d\n", options.FPS)
	fmt.Fprintf(&builder, "FRAME_STEP = %d\n", options.FrameStep)
	fmt.Fprintf(&builder, "LOCATION_SCALE = %s\n\n", blenderFloatString(options.Scale))
	if bounds != nil {
		fmt.Fprintf(&builder, "BOUNDS_MIN = (%s, %s, %s)\n",
			blenderFloatString(float64(bounds.Min.X)*options.Scale),
			blenderFloatString(float64(bounds.Min.Y)*options.Scale),
			blenderFloatString(float64(bounds.Min.Z)*options.Scale),
		)
		fmt.Fprintf(&builder, "BOUNDS_MAX = (%s, %s, %s)\n\n",
			blenderFloatString(float64(bounds.Max.X)*options.Scale),
			blenderFloatString(float64(bounds.Max.Y)*options.Scale),
			blenderFloatString(float64(bounds.Max.Z)*options.Scale),
		)
	} else {
		builder.WriteString("BOUNDS_MIN = None\n")
		builder.WriteString("BOUNDS_MAX = None\n\n")
	}
	builder.WriteString("def ensure_collection(name):\n")
	builder.WriteString("    collection = bpy.data.collections.get(name)\n")
	builder.WriteString("    if collection is None:\n")
	builder.WriteString("        collection = bpy.data.collections.new(name)\n")
	builder.WriteString("        bpy.context.scene.collection.children.link(collection)\n")
	builder.WriteString("    return collection\n\n")
	builder.WriteString("def remove_existing_object(name):\n")
	builder.WriteString("    obj = bpy.data.objects.get(name)\n")
	builder.WriteString("    if obj is not None:\n")
	builder.WriteString("        bpy.data.objects.remove(obj, do_unlink=True)\n\n")
	builder.WriteString("def keyframe_prop(obj, key, value, frame):\n")
	builder.WriteString("    obj[key] = value\n")
	builder.WriteString("    obj.keyframe_insert(data_path=f'[\"{key}\"]', frame=frame)\n\n")
	builder.WriteString("def create_bounds_object(name, bounds_min, bounds_max, collection):\n")
	builder.WriteString("    if bounds_min is None or bounds_max is None:\n")
	builder.WriteString("        return None\n")
	builder.WriteString("    min_x, min_y, min_z = bounds_min\n")
	builder.WriteString("    max_x, max_y, max_z = bounds_max\n")
	builder.WriteString("    verts = [\n")
	builder.WriteString("        (min_x, min_y, min_z),\n")
	builder.WriteString("        (max_x, min_y, min_z),\n")
	builder.WriteString("        (max_x, max_y, min_z),\n")
	builder.WriteString("        (min_x, max_y, min_z),\n")
	builder.WriteString("        (min_x, min_y, max_z),\n")
	builder.WriteString("        (max_x, min_y, max_z),\n")
	builder.WriteString("        (max_x, max_y, max_z),\n")
	builder.WriteString("        (min_x, max_y, max_z),\n")
	builder.WriteString("    ]\n")
	builder.WriteString("    edges = [\n")
	builder.WriteString("        (0, 1), (1, 2), (2, 3), (3, 0),\n")
	builder.WriteString("        (4, 5), (5, 6), (6, 7), (7, 4),\n")
	builder.WriteString("        (0, 4), (1, 5), (2, 6), (3, 7),\n")
	builder.WriteString("    ]\n")
	builder.WriteString("    mesh = bpy.data.meshes.new(name)\n")
	builder.WriteString("    mesh.from_pydata(verts, edges, [])\n")
	builder.WriteString("    mesh.update()\n")
	builder.WriteString("    obj = bpy.data.objects.new(name, mesh)\n")
	builder.WriteString("    obj.display_type = 'WIRE'\n")
	builder.WriteString("    obj.hide_render = True\n")
	builder.WriteString("    collection.objects.link(obj)\n")
	builder.WriteString("    return obj\n\n")
	builder.WriteString("collection = ensure_collection(COLLECTION_NAME)\n")
	builder.WriteString("remove_existing_object(CAMERA_NAME)\n")
	builder.WriteString("remove_existing_object(RIG_NAME)\n")
	builder.WriteString("remove_existing_object(BOUNDS_NAME)\n")
	builder.WriteString("rig = bpy.data.objects.new(RIG_NAME, None)\n")
	builder.WriteString("rig.empty_display_type = 'PLAIN_AXES'\n")
	builder.WriteString("rig.empty_display_size = 0.5\n")
	builder.WriteString("rig.rotation_mode = 'XYZ'\n")
	builder.WriteString("collection.objects.link(rig)\n")
	builder.WriteString("camera_data = bpy.data.cameras.new(CAMERA_NAME)\n")
	builder.WriteString("camera = bpy.data.objects.new(CAMERA_NAME, camera_data)\n")
	builder.WriteString("camera.rotation_mode = 'XYZ'\n")
	builder.WriteString("camera.data.clip_end = 10000\n")
	builder.WriteString("camera.parent = rig\n")
	builder.WriteString("camera.location = (0, 0, 0)\n")
	builder.WriteString("camera.rotation_euler = (math.radians(90), 0, 0)\n")
	builder.WriteString("collection.objects.link(camera)\n")
	builder.WriteString("bounds = create_bounds_object(BOUNDS_NAME, BOUNDS_MIN, BOUNDS_MAX, collection)\n")
	builder.WriteString("scene = bpy.context.scene\n")
	builder.WriteString("scene.camera = camera\n")
	builder.WriteString("scene.render.fps = FPS\n")
	builder.WriteString("camera['r6_rig_name'] = RIG_NAME\n")
	builder.WriteString("rig['r6_track_name'] = TRACK_NAME\n")
	builder.WriteString("rig['r6_track_label'] = TRACK_LABEL\n")
	builder.WriteString("rig['r6_player_guess'] = PLAYER_GUESS\n")
	builder.WriteString("rig['r6_actor_id'] = ACTOR_ID\n")
	builder.WriteString("rig['r6_position_prop'] = POSITION_PROP_ID\n")
	builder.WriteString("rig['r6_rotation_prop'] = ROTATION_PROP_ID\n")
	builder.WriteString("rig['r6_rotation_mode'] = ROTATION_MODE\n")
	builder.WriteString("rig['r6_level_camera'] = LEVEL_CAMERA\n")
	builder.WriteString("rig['r6_yaw_offset_deg'] = YAW_OFFSET_DEG\n")
	builder.WriteString("rig['r6_pitch_offset_deg'] = PITCH_OFFSET_DEG\n")
	builder.WriteString("rig['r6_roll_offset_deg'] = ROLL_OFFSET_DEG\n")
	builder.WriteString("rig['r6_frame_step'] = FRAME_STEP\n")
	builder.WriteString("rig['r6_scale'] = LOCATION_SCALE\n\n")
	builder.WriteString("if bounds is not None:\n")
	builder.WriteString("    bounds['r6_track_name'] = TRACK_NAME\n")
	builder.WriteString("    bounds['r6_player_guess'] = PLAYER_GUESS\n")
	builder.WriteString("    bounds['r6_bounds_min_x'] = BOUNDS_MIN[0]\n")
	builder.WriteString("    bounds['r6_bounds_min_y'] = BOUNDS_MIN[1]\n")
	builder.WriteString("    bounds['r6_bounds_min_z'] = BOUNDS_MIN[2]\n")
	builder.WriteString("    bounds['r6_bounds_max_x'] = BOUNDS_MAX[0]\n")
	builder.WriteString("    bounds['r6_bounds_max_y'] = BOUNDS_MAX[1]\n")
	builder.WriteString("    bounds['r6_bounds_max_z'] = BOUNDS_MAX[2]\n\n")
	lastFrame := 1
	for index, sample := range track.Samples {
		frame := frames[index]
		lastFrame = frame
		position := blenderSamplePosition(sample)
		rawRotation := blenderSampleRawRotation(sample)
		degreeRotation := blenderSampleDegreeRotation(sample)
		var headingRadians *float64
		if index < len(cameraViewHeadingContinuous) && cameraViewHeadingContinuous[index] != nil {
			value := degreesToRadians(*cameraViewHeadingContinuous[index])
			headingRadians = &value
		}
		cameraSample := sample
		if index < len(cameraViewPitch) && cameraViewPitch[index] != nil {
			value := *cameraViewPitch[index]
			cameraSample.ViewPitchDegrees = &value
		}
		cameraRotation := blenderCameraEuler(cameraSample, options, headingRadians)
		cameraRotation.X -= baselinePitch
		cameraRotation.Y -= baselineRoll
		fmt.Fprintf(&builder, "rig.location = (%s, %s, %s)\n",
			blenderFloatString(float64(position.X)*options.Scale),
			blenderFloatString(float64(position.Y)*options.Scale),
			blenderFloatString(float64(position.Z)*options.Scale),
		)
		fmt.Fprintf(&builder, "rig.rotation_euler = (%s, %s, %s)\n",
			blenderFloatString(cameraRotation.X),
			blenderFloatString(cameraRotation.Y),
			blenderFloatString(cameraRotation.Z),
		)
		fmt.Fprintf(&builder, "rig.keyframe_insert(data_path='location', frame=%d)\n", frame)
		fmt.Fprintf(&builder, "rig.keyframe_insert(data_path='rotation_euler', frame=%d)\n", frame)
		fmt.Fprintf(&builder, "keyframe_prop(rig, 'r6_raw_rot_x', %s, %d)\n", blenderFloatString(float64(rawRotation.X)), frame)
		fmt.Fprintf(&builder, "keyframe_prop(rig, 'r6_raw_rot_y', %s, %d)\n", blenderFloatString(float64(rawRotation.Y)), frame)
		fmt.Fprintf(&builder, "keyframe_prop(rig, 'r6_raw_rot_z', %s, %d)\n", blenderFloatString(float64(rawRotation.Z)), frame)
		fmt.Fprintf(&builder, "keyframe_prop(rig, 'r6_deg_rot_x', %s, %d)\n", blenderFloatString(float64(degreeRotation.X)), frame)
		fmt.Fprintf(&builder, "keyframe_prop(rig, 'r6_deg_rot_y', %s, %d)\n", blenderFloatString(float64(degreeRotation.Y)), frame)
		fmt.Fprintf(&builder, "keyframe_prop(rig, 'r6_deg_rot_z', %s, %d)\n", blenderFloatString(float64(degreeRotation.Z)), frame)
		if index < len(rawViewHeadingWrapped) && rawViewHeadingWrapped[index] != nil {
			fmt.Fprintf(&builder, "keyframe_prop(rig, 'r6_view_heading_deg', %s, %d)\n", blenderFloatString(*rawViewHeadingWrapped[index]), frame)
		}
		if index < len(rawViewHeadingContinuous) && rawViewHeadingContinuous[index] != nil {
			fmt.Fprintf(&builder, "keyframe_prop(rig, 'r6_view_heading_continuous_deg', %s, %d)\n", blenderFloatString(*rawViewHeadingContinuous[index]), frame)
		}
		if index < len(cameraViewHeadingWrapped) && cameraViewHeadingWrapped[index] != nil {
			fmt.Fprintf(&builder, "keyframe_prop(rig, 'r6_view_heading_smoothed_deg', %s, %d)\n", blenderFloatString(*cameraViewHeadingWrapped[index]), frame)
		}
		if index < len(cameraViewHeadingContinuous) && cameraViewHeadingContinuous[index] != nil {
			fmt.Fprintf(&builder, "keyframe_prop(rig, 'r6_view_heading_smoothed_continuous_deg', %s, %d)\n", blenderFloatString(*cameraViewHeadingContinuous[index]), frame)
		}
		if index < len(rawViewPitch) && rawViewPitch[index] != nil {
			fmt.Fprintf(&builder, "keyframe_prop(rig, 'r6_view_pitch_deg', %s, %d)\n", blenderFloatString(*rawViewPitch[index]), frame)
		}
		if index < len(cameraViewPitch) && cameraViewPitch[index] != nil {
			fmt.Fprintf(&builder, "keyframe_prop(rig, 'r6_view_pitch_smoothed_deg', %s, %d)\n", blenderFloatString(*cameraViewPitch[index]), frame)
		}
		fmt.Fprintf(&builder, "keyframe_prop(rig, 'r6_sample_offset', %d, %d)\n", sample.Offset, frame)
		if sample.TimeInSeconds != nil {
			fmt.Fprintf(&builder, "keyframe_prop(rig, 'r6_time_seconds', %s, %d)\n", blenderFloatString(*sample.TimeInSeconds), frame)
		}
		builder.WriteString("\n")
	}
	builder.WriteString("scene.frame_start = 1\n")
	fmt.Fprintf(&builder, "scene.frame_end = %d\n", lastFrame)
	builder.WriteString("if rig.animation_data and rig.animation_data.action:\n")
	builder.WriteString("    for fcurve in rig.animation_data.action.fcurves:\n")
	builder.WriteString("        for keyframe in fcurve.keyframe_points:\n")
	builder.WriteString("            keyframe.interpolation = 'LINEAR'\n")
	builder.WriteString("\n")
	builder.WriteString("print(f'Imported {TRACK_NAME} into Blender as {RIG_NAME} -> {CAMERA_NAME}')\n")
	return builder.String()
}

func renderBlenderCameraScriptVariants(data MovementOutput, variants []blenderCameraVariant, options BlenderCameraOptions) string {
	if len(variants) == 0 {
		return ""
	}
	collectionName := "R6 Replay Cameras"
	var builder strings.Builder
	builder.WriteString("import bpy\n")
	builder.WriteString("import math\n\n")
	fmt.Fprintf(&builder, "COLLECTION_NAME = %q\n", collectionName)
	fmt.Fprintf(&builder, "FPS = %d\n", options.FPS)
	fmt.Fprintf(&builder, "FRAME_STEP = %d\n", options.FrameStep)
	fmt.Fprintf(&builder, "LOCATION_SCALE = %s\n\n", blenderFloatString(options.Scale))
	builder.WriteString("def ensure_collection(name):\n")
	builder.WriteString("    collection = bpy.data.collections.get(name)\n")
	builder.WriteString("    if collection is None:\n")
	builder.WriteString("        collection = bpy.data.collections.new(name)\n")
	builder.WriteString("        bpy.context.scene.collection.children.link(collection)\n")
	builder.WriteString("    return collection\n\n")
	builder.WriteString("def remove_existing_object(name):\n")
	builder.WriteString("    obj = bpy.data.objects.get(name)\n")
	builder.WriteString("    if obj is not None:\n")
	builder.WriteString("        bpy.data.objects.remove(obj, do_unlink=True)\n\n")
	builder.WriteString("def keyframe_prop(obj, key, value, frame):\n")
	builder.WriteString("    obj[key] = value\n")
	builder.WriteString("    obj.keyframe_insert(data_path=f'[\"{key}\"]', frame=frame)\n\n")
	builder.WriteString("def create_bounds_object(name, bounds_min, bounds_max, collection):\n")
	builder.WriteString("    if bounds_min is None or bounds_max is None:\n")
	builder.WriteString("        return None\n")
	builder.WriteString("    min_x, min_y, min_z = bounds_min\n")
	builder.WriteString("    max_x, max_y, max_z = bounds_max\n")
	builder.WriteString("    verts = [\n")
	builder.WriteString("        (min_x, min_y, min_z),\n")
	builder.WriteString("        (max_x, min_y, min_z),\n")
	builder.WriteString("        (max_x, max_y, min_z),\n")
	builder.WriteString("        (min_x, max_y, min_z),\n")
	builder.WriteString("        (min_x, min_y, max_z),\n")
	builder.WriteString("        (max_x, min_y, max_z),\n")
	builder.WriteString("        (max_x, max_y, max_z),\n")
	builder.WriteString("        (min_x, max_y, max_z),\n")
	builder.WriteString("    ]\n")
	builder.WriteString("    edges = [\n")
	builder.WriteString("        (0, 1), (1, 2), (2, 3), (3, 0),\n")
	builder.WriteString("        (4, 5), (5, 6), (6, 7), (7, 4),\n")
	builder.WriteString("        (0, 4), (1, 5), (2, 6), (3, 7),\n")
	builder.WriteString("    ]\n")
	builder.WriteString("    mesh = bpy.data.meshes.new(name)\n")
	builder.WriteString("    mesh.from_pydata(verts, edges, [])\n")
	builder.WriteString("    mesh.update()\n")
	builder.WriteString("    obj = bpy.data.objects.new(name, mesh)\n")
	builder.WriteString("    obj.display_type = 'WIRE'\n")
	builder.WriteString("    obj.hide_render = True\n")
	builder.WriteString("    collection.objects.link(obj)\n")
	builder.WriteString("    return obj\n\n")
	builder.WriteString("collection = ensure_collection(COLLECTION_NAME)\n")
	builder.WriteString("scene = bpy.context.scene\n")
	builder.WriteString("scene.render.fps = FPS\n\n")

	lastFrame := 1
	sceneCameraAssigned := false
	for _, variant := range variants {
		setSceneCamera := variant.PrimaryCamera && !sceneCameraAssigned
		frame := appendBlenderCameraVariantScript(&builder, data, variant, options, setSceneCamera)
		if frame > lastFrame {
			lastFrame = frame
		}
		if setSceneCamera {
			sceneCameraAssigned = true
		}
	}
	builder.WriteString("scene.frame_start = 1\n")
	fmt.Fprintf(&builder, "scene.frame_end = %d\n", lastFrame)
	builder.WriteString("print('Imported R6 compare camera variants into Blender')\n")
	return builder.String()
}

func appendBlenderCameraVariantScript(builder *strings.Builder, data MovementOutput, variant blenderCameraVariant, options BlenderCameraOptions, setSceneCamera bool) int {
	baseDisplayName := blenderTrackDisplayName(variant.Track)
	if strings.TrimSpace(variant.NameSuffix) != "" {
		baseDisplayName += "__" + strings.TrimSpace(variant.NameSuffix)
	}
	cameraBaseName := sanitizeBlenderName(baseDisplayName)
	cameraName := "R6Cam_" + cameraBaseName
	rigName := "R6Rig_" + cameraBaseName
	boundsName := "R6Bounds_" + cameraBaseName
	bounds := blenderTrackBounds(variant.Track)
	stableIndex := blenderStableStartIndex(variant.Track.Samples)
	rawViewHeadingContinuous := blenderStableViewingHeadingContinuous(variant.Track.Samples, stableIndex)
	rawViewHeadingWrapped := blenderWrappedViewingHeadingFromContinuous(rawViewHeadingContinuous)
	rawViewPitch := blenderViewPitchDegrees(variant.Track.Samples)
	cameraViewHeadingContinuous := blenderSmoothScalarTimeline(rawViewHeadingContinuous, 2, 0.45, true)
	cameraViewHeadingWrapped := blenderWrappedViewingHeadingFromContinuous(cameraViewHeadingContinuous)
	cameraViewPitch := blenderSmoothScalarTimeline(rawViewPitch, 2, 0.35, false)
	frames := blenderSampleFrames(variant.Track.Samples, options.FPS, options.FrameStep)
	baselinePitch, baselineRoll := blenderBaselinePitchRoll(variant.Track.Samples, options, cameraViewHeadingContinuous, cameraViewPitch, stableIndex)

	fmt.Fprintf(builder, "remove_existing_object(%q)\n", cameraName)
	fmt.Fprintf(builder, "remove_existing_object(%q)\n", rigName)
	fmt.Fprintf(builder, "remove_existing_object(%q)\n", boundsName)
	fmt.Fprintf(builder, "rig = bpy.data.objects.new(%q, None)\n", rigName)
	builder.WriteString("rig.empty_display_type = 'PLAIN_AXES'\n")
	builder.WriteString("rig.empty_display_size = 0.5\n")
	builder.WriteString("rig.rotation_mode = 'XYZ'\n")
	builder.WriteString("collection.objects.link(rig)\n")
	fmt.Fprintf(builder, "camera_data = bpy.data.cameras.new(%q)\n", cameraName)
	fmt.Fprintf(builder, "camera = bpy.data.objects.new(%q, camera_data)\n", cameraName)
	builder.WriteString("camera.rotation_mode = 'XYZ'\n")
	builder.WriteString("camera.data.clip_end = 10000\n")
	builder.WriteString("camera.parent = rig\n")
	builder.WriteString("camera.location = (0, 0, 0)\n")
	builder.WriteString("camera.rotation_euler = (math.radians(90), 0, 0)\n")
	builder.WriteString("collection.objects.link(camera)\n")
	fmt.Fprintf(builder, "bounds = create_bounds_object(%q, %s, %s, collection)\n", boundsName, blenderBoundsTuple(bounds, options.Scale, false), blenderBoundsTuple(bounds, options.Scale, true))
	if setSceneCamera {
		builder.WriteString("scene.camera = camera\n")
	}
	fmt.Fprintf(builder, "camera['r6_rig_name'] = %q\n", rigName)
	fmt.Fprintf(builder, "rig['r6_track_name'] = %q\n", blenderTrackDisplayName(variant.Track))
	fmt.Fprintf(builder, "rig['r6_track_label'] = %q\n", variant.Track.Label)
	fmt.Fprintf(builder, "rig['r6_player_guess'] = %q\n", variant.Track.PlayerNameGuess)
	fmt.Fprintf(builder, "rig['r6_actor_id'] = %q\n", variant.Track.ActorID)
	fmt.Fprintf(builder, "rig['r6_position_prop'] = %q\n", data.PositionPropID)
	fmt.Fprintf(builder, "rig['r6_rotation_prop'] = %q\n", data.RotationPropID)
	fmt.Fprintf(builder, "rig['r6_rotation_mode'] = %q\n", options.RotationMode)
	fmt.Fprintf(builder, "rig['r6_level_camera'] = %t\n", options.LevelCamera)
	fmt.Fprintf(builder, "rig['r6_yaw_offset_deg'] = %s\n", blenderFloatString(options.YawOffsetDeg))
	fmt.Fprintf(builder, "rig['r6_pitch_offset_deg'] = %s\n", blenderFloatString(options.PitchOffsetDeg))
	fmt.Fprintf(builder, "rig['r6_roll_offset_deg'] = %s\n", blenderFloatString(options.RollOffsetDeg))
	fmt.Fprintf(builder, "rig['r6_frame_step'] = %d\n", options.FrameStep)
	fmt.Fprintf(builder, "rig['r6_scale'] = %s\n", blenderFloatString(options.Scale))
	fmt.Fprintf(builder, "rig['r6_variant_kind'] = %q\n", variant.VariantKind)
	fmt.Fprintf(builder, "rig['r6_variant_label'] = %q\n", variant.VariantLabel)
	if variant.Candidate != nil {
		fmt.Fprintf(builder, "rig['r6_candidate_source'] = %q\n", variant.Candidate.Source)
		fmt.Fprintf(builder, "rig['r6_candidate_prop'] = %q\n", variant.Candidate.PropID)
		fmt.Fprintf(builder, "rig['r6_candidate_actor'] = %q\n", variant.Candidate.ActorID)
		fmt.Fprintf(builder, "rig['r6_candidate_vector_offset'] = %d\n", variant.Candidate.VectorOffset)
		fmt.Fprintf(builder, "rig['r6_candidate_alignment'] = %q\n", variant.Candidate.Alignment)
		fmt.Fprintf(builder, "rig['r6_candidate_transform'] = %q\n", variant.Candidate.Transform)
		fmt.Fprintf(builder, "rig['r6_candidate_score'] = %s\n", blenderFloatString(variant.Candidate.Score))
	}
	if bounds != nil {
		fmt.Fprintf(builder, "if bounds is not None:\n")
		fmt.Fprintf(builder, "    bounds['r6_track_name'] = %q\n", blenderTrackDisplayName(variant.Track))
		fmt.Fprintf(builder, "    bounds['r6_player_guess'] = %q\n", variant.Track.PlayerNameGuess)
		fmt.Fprintf(builder, "    bounds['r6_variant_kind'] = %q\n", variant.VariantKind)
		fmt.Fprintf(builder, "    bounds['r6_bounds_min_x'] = %s\n", blenderFloatString(float64(bounds.Min.X)*options.Scale))
		fmt.Fprintf(builder, "    bounds['r6_bounds_min_y'] = %s\n", blenderFloatString(float64(bounds.Min.Y)*options.Scale))
		fmt.Fprintf(builder, "    bounds['r6_bounds_min_z'] = %s\n", blenderFloatString(float64(bounds.Min.Z)*options.Scale))
		fmt.Fprintf(builder, "    bounds['r6_bounds_max_x'] = %s\n", blenderFloatString(float64(bounds.Max.X)*options.Scale))
		fmt.Fprintf(builder, "    bounds['r6_bounds_max_y'] = %s\n", blenderFloatString(float64(bounds.Max.Y)*options.Scale))
		fmt.Fprintf(builder, "    bounds['r6_bounds_max_z'] = %s\n", blenderFloatString(float64(bounds.Max.Z)*options.Scale))
	}
	builder.WriteString("\n")
	lastFrame := 1
	for index, sample := range variant.Track.Samples {
		frame := frames[index]
		lastFrame = frame
		position := blenderSamplePosition(sample)
		rawRotation := blenderSampleRawRotation(sample)
		degreeRotation := blenderSampleDegreeRotation(sample)
		var headingRadians *float64
		if index < len(cameraViewHeadingContinuous) && cameraViewHeadingContinuous[index] != nil {
			value := degreesToRadians(*cameraViewHeadingContinuous[index])
			headingRadians = &value
		}
		cameraSample := sample
		if index < len(cameraViewPitch) && cameraViewPitch[index] != nil {
			value := *cameraViewPitch[index]
			cameraSample.ViewPitchDegrees = &value
		}
		cameraRotation := blenderCameraEuler(cameraSample, options, headingRadians)
		cameraRotation.X -= baselinePitch
		cameraRotation.Y -= baselineRoll
		fmt.Fprintf(builder, "rig.location = (%s, %s, %s)\n",
			blenderFloatString(float64(position.X)*options.Scale),
			blenderFloatString(float64(position.Y)*options.Scale),
			blenderFloatString(float64(position.Z)*options.Scale),
		)
		fmt.Fprintf(builder, "rig.rotation_euler = (%s, %s, %s)\n",
			blenderFloatString(cameraRotation.X),
			blenderFloatString(cameraRotation.Y),
			blenderFloatString(cameraRotation.Z),
		)
		fmt.Fprintf(builder, "rig.keyframe_insert(data_path='location', frame=%d)\n", frame)
		fmt.Fprintf(builder, "rig.keyframe_insert(data_path='rotation_euler', frame=%d)\n", frame)
		fmt.Fprintf(builder, "keyframe_prop(rig, 'r6_raw_rot_x', %s, %d)\n", blenderFloatString(float64(rawRotation.X)), frame)
		fmt.Fprintf(builder, "keyframe_prop(rig, 'r6_raw_rot_y', %s, %d)\n", blenderFloatString(float64(rawRotation.Y)), frame)
		fmt.Fprintf(builder, "keyframe_prop(rig, 'r6_raw_rot_z', %s, %d)\n", blenderFloatString(float64(rawRotation.Z)), frame)
		fmt.Fprintf(builder, "keyframe_prop(rig, 'r6_deg_rot_x', %s, %d)\n", blenderFloatString(float64(degreeRotation.X)), frame)
		fmt.Fprintf(builder, "keyframe_prop(rig, 'r6_deg_rot_y', %s, %d)\n", blenderFloatString(float64(degreeRotation.Y)), frame)
		fmt.Fprintf(builder, "keyframe_prop(rig, 'r6_deg_rot_z', %s, %d)\n", blenderFloatString(float64(degreeRotation.Z)), frame)
		if index < len(rawViewHeadingWrapped) && rawViewHeadingWrapped[index] != nil {
			fmt.Fprintf(builder, "keyframe_prop(rig, 'r6_view_heading_deg', %s, %d)\n", blenderFloatString(*rawViewHeadingWrapped[index]), frame)
		}
		if index < len(rawViewHeadingContinuous) && rawViewHeadingContinuous[index] != nil {
			fmt.Fprintf(builder, "keyframe_prop(rig, 'r6_view_heading_continuous_deg', %s, %d)\n", blenderFloatString(*rawViewHeadingContinuous[index]), frame)
		}
		if index < len(cameraViewHeadingWrapped) && cameraViewHeadingWrapped[index] != nil {
			fmt.Fprintf(builder, "keyframe_prop(rig, 'r6_view_heading_smoothed_deg', %s, %d)\n", blenderFloatString(*cameraViewHeadingWrapped[index]), frame)
		}
		if index < len(cameraViewHeadingContinuous) && cameraViewHeadingContinuous[index] != nil {
			fmt.Fprintf(builder, "keyframe_prop(rig, 'r6_view_heading_smoothed_continuous_deg', %s, %d)\n", blenderFloatString(*cameraViewHeadingContinuous[index]), frame)
		}
		if index < len(rawViewPitch) && rawViewPitch[index] != nil {
			fmt.Fprintf(builder, "keyframe_prop(rig, 'r6_view_pitch_deg', %s, %d)\n", blenderFloatString(*rawViewPitch[index]), frame)
		}
		if index < len(cameraViewPitch) && cameraViewPitch[index] != nil {
			fmt.Fprintf(builder, "keyframe_prop(rig, 'r6_view_pitch_smoothed_deg', %s, %d)\n", blenderFloatString(*cameraViewPitch[index]), frame)
		}
		fmt.Fprintf(builder, "keyframe_prop(rig, 'r6_sample_offset', %d, %d)\n", sample.Offset, frame)
		if sample.TimeInSeconds != nil {
			fmt.Fprintf(builder, "keyframe_prop(rig, 'r6_time_seconds', %s, %d)\n", blenderFloatString(*sample.TimeInSeconds), frame)
		}
		builder.WriteString("\n")
	}
	builder.WriteString("if rig.animation_data and rig.animation_data.action:\n")
	builder.WriteString("    for fcurve in rig.animation_data.action.fcurves:\n")
	builder.WriteString("        for keyframe in fcurve.keyframe_points:\n")
	builder.WriteString("            keyframe.interpolation = 'LINEAR'\n")
	builder.WriteString("\n")
	return lastFrame
}

func blenderBoundsTuple(bounds *MovementBounds, scale float64, useMax bool) string {
	if bounds == nil {
		return "None"
	}
	value := bounds.Min
	if useMax {
		value = bounds.Max
	}
	return fmt.Sprintf("(%s, %s, %s)",
		blenderFloatString(float64(value.X)*scale),
		blenderFloatString(float64(value.Y)*scale),
		blenderFloatString(float64(value.Z)*scale),
	)
}

func blenderTrackDisplayName(track MovementTrack) string {
	if track.PlayerNameGuess != "" && track.Label != "" {
		return track.PlayerNameGuess + "_" + track.Label
	}
	if track.PlayerNameGuess != "" {
		return track.PlayerNameGuess
	}
	if track.Label != "" {
		return track.Label
	}
	if len(track.ActorID) >= 8 {
		return "actor_" + track.ActorID[:8]
	}
	return "track"
}

func sanitizeBlenderName(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "track"
	}
	var builder strings.Builder
	underscore := false
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') {
			builder.WriteRune(char)
			underscore = false
			continue
		}
		if !underscore {
			builder.WriteByte('_')
			underscore = true
		}
	}
	result := strings.Trim(builder.String(), "_")
	if result == "" {
		return "track"
	}
	return result
}

func blenderSamplePosition(sample MovementSample) Vector3 {
	if sample.Position != nil {
		return *sample.Position
	}
	return Vector3{}
}

func blenderSampleRawRotation(sample MovementSample) Vector3 {
	if sample.Rotation != nil {
		return *sample.Rotation
	}
	return Vector3{}
}

func blenderSampleDegreeRotation(sample MovementSample) Vector3 {
	if sample.RotationDegrees != nil {
		return *sample.RotationDegrees
	}
	if sample.Rotation != nil {
		rotation := rotationDegreesVector(*sample.Rotation)
		return rotation
	}
	return Vector3{}
}

func blenderTrackBounds(track MovementTrack) *MovementBounds {
	if track.Bounds != nil {
		return track.Bounds
	}
	var bounds *MovementBounds
	for _, sample := range track.Samples {
		if sample.Position == nil {
			continue
		}
		pos := *sample.Position
		if bounds == nil {
			bounds = &MovementBounds{Min: pos, Max: pos}
			continue
		}
		if pos.X < bounds.Min.X {
			bounds.Min.X = pos.X
		}
		if pos.Y < bounds.Min.Y {
			bounds.Min.Y = pos.Y
		}
		if pos.Z < bounds.Min.Z {
			bounds.Min.Z = pos.Z
		}
		if pos.X > bounds.Max.X {
			bounds.Max.X = pos.X
		}
		if pos.Y > bounds.Max.Y {
			bounds.Max.Y = pos.Y
		}
		if pos.Z > bounds.Max.Z {
			bounds.Max.Z = pos.Z
		}
	}
	return bounds
}

type blenderEuler struct {
	X float64
	Y float64
	Z float64
}

func blenderCameraEuler(sample MovementSample, options BlenderCameraOptions, headingRadians *float64) blenderEuler {
	pitch := options.PitchSign * blenderPitchValue(sample, options.PitchAxis, options.RotationMode)
	roll := options.RollSign * blenderRotationValue(sample, options.RollAxis, options.RotationMode)
	yaw := options.YawSign * blenderYawValue(sample, options.RotationMode, headingRadians)
	if headingRadians != nil && options.LevelCamera {
		pitch = 0
		roll = 0
	}
	pitch += degreesToRadians(options.PitchOffsetDeg)
	roll += degreesToRadians(options.RollOffsetDeg)
	yaw += degreesToRadians(options.YawOffsetDeg)
	return blenderEuler{
		X: pitch,
		Y: roll,
		Z: yaw,
	}
}

func blenderPitchValue(sample MovementSample, axis string, mode string) float64 {
	if sample.ViewPitchDegrees != nil {
		return degreesToRadians(*sample.ViewPitchDegrees)
	}
	return blenderRotationValue(sample, axis, mode)
}

func blenderRotationValue(sample MovementSample, axis string, mode string) float64 {
	axis = normalizeBlenderAxis(axis, "z")
	if mode == "raw" {
		if sample.Rotation != nil {
			return float64(blenderAxisComponent(*sample.Rotation, axis))
		}
		if sample.RotationDegrees != nil {
			return degreesToRadians(float64(blenderAxisComponent(*sample.RotationDegrees, axis)))
		}
		return 0
	}
	if sample.RotationDegrees != nil {
		return degreesToRadians(float64(blenderAxisComponent(*sample.RotationDegrees, axis)))
	}
	if sample.Rotation != nil {
		return wrapRadians(float64(blenderAxisComponent(*sample.Rotation, axis)))
	}
	return 0
}

func blenderYawValue(sample MovementSample, mode string, headingRadians *float64) float64 {
	if headingRadians != nil {
		if mode == "raw" {
			return *headingRadians
		}
		return wrapRadians(*headingRadians)
	}
	return blenderRotationValue(sample, "z", mode)
}

func blenderWrappedViewingHeadingDegrees(samples []MovementSample) []*float64 {
	values := make([]*float64, len(samples))
	for index, sample := range samples {
		if sample.ViewingDirectionDegrees == nil {
			continue
		}
		value := *sample.ViewingDirectionDegrees
		values[index] = &value
	}
	return values
}

func blenderViewPitchDegrees(samples []MovementSample) []*float64 {
	values := make([]*float64, len(samples))
	for index, sample := range samples {
		if sample.ViewPitchDegrees == nil {
			continue
		}
		value := *sample.ViewPitchDegrees
		values[index] = &value
	}
	return values
}

func blenderContinuousViewingHeadingDegrees(samples []MovementSample) []*float64 {
	values := make([]*float64, len(samples))
	var previous float64
	var hasPrevious bool
	for index, sample := range samples {
		if sample.ViewingDirectionDegrees == nil {
			continue
		}
		current := *sample.ViewingDirectionDegrees
		if !hasPrevious {
			value := current
			values[index] = &value
			previous = current
			hasPrevious = true
			continue
		}
		delta := current - previous
		for delta > 180 {
			current -= 360
			delta = current - previous
		}
		for delta < -180 {
			current += 360
			delta = current - previous
		}
		value := current
		values[index] = &value
		previous = current
	}
	return values
}

func blenderWrappedViewingHeadingFromContinuous(values []*float64) []*float64 {
	wrapped := make([]*float64, len(values))
	for index, value := range values {
		if value == nil {
			continue
		}
		current := wrapDegrees(*value)
		wrapped[index] = &current
	}
	return wrapped
}

func blenderStableStartIndex(samples []MovementSample) int {
	for index, sample := range samples {
		if sample.Position != nil {
			return index
		}
	}
	return 0
}

func blenderStableViewingHeadingContinuous(samples []MovementSample, stableIndex int) []*float64 {
	values := blenderContinuousViewingHeadingDegrees(samples)
	if stableIndex <= 0 || stableIndex >= len(values) || values[stableIndex] == nil {
		return values
	}
	var headingMin float64
	var headingMax float64
	var rawZMin float64
	var rawZMax float64
	headingCount := 0
	rawCount := 0
	for index := 0; index < stableIndex; index++ {
		if values[index] != nil {
			value := *values[index]
			if headingCount == 0 || value < headingMin {
				headingMin = value
			}
			if headingCount == 0 || value > headingMax {
				headingMax = value
			}
			headingCount++
		}
		if samples[index].RotationDegrees != nil {
			value := float64(samples[index].RotationDegrees.Z)
			if rawCount == 0 || value < rawZMin {
				rawZMin = value
			}
			if rawCount == 0 || value > rawZMax {
				rawZMax = value
			}
			rawCount++
		}
	}
	if headingCount < 4 || rawCount < 4 {
		return values
	}
	headingSpan := math.Abs(headingMax - headingMin)
	rawSpan := math.Abs(rawZMax - rawZMin)
	if headingSpan < 180 || rawSpan > 15 {
		return values
	}
	baseline := *values[stableIndex]
	for index := 0; index < stableIndex; index++ {
		if values[index] == nil {
			continue
		}
		value := baseline
		values[index] = &value
	}
	return values
}

func blenderBaselinePitchRoll(samples []MovementSample, options BlenderCameraOptions, headings []*float64, pitches []*float64, stableIndex int) (float64, float64) {
	if len(samples) == 0 {
		return 0, 0
	}
	if stableIndex < 0 || stableIndex >= len(samples) {
		stableIndex = 0
	}
	var headingRadians *float64
	if stableIndex < len(headings) && headings[stableIndex] != nil {
		value := degreesToRadians(*headings[stableIndex])
		headingRadians = &value
	}
	sample := samples[stableIndex]
	if pitches != nil && stableIndex < len(pitches) && pitches[stableIndex] != nil {
		value := *pitches[stableIndex]
		sample.ViewPitchDegrees = &value
	}
	base := blenderCameraEuler(sample, options, headingRadians)
	if options.LevelCamera {
		return 0, 0
	}
	return base.X, base.Y
}

func blenderSampleFrames(samples []MovementSample, fps int, frameStep int) []int {
	if frameStep <= 0 {
		frameStep = 1
	}
	frames := make([]int, len(samples))
	startSeconds, ok := blenderFirstTimedSampleSeconds(samples)
	if !ok {
		for index := range samples {
			frames[index] = 1 + (index * frameStep)
		}
		return frames
	}
	lastFrame := 1
	for index, sample := range samples {
		frame := 1 + (index * frameStep)
		if sample.TimeInSeconds != nil {
			elapsedSeconds := startSeconds - *sample.TimeInSeconds
			if elapsedSeconds < 0 {
				elapsedSeconds = 0
			}
			frame = 1 + (int(math.Round(elapsedSeconds*float64(fps))) * frameStep)
			if frame <= lastFrame && index > 0 {
				frame = lastFrame + frameStep
			}
		}
		frames[index] = frame
		lastFrame = frame
	}
	return frames
}

func blenderFirstTimedSampleSeconds(samples []MovementSample) (float64, bool) {
	for _, sample := range samples {
		if sample.TimeInSeconds != nil {
			return *sample.TimeInSeconds, true
		}
	}
	return 0, false
}

func blenderSmoothScalarTimeline(values []*float64, radius int, alpha float64, wrap bool) []*float64 {
	if len(values) == 0 {
		return values
	}
	if radius < 0 {
		radius = 0
	}
	if alpha <= 0 || alpha > 1 {
		alpha = 0.4
	}
	series := make([]*float64, len(values))
	for index, value := range values {
		if value == nil {
			continue
		}
		copied := *value
		series[index] = &copied
	}
	if wrap {
		series = blenderContinuousViewingHeadingFromPointers(series)
	}
	filtered := make([]*float64, len(series))
	for index, value := range series {
		if value == nil {
			continue
		}
		window := make([]float64, 0, (radius*2)+1)
		for offset := -radius; offset <= radius; offset++ {
			otherIndex := index + offset
			if otherIndex < 0 || otherIndex >= len(series) || series[otherIndex] == nil {
				continue
			}
			window = append(window, *series[otherIndex])
		}
		if len(window) == 0 {
			continue
		}
		sort.Float64s(window)
		median := window[len(window)/2]
		filtered[index] = &median
	}
	smoothed := make([]*float64, len(filtered))
	var previous float64
	havePrevious := false
	for index, value := range filtered {
		if value == nil {
			continue
		}
		current := *value
		if !havePrevious {
			smoothed[index] = &current
			previous = current
			havePrevious = true
			continue
		}
		current = previous + (alpha * (current - previous))
		smoothed[index] = &current
		previous = current
	}
	return smoothed
}

func blenderContinuousViewingHeadingFromPointers(values []*float64) []*float64 {
	out := make([]*float64, len(values))
	var previous float64
	var hasPrevious bool
	for index, value := range values {
		if value == nil {
			continue
		}
		current := *value
		if !hasPrevious {
			out[index] = &current
			previous = current
			hasPrevious = true
			continue
		}
		delta := current - previous
		for delta > 180 {
			current -= 360
			delta = current - previous
		}
		for delta < -180 {
			current += 360
			delta = current - previous
		}
		out[index] = &current
		previous = current
	}
	return out
}

func blenderAxisComponent(vector Vector3, axis string) float32 {
	switch axis {
	case "x":
		return vector.X
	case "y":
		return vector.Y
	default:
		return vector.Z
	}
}

func blenderFloatString(value float64) string {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return "0.0"
	}
	return strconv.FormatFloat(value, 'f', -1, 64)
}

func wrapDegrees(value float64) float64 {
	for value > 180 {
		value -= 360
	}
	for value < -180 {
		value += 360
	}
	return value
}

func degreesToRadians(value float64) float64 {
	return (value * math.Pi) / 180
}
