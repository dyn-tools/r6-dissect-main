package dissect

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

type BlenderCameraOptions struct {
	MovementOptions
	Track          string
	FPS            int
	FrameStep      int
	Scale          float64
	RotationMode   string
	YawAxis        string
	PitchAxis      string
	RollAxis       string
	YawSign        float64
	PitchSign      float64
	RollSign       float64
	LevelCamera    bool
	YawOffsetDeg   float64
	PitchOffsetDeg float64
	RollOffsetDeg  float64
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
	return renderBlenderCameraScript(data, track, normalized), nil
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

func renderBlenderCameraScript(data MovementOutput, track MovementTrack, options BlenderCameraOptions) string {
	cameraBaseName := sanitizeBlenderName(blenderTrackDisplayName(track))
	cameraName := "R6Cam_" + cameraBaseName
	rigName := "R6Rig_" + cameraBaseName
	boundsName := "R6Bounds_" + cameraBaseName
	collectionName := "R6 Replay Cameras"
	bounds := blenderTrackBounds(track)
	stableIndex := blenderStableStartIndex(track.Samples)
	viewHeadingContinuous := blenderStableViewingHeadingContinuous(track.Samples, stableIndex)
	viewHeadingWrapped := blenderWrappedViewingHeadingFromContinuous(viewHeadingContinuous)
	baselinePitch, baselineRoll := blenderBaselinePitchRoll(track.Samples, options, viewHeadingContinuous, stableIndex)
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
	builder.WriteString("camera.rotation_euler = (0, 0, 0)\n")
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
		frame := 1 + (index * options.FrameStep)
		lastFrame = frame
		position := blenderSamplePosition(sample)
		rawRotation := blenderSampleRawRotation(sample)
		degreeRotation := blenderSampleDegreeRotation(sample)
		var headingRadians *float64
		if index < len(viewHeadingContinuous) && viewHeadingContinuous[index] != nil {
			value := degreesToRadians(*viewHeadingContinuous[index])
			headingRadians = &value
		}
		cameraRotation := blenderCameraEuler(sample, options, headingRadians)
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
		if index < len(viewHeadingWrapped) && viewHeadingWrapped[index] != nil {
			fmt.Fprintf(&builder, "keyframe_prop(rig, 'r6_view_heading_deg', %s, %d)\n", blenderFloatString(*viewHeadingWrapped[index]), frame)
		}
		if index < len(viewHeadingContinuous) && viewHeadingContinuous[index] != nil {
			fmt.Fprintf(&builder, "keyframe_prop(rig, 'r6_view_heading_continuous_deg', %s, %d)\n", blenderFloatString(*viewHeadingContinuous[index]), frame)
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

func blenderBaselinePitchRoll(samples []MovementSample, options BlenderCameraOptions, headings []*float64, stableIndex int) (float64, float64) {
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
	base := blenderCameraEuler(samples[stableIndex], options, headingRadians)
	if options.LevelCamera {
		return 0, 0
	}
	return base.X, base.Y
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
