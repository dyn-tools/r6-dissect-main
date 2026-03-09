package dissect

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

type BlenderCameraOptions struct {
	MovementOptions
	Track        string
	FPS          int
	FrameStep    int
	Scale        float64
	RotationMode string
	YawAxis      string
	PitchAxis    string
	RollAxis     string
	YawSign      float64
	PitchSign    float64
	RollSign     float64
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
	if options.PositionPropID != "" || options.RotationPropID != "" || !blenderShouldScanCandidateProps(base.Header, options.Track) {
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
	collectionName := "R6 Replay Cameras"
	var builder strings.Builder
	builder.WriteString("import bpy\n")
	builder.WriteString("import math\n\n")
	fmt.Fprintf(&builder, "COLLECTION_NAME = %q\n", collectionName)
	fmt.Fprintf(&builder, "CAMERA_NAME = %q\n", cameraName)
	fmt.Fprintf(&builder, "TRACK_NAME = %q\n", blenderTrackDisplayName(track))
	fmt.Fprintf(&builder, "TRACK_LABEL = %q\n", track.Label)
	fmt.Fprintf(&builder, "PLAYER_GUESS = %q\n", track.PlayerNameGuess)
	fmt.Fprintf(&builder, "ACTOR_ID = %q\n", track.ActorID)
	fmt.Fprintf(&builder, "POSITION_PROP_ID = %q\n", data.PositionPropID)
	fmt.Fprintf(&builder, "ROTATION_PROP_ID = %q\n", data.RotationPropID)
	fmt.Fprintf(&builder, "ROTATION_MODE = %q\n", options.RotationMode)
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
	builder.WriteString("collection = ensure_collection(COLLECTION_NAME)\n")
	builder.WriteString("remove_existing_object(CAMERA_NAME)\n")
	builder.WriteString("camera_data = bpy.data.cameras.new(CAMERA_NAME)\n")
	builder.WriteString("camera = bpy.data.objects.new(CAMERA_NAME, camera_data)\n")
	builder.WriteString("camera.rotation_mode = 'XYZ'\n")
	builder.WriteString("camera.data.clip_end = 10000\n")
	builder.WriteString("collection.objects.link(camera)\n")
	builder.WriteString("scene = bpy.context.scene\n")
	builder.WriteString("scene.camera = camera\n")
	builder.WriteString("scene.render.fps = FPS\n")
	builder.WriteString("camera['r6_track_name'] = TRACK_NAME\n")
	builder.WriteString("camera['r6_track_label'] = TRACK_LABEL\n")
	builder.WriteString("camera['r6_player_guess'] = PLAYER_GUESS\n")
	builder.WriteString("camera['r6_actor_id'] = ACTOR_ID\n")
	builder.WriteString("camera['r6_position_prop'] = POSITION_PROP_ID\n")
	builder.WriteString("camera['r6_rotation_prop'] = ROTATION_PROP_ID\n")
	builder.WriteString("camera['r6_rotation_mode'] = ROTATION_MODE\n")
	builder.WriteString("camera['r6_frame_step'] = FRAME_STEP\n")
	builder.WriteString("camera['r6_scale'] = LOCATION_SCALE\n\n")
	lastFrame := 1
	for index, sample := range track.Samples {
		frame := 1 + (index * options.FrameStep)
		lastFrame = frame
		position := blenderSamplePosition(sample)
		rawRotation := blenderSampleRawRotation(sample)
		degreeRotation := blenderSampleDegreeRotation(sample)
		cameraRotation := blenderCameraEuler(sample, options)
		fmt.Fprintf(&builder, "camera.location = (%s, %s, %s)\n",
			blenderFloatString(float64(position.X)*options.Scale),
			blenderFloatString(float64(position.Y)*options.Scale),
			blenderFloatString(float64(position.Z)*options.Scale),
		)
		fmt.Fprintf(&builder, "camera.rotation_euler = (%s, %s, %s)\n",
			blenderFloatString(cameraRotation.X),
			blenderFloatString(cameraRotation.Y),
			blenderFloatString(cameraRotation.Z),
		)
		fmt.Fprintf(&builder, "camera.keyframe_insert(data_path='location', frame=%d)\n", frame)
		fmt.Fprintf(&builder, "camera.keyframe_insert(data_path='rotation_euler', frame=%d)\n", frame)
		fmt.Fprintf(&builder, "keyframe_prop(camera, 'r6_raw_rot_x', %s, %d)\n", blenderFloatString(float64(rawRotation.X)), frame)
		fmt.Fprintf(&builder, "keyframe_prop(camera, 'r6_raw_rot_y', %s, %d)\n", blenderFloatString(float64(rawRotation.Y)), frame)
		fmt.Fprintf(&builder, "keyframe_prop(camera, 'r6_raw_rot_z', %s, %d)\n", blenderFloatString(float64(rawRotation.Z)), frame)
		fmt.Fprintf(&builder, "keyframe_prop(camera, 'r6_deg_rot_x', %s, %d)\n", blenderFloatString(float64(degreeRotation.X)), frame)
		fmt.Fprintf(&builder, "keyframe_prop(camera, 'r6_deg_rot_y', %s, %d)\n", blenderFloatString(float64(degreeRotation.Y)), frame)
		fmt.Fprintf(&builder, "keyframe_prop(camera, 'r6_deg_rot_z', %s, %d)\n", blenderFloatString(float64(degreeRotation.Z)), frame)
		fmt.Fprintf(&builder, "keyframe_prop(camera, 'r6_sample_offset', %d, %d)\n", sample.Offset, frame)
		if sample.TimeInSeconds != nil {
			fmt.Fprintf(&builder, "keyframe_prop(camera, 'r6_time_seconds', %s, %d)\n", blenderFloatString(*sample.TimeInSeconds), frame)
		}
		builder.WriteString("\n")
	}
	builder.WriteString("scene.frame_start = 1\n")
	fmt.Fprintf(&builder, "scene.frame_end = %d\n", lastFrame)
	builder.WriteString("if camera.animation_data and camera.animation_data.action:\n")
	builder.WriteString("    for fcurve in camera.animation_data.action.fcurves:\n")
	builder.WriteString("        for keyframe in fcurve.keyframe_points:\n")
	builder.WriteString("            keyframe.interpolation = 'LINEAR'\n")
	builder.WriteString("\n")
	builder.WriteString("print(f'Imported {TRACK_NAME} into Blender as {CAMERA_NAME}')\n")
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

type blenderEuler struct {
	X float64
	Y float64
	Z float64
}

func blenderCameraEuler(sample MovementSample, options BlenderCameraOptions) blenderEuler {
	return blenderEuler{
		X: options.PitchSign * blenderRotationValue(sample, options.PitchAxis, options.RotationMode),
		Y: options.RollSign * blenderRotationValue(sample, options.RollAxis, options.RotationMode),
		Z: options.YawSign * blenderRotationValue(sample, options.YawAxis, options.RotationMode),
	}
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

func degreesToRadians(value float64) float64 {
	return (value * math.Pi) / 180
}
