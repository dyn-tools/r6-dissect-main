package dissect

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

const movementPrimaryTrackMinPositions = 20
const movementZeroActorPrefixThreshold = 20
const movementStaticTrackSampleThreshold = 100
const movementStaticTrackDistanceMax = 1.0

var (
	movementPositionProp  = []byte{0x00, 0x00, 0x00, 0x00, 0x23, 0xCF, 0x03, 0xF0, 0x00, 0x00, 0x00, 0x00}
	movementRotationProp  = []byte{0x00, 0x00, 0x00, 0x00, 0x3A, 0xA4, 0x40, 0x5A, 0x61, 0x00, 0x00, 0x00}
	movementRecordMarker  = []byte{0x60, 0x73, 0x85, 0xFE}
	movementRecordTail    = []byte{0x00, 0x00, 0x00, 0x00}
	movementTimePattern   = []byte{0x1F, 0x07, 0xEF, 0xC9}
	movementY7TimePattern = []byte{0x1E, 0xF1, 0x11, 0xAB}
)

type Vector3 struct {
	X float32 `json:"x"`
	Y float32 `json:"y"`
	Z float32 `json:"z"`
}

type MovementBounds struct {
	Min Vector3 `json:"min"`
	Max Vector3 `json:"max"`
}

type MovementSample struct {
	Offset                  int      `json:"offset"`
	Time                    string   `json:"time,omitempty"`
	TimeInSeconds           *float64 `json:"timeInSeconds,omitempty"`
	Changed                 string   `json:"changed"`
	Position                *Vector3 `json:"position,omitempty"`
	Rotation                *Vector3 `json:"rotation,omitempty"`
	RotationDegrees         *Vector3 `json:"rotationDegrees,omitempty"`
	ViewingDirectionDegrees *float64 `json:"viewingDirectionDegrees,omitempty"`
}

type MovementTrack struct {
	ActorID           string           `json:"actorID"`
	Label             string           `json:"label,omitempty"`
	LikelyPlayer      bool             `json:"likelyPlayer,omitempty"`
	StartGroup        int              `json:"startGroup,omitempty"`
	TeamIndexGuess    *int             `json:"teamIndexGuess,omitempty"`
	TeamRoleGuess     TeamRole         `json:"teamRoleGuess,omitempty"`
	PlayerNameGuess   string           `json:"playerNameGuess,omitempty"`
	PlayerGuessSource string           `json:"playerGuessSource,omitempty"`
	OperatorGuess     Operator         `json:"operatorGuess,omitempty"`
	SpawnGuess        string           `json:"spawnGuess,omitempty"`
	PositionSamples   int              `json:"positionSamples"`
	RotationSamples   int              `json:"rotationSamples"`
	FirstOffset       int              `json:"firstOffset"`
	LastOffset        int              `json:"lastOffset"`
	FirstPosition     *Vector3         `json:"firstPosition,omitempty"`
	LastPosition      *Vector3         `json:"lastPosition,omitempty"`
	FirstRotation     *Vector3         `json:"firstRotation,omitempty"`
	LastRotation      *Vector3         `json:"lastRotation,omitempty"`
	Distance          float64          `json:"distance,omitempty"`
	Bounds            *MovementBounds  `json:"bounds,omitempty"`
	Samples           []MovementSample `json:"samples"`
}

type MovementOutput struct {
	Header                 Header                   `json:"header"`
	Tracks                 []MovementTrack          `json:"tracks"`
	PrimaryTracks          []MovementTrack          `json:"primaryTracks,omitempty"`
	Clock                  *MovementClock           `json:"clock,omitempty"`
	Usage                  *MovementUsage           `json:"usage,omitempty"`
	Validation             *MovementValidation      `json:"validation,omitempty"`
	DirectionSearch        *MovementDirectionSearch `json:"directionSearch,omitempty"`
	PositionPropID         string                   `json:"positionPropID,omitempty"`
	RotationPropID         string                   `json:"rotationPropID,omitempty"`
	PositionPropCandidates []MovementPropCandidate  `json:"positionPropCandidates,omitempty"`
	RotationPropCandidates []MovementPropCandidate  `json:"rotationPropCandidates,omitempty"`
}

type movementState struct {
	position *Vector3
	rotation *Vector3
}

type movementQuaternion struct {
	X float32
	Y float32
	Z float32
	W float32
}

type movementImplicitQuaternionModel struct {
	name  string
	mode  string
	order [3]int
	signs [3]float32
}

var movementDefaultImplicitQuaternionModel = movementImplicitQuaternionModel{
	name:  "xyz",
	mode:  "implicit_w",
	order: [3]int{0, 1, 2},
	signs: [3]float32{1, 1, 1},
}

type movementCandidate struct {
	actor         []byte
	prop          []byte
	primary       Vector3
	secondary     Vector3
	hasSecondary  bool
	quaternion    movementQuaternion
	hasQuaternion bool
	tailZero      bool
}

func (r *Reader) MovementDataWithPlayers() (MovementOutput, error) {
	return r.MovementDataWithPlayersOptions(MovementOptions{})
}

func (r *Reader) MovementDataWithPlayersOptions(options MovementOptions) (MovementOutput, error) {
	data := append([]byte(nil), r.b...)
	offset := r.offset
	if err := r.Read(); !Ok(err) {
		return MovementOutput{}, err
	}
	feedback := append([]MatchUpdate(nil), r.MatchFeedback...)
	r.b = data
	r.offset = offset
	r.MatchFeedback = feedback
	return r.MovementDataOptions(options), nil
}

func (r *Reader) MovementData() MovementOutput {
	return r.MovementDataOptions(MovementOptions{})
}

func (r *Reader) MovementDataOptions(options MovementOptions) MovementOutput {
	zeroActorPrefixes := countMovementZeroActorPrefixes(r.Header.CodeVersion, r.b, r.offset)
	selection := discoverMovementPropSelection(r.Header, r.b, r.offset, zeroActorPrefixes, options)
	ordered, primary := movementTracksForProps(r.Header, r.MatchFeedback, r.b, r.offset, selection.positionProp, selection.rotationProp, zeroActorPrefixes, true)
	ordered, primary = movementAutoTuneSinglePlayerRotation(r.Header, r.MatchFeedback, r.b, r.offset, selection.positionProp, selection.rotationProp, zeroActorPrefixes, ordered, primary)
	ordered, primary, clock := movementAttachRecoveredClock(r.Header.CodeVersion, r.b, ordered, primary)
	direction := movementDirectionSearchWithDerived(r.Header, r.b, r.offset, zeroActorPrefixes, primary)
	ordered, primary = movementApplyDerivedViewingDirections(ordered, primary, direction)
	validation := movementValidation(primary)
	return MovementOutput{
		Header:                 r.Header,
		Tracks:                 ordered,
		PrimaryTracks:          primary,
		Clock:                  clock,
		Usage:                  movementUsageStats(r.Header.CodeVersion, r.b, r.offset, zeroActorPrefixes, selection.positionProp, selection.rotationProp, ordered, primary, direction),
		Validation:             validation,
		DirectionSearch:        direction.summary,
		PositionPropID:         hex.EncodeToString(selection.positionProp),
		RotationPropID:         hex.EncodeToString(selection.rotationProp),
		PositionPropCandidates: selection.positionCandidates,
		RotationPropCandidates: selection.rotationCandidates,
	}
}

func movementApplyDerivedViewingDirections(ordered []MovementTrack, primary []MovementTrack, direction movementDirectionSearchResult) ([]MovementTrack, []MovementTrack) {
	if len(direction.derivedByActor) == 0 && len(direction.timelineByActor) == 0 {
		return ordered, primary
	}
	for trackIndex := range ordered {
		if timeline, ok := direction.timelineByActor[ordered[trackIndex].ActorID]; ok {
			for sampleIndex, viewingDegrees := range timeline {
				if sampleIndex < 0 || sampleIndex >= len(ordered[trackIndex].Samples) {
					continue
				}
				value := viewingDegrees
				ordered[trackIndex].Samples[sampleIndex].ViewingDirectionDegrees = &value
			}
		}
		if derived, ok := direction.derivedByActor[ordered[trackIndex].ActorID]; ok {
			for sampleIndex, viewingDegrees := range derived {
				if sampleIndex < 0 || sampleIndex >= len(ordered[trackIndex].Samples) {
					continue
				}
				if ordered[trackIndex].Samples[sampleIndex].ViewingDirectionDegrees != nil {
					continue
				}
				value := viewingDegrees
				ordered[trackIndex].Samples[sampleIndex].ViewingDirectionDegrees = &value
			}
		}
	}
	return ordered, movementPrimaryTracksFromOrdered(ordered, primary)
}

func movementTracksForProps(header Header, feedback []MatchUpdate, buf []byte, start int, positionProp []byte, rotationProp []byte, zeroActorPrefixes map[string]int, annotate bool) ([]MovementTrack, []MovementTrack) {
	return movementTracksForPropsWithModel(header, feedback, buf, start, positionProp, rotationProp, zeroActorPrefixes, annotate, movementDefaultImplicitQuaternionModel)
}

func movementTracksForPropsWithModel(header Header, feedback []MatchUpdate, buf []byte, start int, positionProp []byte, rotationProp []byte, zeroActorPrefixes map[string]int, annotate bool, model movementImplicitQuaternionModel) ([]MovementTrack, []MovementTrack) {
	tracks := map[string]*MovementTrack{}
	states := map[string]*movementState{}
	currentTime := 0.0
	currentTimeRaw := ""
	for i := start; i < len(buf); i++ {
		if raw, seconds, ok := movementTimeAt(header.CodeVersion, buf, i); ok {
			if seconds > 0 || currentTimeRaw != "" {
				currentTime = seconds
				currentTimeRaw = raw
			}
			if header.CodeVersion >= Y8S1 {
				i += 8
			} else {
				i += 4 + len(raw)
			}
			continue
		}
		actor, changed, value, ok := movementRecordAtWithPropsWithModel(header.CodeVersion, buf, i, positionProp, rotationProp, zeroActorPrefixes, model)
		if !ok {
			continue
		}
		actorID := hex.EncodeToString(actor)
		track := tracks[actorID]
		if track == nil {
			track = &MovementTrack{ActorID: actorID}
			tracks[actorID] = track
		}
		state := states[actorID]
		if state == nil {
			state = &movementState{}
			states[actorID] = state
		}
		sample := MovementSample{Offset: i, Changed: changed}
		if currentTimeRaw != "" {
			timeValue := currentTime
			sample.Time = currentTimeRaw
			sample.TimeInSeconds = &timeValue
		}
		switch changed {
		case "position":
			track.PositionSamples++
			vec := value
			state.position = &vec
		case "rotation":
			track.RotationSamples++
			vec := value
			state.rotation = &vec
		}
		if state.position != nil {
			pos := *state.position
			sample.Position = &pos
		}
		if state.rotation != nil {
			rot := *state.rotation
			sample.Rotation = &rot
			rotDegrees := rotationDegreesVector(rot)
			sample.RotationDegrees = &rotDegrees
		}
		track.Samples = append(track.Samples, sample)
		if header.CodeVersion < Y11S1Alpha3 {
			i += 37
		}
	}

	ordered := make([]MovementTrack, 0, len(tracks))
	for _, track := range tracks {
		ordered = append(ordered, *track)
	}
	for i := range ordered {
		ordered[i] = enrichMovementTrack(ordered[i])
	}
	if header.CodeVersion >= Y11S1Alpha3 {
		ordered = mergeSoloRotationTracks(header, ordered)
	}
	sort.Slice(ordered, func(i, j int) bool {
		left := movementTrackSortScore(ordered[i])
		right := movementTrackSortScore(ordered[j])
		if left == right {
			return ordered[i].ActorID < ordered[j].ActorID
		}
		return left > right
	})
	if !annotate {
		return ordered, nil
	}
	primary := selectPrimaryMovementTracks(header, ordered)
	primary = annotatePrimaryMovementTracks(header, feedback, primary)
	annotated := map[string]MovementTrack{}
	for _, track := range primary {
		annotated[track.ActorID] = track
	}
	for i := range ordered {
		if track, ok := annotated[ordered[i].ActorID]; ok {
			ordered[i] = track
		}
	}
	return ordered, primary
}

func movementRecordAt(buf []byte, propOffset int, zeroActorPrefixes map[string]int) ([]byte, string, Vector3, bool) {
	return movementRecordAtWithProps(Y10S3_1, buf, propOffset, movementPositionProp, movementRotationProp, zeroActorPrefixes)
}

func movementRecordAtWithProps(codeVersion int, buf []byte, propOffset int, positionProp []byte, rotationProp []byte, zeroActorPrefixes map[string]int) ([]byte, string, Vector3, bool) {
	return movementRecordAtWithPropsWithModel(codeVersion, buf, propOffset, positionProp, rotationProp, zeroActorPrefixes, movementDefaultImplicitQuaternionModel)
}

func movementRecordAtWithPropsWithModel(codeVersion int, buf []byte, propOffset int, positionProp []byte, rotationProp []byte, zeroActorPrefixes map[string]int, model movementImplicitQuaternionModel) ([]byte, string, Vector3, bool) {
	candidate, ok := movementCandidateRecordAtCodeVersion(codeVersion, buf, propOffset, zeroActorPrefixes)
	if !ok {
		return nil, "", Vector3{}, false
	}
	if codeVersion >= Y11S1Alpha3 && movementMatchAt(candidate.prop, 0, positionProp) && movementMatchAt(candidate.prop, 0, rotationProp) {
		switch {
		case movementY11CandidateIsPosition(candidate):
			value, _ := movementY11CandidatePosition(candidate)
			return candidate.actor, "position", value, true
		case movementY11CandidateIsRotationWithModel(candidate, model):
			quat, _ := movementY11CandidateQuaternionWithModel(candidate, model)
			return candidate.actor, "rotation", movementQuaternionToEuler(quat), true
		default:
			return nil, "", Vector3{}, false
		}
	}
	if movementMatchAt(candidate.prop, 0, positionProp) {
		if codeVersion >= Y11S1Alpha3 {
			if !movementY11CandidateIsPosition(candidate) {
				return nil, "", Vector3{}, false
			}
			value, ok := movementY11CandidatePosition(candidate)
			if !ok {
				return nil, "", Vector3{}, false
			}
			return candidate.actor, "position", value, true
		}
		return candidate.actor, "position", candidate.primary, true
	}
	if movementMatchAt(candidate.prop, 0, rotationProp) {
		if codeVersion >= Y11S1Alpha3 {
			if !movementY11CandidateIsRotationWithModel(candidate, model) {
				return nil, "", Vector3{}, false
			}
			quat, _ := movementY11CandidateQuaternionWithModel(candidate, model)
			return candidate.actor, "rotation", movementQuaternionToEuler(quat), true
		}
		return candidate.actor, "rotation", candidate.primary, true
	}
	return nil, "", Vector3{}, false
}

func movementCandidateRecordAt(buf []byte, propOffset int, zeroActorPrefixes map[string]int) ([]byte, []byte, Vector3, bool) {
	candidate, ok := movementCandidateRecordAtCodeVersion(Y10S3_1, buf, propOffset, zeroActorPrefixes)
	if !ok {
		return nil, nil, Vector3{}, false
	}
	return candidate.actor, candidate.prop, candidate.primary, true
}

func movementCandidateRecordAtCodeVersion(codeVersion int, buf []byte, propOffset int, zeroActorPrefixes map[string]int) (movementCandidate, bool) {
	if codeVersion >= Y11S1Alpha3 {
		return movementCandidateRecordAtY11(buf, propOffset, zeroActorPrefixes)
	}
	return movementCandidateRecordAtLegacy(buf, propOffset, zeroActorPrefixes)
}

func movementCandidateRecordAtLegacy(buf []byte, propOffset int, zeroActorPrefixes map[string]int) (movementCandidate, bool) {
	if propOffset < 8 || propOffset+38 > len(buf) {
		return movementCandidate{}, false
	}
	if !movementMatchAt(buf, propOffset+16, movementRecordMarker) {
		return movementCandidate{}, false
	}
	if !movementMatchAt(buf, propOffset+34, movementRecordTail) {
		return movementCandidate{}, false
	}
	primary, ok := movementVectorAt(buf, propOffset+22)
	if !ok {
		return movementCandidate{}, false
	}
	return movementCandidate{
		actor:    movementActorKeyAt(buf, propOffset, zeroActorPrefixes),
		prop:     append([]byte(nil), buf[propOffset:propOffset+12]...),
		primary:  primary,
		tailZero: true,
	}, true
}

func movementCandidateRecordAtY11(buf []byte, propOffset int, zeroActorPrefixes map[string]int) (movementCandidate, bool) {
	if propOffset < 8 || propOffset+43 > len(buf) {
		return movementCandidate{}, false
	}
	if !movementMatchAt(buf, propOffset+16, movementRecordMarker) {
		return movementCandidate{}, false
	}
	primary, ok := movementVectorAt(buf, propOffset+22)
	if !ok {
		return movementCandidate{}, false
	}
	secondary, hasSecondary := movementVectorAt(buf, propOffset+31)
	quaternion, hasQuaternion := movementQuaternionAt(buf, propOffset+22)
	return movementCandidate{
		actor:         movementActorKeyAt(buf, propOffset, zeroActorPrefixes),
		prop:          append([]byte(nil), buf[propOffset:propOffset+12]...),
		primary:       primary,
		secondary:     secondary,
		hasSecondary:  hasSecondary,
		quaternion:    quaternion,
		hasQuaternion: hasQuaternion,
		tailZero:      movementMatchAt(buf, propOffset+34, movementRecordTail),
	}, true
}

func movementQuaternionAt(buf []byte, offset int) (movementQuaternion, bool) {
	if offset < 0 || offset+16 > len(buf) {
		return movementQuaternion{}, false
	}
	quat := movementQuaternion{
		X: math.Float32frombits(binary.LittleEndian.Uint32(buf[offset : offset+4])),
		Y: math.Float32frombits(binary.LittleEndian.Uint32(buf[offset+4 : offset+8])),
		Z: math.Float32frombits(binary.LittleEndian.Uint32(buf[offset+8 : offset+12])),
		W: math.Float32frombits(binary.LittleEndian.Uint32(buf[offset+12 : offset+16])),
	}
	if !movementQuaternionLooksUnit(quat) {
		return movementQuaternion{}, false
	}
	return quat, true
}

func movementQuaternionLooksUnit(quat movementQuaternion) bool {
	components := []float64{float64(quat.X), float64(quat.Y), float64(quat.Z), float64(quat.W)}
	normSquared := 0.0
	for _, component := range components {
		if math.IsNaN(component) || math.IsInf(component, 0) || absFloat64(component) > 1.01 {
			return false
		}
		normSquared += component * component
	}
	norm := math.Sqrt(normSquared)
	return norm >= 0.99 && norm <= 1.01
}

func movementQuaternionToEuler(quat movementQuaternion) Vector3 {
	x := float64(quat.X)
	y := float64(quat.Y)
	z := float64(quat.Z)
	w := float64(quat.W)
	sinrCosp := 2 * (w*x + y*z)
	cosrCosp := 1 - 2*(x*x+y*y)
	roll := math.Atan2(sinrCosp, cosrCosp)
	sinp := 2 * (w*y - z*x)
	pitch := 0.0
	if math.Abs(sinp) >= 1 {
		pitch = math.Copysign(math.Pi/2, sinp)
	} else {
		pitch = math.Asin(sinp)
	}
	sinyCosp := 2 * (w*z + x*y)
	cosyCosp := 1 - 2*(y*y+z*z)
	yaw := math.Atan2(sinyCosp, cosyCosp)
	return Vector3{X: float32(roll), Y: float32(pitch), Z: float32(yaw)}
}

func movementVectorAt(buf []byte, offset int) (Vector3, bool) {
	if offset < 0 || offset+12 > len(buf) {
		return Vector3{}, false
	}
	return Vector3{
		X: math.Float32frombits(binary.LittleEndian.Uint32(buf[offset : offset+4])),
		Y: math.Float32frombits(binary.LittleEndian.Uint32(buf[offset+4 : offset+8])),
		Z: math.Float32frombits(binary.LittleEndian.Uint32(buf[offset+8 : offset+12])),
	}, true
}

func movementY11CandidateIsPosition(candidate movementCandidate) bool {
	if movementY11PropFamily(candidate.prop) != 0 || !candidate.tailZero {
		return false
	}
	_, ok := movementY11CandidatePosition(candidate)
	return ok
}

func movementY11CandidateIsRotation(candidate movementCandidate) bool {
	return movementY11CandidateIsRotationWithModel(candidate, movementDefaultImplicitQuaternionModel)
}

func movementY11CandidateIsRotationWithModel(candidate movementCandidate, model movementImplicitQuaternionModel) bool {
	if movementY11PropFamily(candidate.prop) != 0 || candidate.tailZero {
		return false
	}
	_, ok := movementY11CandidateQuaternionWithModel(candidate, model)
	return ok
}

func movementY11CandidatePosition(candidate movementCandidate) (Vector3, bool) {
	limit := float32(movementPropAbs95Max)
	if movementVectorHasMeaningfulComponent(candidate.primary) && movementVectorComponentsWithin(candidate.primary, limit) {
		return candidate.primary, true
	}
	if candidate.hasSecondary && movementVectorHasMeaningfulComponent(candidate.secondary) && movementVectorComponentsWithin(candidate.secondary, limit) {
		return candidate.secondary, true
	}
	return Vector3{}, false
}

func movementY11CandidateQuaternion(candidate movementCandidate) (movementQuaternion, bool) {
	return movementY11CandidateQuaternionWithModel(candidate, movementDefaultImplicitQuaternionModel)
}

func movementY11CandidateQuaternionWithModel(candidate movementCandidate, model movementImplicitQuaternionModel) (movementQuaternion, bool) {
	if candidate.hasQuaternion {
		return candidate.quaternion, true
	}
	if !candidate.hasSecondary {
		return movementQuaternion{}, false
	}
	if !movementVectorLooksImplicitQuaternionXYZ(candidate.secondary) {
		return movementQuaternion{}, false
	}
	if !movementVectorLooksZeroLike(candidate.primary, 0.05) {
		return movementQuaternion{}, false
	}
	quat := movementImplicitQuaternionFromSecondary(candidate.secondary, model)
	if quat.W == 0 && model.mode == "implicit_w" {
		wSquared := 1 - ((float64(quat.X) * float64(quat.X)) + (float64(quat.Y) * float64(quat.Y)) + (float64(quat.Z) * float64(quat.Z)))
		if wSquared < 0 {
			if wSquared > -0.001 {
				wSquared = 0
			} else {
				return movementQuaternion{}, false
			}
		}
		quat.W = float32(math.Sqrt(wSquared))
	}
	if !movementQuaternionLooksUnit(quat) {
		return movementQuaternion{}, false
	}
	return quat, true
}

func movementImplicitQuaternionFromSecondary(value Vector3, model movementImplicitQuaternionModel) movementQuaternion {
	components := [3]float32{value.X, value.Y, value.Z}
	vector := Vector3{
		X: components[model.order[0]] * model.signs[0],
		Y: components[model.order[1]] * model.signs[1],
		Z: components[model.order[2]] * model.signs[2],
	}
	switch model.mode {
	case "tangent_half":
		normSquared := float64(vector.X*vector.X) + float64(vector.Y*vector.Y) + float64(vector.Z*vector.Z)
		scale := float32(1 / math.Sqrt(1+normSquared))
		return movementQuaternion{
			X: vector.X * scale,
			Y: vector.Y * scale,
			Z: vector.Z * scale,
			W: scale,
		}
	case "axis_angle":
		angle := math.Sqrt(float64(vector.X*vector.X) + float64(vector.Y*vector.Y) + float64(vector.Z*vector.Z))
		if angle == 0 {
			return movementQuaternion{W: 1}
		}
		scale := float32(math.Sin(angle/2) / angle)
		return movementQuaternion{
			X: vector.X * scale,
			Y: vector.Y * scale,
			Z: vector.Z * scale,
			W: float32(math.Cos(angle / 2)),
		}
	default:
		return movementQuaternion{
			X: vector.X,
			Y: vector.Y,
			Z: vector.Z,
		}
	}
}

func movementY11PropFamily(prop []byte) int {
	if len(prop) < 3 {
		return -1
	}
	return int(prop[2])
}

func movementVectorHasMeaningfulComponent(value Vector3) bool {
	return absFloat64(float64(value.X)) >= movementValueAbsMin || absFloat64(float64(value.Y)) >= movementValueAbsMin || absFloat64(float64(value.Z)) >= movementValueAbsMin
}

func movementVectorComponentsWithin(value Vector3, limit float32) bool {
	for _, component := range []float32{value.X, value.Y, value.Z} {
		if absFloat64(float64(component)) > float64(limit) {
			return false
		}
	}
	return true
}

func movementVectorLooksZeroLike(value Vector3, limit float32) bool {
	for _, component := range []float32{value.X, value.Y, value.Z} {
		if absFloat64(float64(component)) > float64(limit) {
			return false
		}
	}
	return true
}

func movementVectorLooksImplicitQuaternionXYZ(value Vector3) bool {
	if !movementVectorHasMeaningfulComponent(value) {
		return false
	}
	if !movementVectorComponentsWithin(value, 1.01) {
		return false
	}
	normSquared := float64(value.X*value.X) + float64(value.Y*value.Y) + float64(value.Z*value.Z)
	return normSquared > 0 && normSquared <= 1.001
}

func countMovementZeroActorPrefixes(codeVersion int, buf []byte, start int) map[string]int {
	counts := map[string]int{}
	for i := start; i < len(buf); i++ {
		candidate, ok := movementCandidateRecordAtCodeVersion(codeVersion, buf, i, nil)
		if !ok || i < 16 || !movementActorKeyIsZero(candidate.actor) {
			continue
		}
		counts[string(buf[i-16:i-8])]++
		if codeVersion < Y11S1Alpha3 {
			i += 37
		}
	}
	return counts
}

func movementActorKeyAt(buf []byte, propOffset int, zeroActorPrefixes map[string]int) []byte {
	actor := append([]byte(nil), buf[propOffset-8:propOffset]...)
	if len(zeroActorPrefixes) == 0 || propOffset < 16 || !movementActorKeyIsZero(actor) {
		return actor
	}
	prefix := buf[propOffset-16 : propOffset-8]
	if zeroActorPrefixes[string(prefix)] < movementZeroActorPrefixThreshold {
		return actor
	}
	return append([]byte(nil), prefix...)
}

func movementActorKeyIsZero(actor []byte) bool {
	for _, b := range actor {
		if b != 0 {
			return false
		}
	}
	return true
}
func movementTimeAt(codeVersion int, buf []byte, offset int) (string, float64, bool) {
	if codeVersion >= Y8S1 {
		if !movementMatchAt(buf, offset, movementTimePattern) || offset+9 > len(buf) {
			return "", 0, false
		}
		seconds := binary.LittleEndian.Uint32(buf[offset+5 : offset+9])
		raw := fmt.Sprintf("%d:%02d", seconds/60, seconds%60)
		return raw, float64(seconds), true
	}
	if !movementMatchAt(buf, offset, movementY7TimePattern) || offset+5 > len(buf) {
		return "", 0, false
	}
	size := int(buf[offset+4])
	if offset+5+size > len(buf) {
		return "", 0, false
	}
	raw := string(buf[offset+5 : offset+5+size])
	if raw == "" {
		return "", 0, false
	}
	parts := strings.Split(raw, ":")
	if len(parts) == 1 {
		seconds, err := strconv.ParseFloat(parts[0], 64)
		if err != nil {
			return "", 0, false
		}
		return raw, seconds, true
	}
	minutes, err := strconv.Atoi(parts[0])
	if err != nil {
		return "", 0, false
	}
	seconds, err := strconv.Atoi(parts[1])
	if err != nil {
		return "", 0, false
	}
	return raw, float64((minutes * 60) + seconds), true
}

func movementMatchAt(buf []byte, offset int, pattern []byte) bool {
	if offset < 0 || offset+len(pattern) > len(buf) {
		return false
	}
	for i := range pattern {
		if buf[offset+i] != pattern[i] {
			return false
		}
	}
	return true
}

func enrichMovementTrack(track MovementTrack) MovementTrack {
	if len(track.Samples) == 0 {
		return track
	}
	track.FirstOffset = track.Samples[0].Offset
	track.LastOffset = track.Samples[len(track.Samples)-1].Offset
	var prevPosition *Vector3
	var bounds *MovementBounds
	for _, sample := range track.Samples {
		if track.FirstPosition == nil && sample.Position != nil {
			pos := *sample.Position
			track.FirstPosition = &pos
		}
		if track.FirstRotation == nil && sample.Rotation != nil {
			rot := *sample.Rotation
			track.FirstRotation = &rot
		}
		if sample.Position != nil {
			pos := *sample.Position
			track.LastPosition = &pos
			if bounds == nil {
				bounds = &MovementBounds{Min: pos, Max: pos}
			} else {
				bounds.Min.X = minFloat32(bounds.Min.X, pos.X)
				bounds.Min.Y = minFloat32(bounds.Min.Y, pos.Y)
				bounds.Min.Z = minFloat32(bounds.Min.Z, pos.Z)
				bounds.Max.X = maxFloat32(bounds.Max.X, pos.X)
				bounds.Max.Y = maxFloat32(bounds.Max.Y, pos.Y)
				bounds.Max.Z = maxFloat32(bounds.Max.Z, pos.Z)
			}
			if prevPosition != nil {
				track.Distance += vectorDistance(*prevPosition, pos)
			}
			prev := pos
			prevPosition = &prev
		}
		if sample.Rotation != nil {
			rot := *sample.Rotation
			track.LastRotation = &rot
		}
	}
	track.Bounds = bounds
	return track
}

func mergeSoloRotationTracks(header Header, tracks []MovementTrack) []MovementTrack {
	if len(header.Players) != 1 || len(tracks) < 2 {
		return tracks
	}
	baseIndex := -1
	for i, track := range tracks {
		if track.PositionSamples == 0 {
			continue
		}
		if baseIndex == -1 || track.PositionSamples > tracks[baseIndex].PositionSamples || (track.PositionSamples == tracks[baseIndex].PositionSamples && track.RotationSamples > tracks[baseIndex].RotationSamples) {
			baseIndex = i
		}
	}
	if baseIndex == -1 {
		return tracks
	}
	base := tracks[baseIndex]
	merged := false
	remaining := make([]MovementTrack, 0, len(tracks))
	for i, track := range tracks {
		if i == baseIndex {
			continue
		}
		if track.PositionSamples != 0 || track.RotationSamples == 0 {
			remaining = append(remaining, track)
			continue
		}
		base.RotationSamples += track.RotationSamples
		base.Samples = append(base.Samples, track.Samples...)
		merged = true
	}
	if !merged {
		return tracks
	}
	sort.Slice(base.Samples, func(i, j int) bool {
		if base.Samples[i].Offset == base.Samples[j].Offset {
			return base.Samples[i].Changed < base.Samples[j].Changed
		}
		return base.Samples[i].Offset < base.Samples[j].Offset
	})
	base.Samples = movementCarryForwardSamples(base.Samples)
	base = enrichMovementTrack(base)
	remaining = append(remaining, base)
	return remaining
}

func movementCarryForwardSamples(samples []MovementSample) []MovementSample {
	carried := make([]MovementSample, len(samples))
	var position *Vector3
	var rotation *Vector3
	for i, sample := range samples {
		carried[i] = sample
		if sample.Position != nil {
			pos := *sample.Position
			position = &pos
		}
		if sample.Rotation != nil {
			rot := *sample.Rotation
			rotation = &rot
		}
		if position != nil {
			pos := *position
			carried[i].Position = &pos
		}
		if rotation != nil {
			rot := *rotation
			carried[i].Rotation = &rot
			rotDegrees := rotationDegreesVector(rot)
			carried[i].RotationDegrees = &rotDegrees
		} else {
			carried[i].Rotation = nil
			carried[i].RotationDegrees = nil
		}
	}
	return carried
}

func selectPrimaryMovementTracks(header Header, tracks []MovementTrack) []MovementTrack {
	if len(tracks) == 0 {
		return nil
	}
	target := len(header.Players)
	if target == 0 {
		return nil
	}
	candidateIndexes := make([]int, 0, len(tracks))
	for i, track := range tracks {
		if movementTrackEffectivePositionSamples(track) >= movementPrimaryTrackMinPositions {
			candidateIndexes = append(candidateIndexes, i)
		}
	}
	if len(candidateIndexes) < target {
		candidateIndexes = candidateIndexes[:0]
		for i, track := range tracks {
			if movementTrackEffectivePositionSamples(track) > 0 {
				candidateIndexes = append(candidateIndexes, i)
			}
		}
	}
	if len(candidateIndexes) == 0 {
		return nil
	}
	if len(candidateIndexes) > target {
		candidateIndexes = candidateIndexes[:target]
	}
	primary := make([]MovementTrack, 0, len(candidateIndexes))
	for _, index := range candidateIndexes {
		tracks[index].LikelyPlayer = true
		primary = append(primary, tracks[index])
	}
	return primary
}

func movementTrackEffectivePositionSamples(track MovementTrack) int {
	if track.PositionSamples >= movementStaticTrackSampleThreshold && track.Distance <= movementStaticTrackDistanceMax {
		return 0
	}
	return track.PositionSamples
}

func movementTrackSortScore(track MovementTrack) int {
	return movementTrackEffectivePositionSamples(track) + track.RotationSamples
}

func movementTrackLooksOriginLike(track MovementTrack) bool {
	if track.FirstPosition != nil && vectorNearOrigin(*track.FirstPosition, 2) {
		return true
	}
	if track.LastPosition != nil && vectorNearOrigin(*track.LastPosition, 2) {
		return true
	}
	return false
}

func vectorNearOrigin(value Vector3, limit float32) bool {
	return movementVectorComponentsWithin(value, limit)
}

func annotatePrimaryMovementTracks(header Header, feedback []MatchUpdate, primary []MovementTrack) []MovementTrack {
	if len(primary) == 0 {
		return nil
	}
	for i := range primary {
		primary[i].Label = fmt.Sprintf("P%02d", i+1)
	}
	if len(primary) == 1 && len(header.Players) == 1 {
		assignMovementPlayerGuess(&primary[0], header.Players[0], "single-player")
		return primary
	}
	teamSizes := movementTeamSizes(header)
	if len(teamSizes) != 2 || teamSizes[0]+teamSizes[1] != len(primary) {
		return primary
	}
	groupA, groupB, ok := splitMovementTrackStarts(primary, teamSizes[0], teamSizes[1])
	if !ok {
		return primary
	}
	if teamSizes[0] != teamSizes[1] {
		altA, altB, altOK := splitMovementTrackStarts(primary, teamSizes[1], teamSizes[0])
		if altOK && movementGroupCompactness(primary, altA)+movementGroupCompactness(primary, altB) < movementGroupCompactness(primary, groupA)+movementGroupCompactness(primary, groupB) {
			groupA = altA
			groupB = altB
		}
	}
	assignMovementGroup(primary, 1, groupA)
	assignMovementGroup(primary, 2, groupB)
	attackIndex, attackOK := movementTeamIndexByRole(header, Attack)
	defenseIndex, defenseOK := movementTeamIndexByRole(header, Defense)
	if !attackOK || !defenseOK {
		return primary
	}
	compactA := movementGroupCompactness(primary, groupA)
	compactB := movementGroupCompactness(primary, groupB)
	defenseGroup := groupA
	attackGroup := groupB
	if compactB < compactA {
		defenseGroup = groupB
		attackGroup = groupA
	}
	assignMovementTeamGuess(primary, defenseGroup, defenseIndex, Defense)
	assignMovementTeamGuess(primary, attackGroup, attackIndex, Attack)
	guessMovementTrackPlayers(header, feedback, primary)
	return primary
}

func splitMovementTrackStarts(tracks []MovementTrack, sizeA int, sizeB int) ([]int, []int, bool) {
	if sizeA <= 0 || sizeB <= 0 || sizeA+sizeB != len(tracks) {
		return nil, nil, false
	}
	seedA, seedB, ok := movementFarthestSeedPair(tracks)
	if !ok {
		return nil, nil, false
	}
	type splitDistance struct {
		Index int
		Diff  float64
	}
	distances := make([]splitDistance, 0, len(tracks))
	for i := range tracks {
		distanceA := distanceToTrackStart(tracks[i], tracks[seedA])
		distanceB := distanceToTrackStart(tracks[i], tracks[seedB])
		if math.IsInf(distanceA, 1) || math.IsInf(distanceB, 1) {
			return nil, nil, false
		}
		distances = append(distances, splitDistance{Index: i, Diff: distanceA - distanceB})
	}
	sort.Slice(distances, func(i, j int) bool {
		if distances[i].Diff == distances[j].Diff {
			return distances[i].Index < distances[j].Index
		}
		return distances[i].Diff < distances[j].Diff
	})
	groupA := make([]int, 0, sizeA)
	groupB := make([]int, 0, sizeB)
	for i, item := range distances {
		if i < sizeA {
			groupA = append(groupA, item.Index)
			continue
		}
		groupB = append(groupB, item.Index)
	}
	if len(groupA) != sizeA || len(groupB) != sizeB {
		return nil, nil, false
	}
	return groupA, groupB, true
}

func movementFarthestSeedPair(tracks []MovementTrack) (int, int, bool) {
	bestA := -1
	bestB := -1
	bestDistance := -1.0
	for i := range tracks {
		if tracks[i].FirstPosition == nil {
			continue
		}
		for j := i + 1; j < len(tracks); j++ {
			if tracks[j].FirstPosition == nil {
				continue
			}
			distance := distanceToTrackStart(tracks[i], tracks[j])
			if distance > bestDistance {
				bestDistance = distance
				bestA = i
				bestB = j
			}
		}
	}
	if bestA == -1 || bestB == -1 {
		return 0, 0, false
	}
	return bestA, bestB, true
}

func movementTeamSizes(header Header) []int {
	counts := map[int]int{}
	for _, player := range header.Players {
		counts[player.TeamIndex]++
	}
	keys := make([]int, 0, len(counts))
	for key, count := range counts {
		if count > 0 {
			keys = append(keys, key)
		}
	}
	sort.Ints(keys)
	sizes := make([]int, 0, len(keys))
	for _, key := range keys {
		sizes = append(sizes, counts[key])
	}
	return sizes
}

func movementTeamIndexByRole(header Header, role TeamRole) (int, bool) {
	for i, team := range header.Teams {
		if team.Role == role {
			return i, true
		}
	}
	return 0, false
}

func movementGroupCompactness(tracks []MovementTrack, indexes []int) float64 {
	if len(indexes) == 0 {
		return math.Inf(1)
	}
	centroid, ok := movementGroupCentroid(tracks, indexes)
	if !ok {
		return math.Inf(1)
	}
	total := 0.0
	for _, index := range indexes {
		if tracks[index].FirstPosition == nil {
			continue
		}
		total += vectorDistance(*tracks[index].FirstPosition, centroid)
	}
	return total / float64(len(indexes))
}

func movementGroupCentroid(tracks []MovementTrack, indexes []int) (Vector3, bool) {
	if len(indexes) == 0 {
		return Vector3{}, false
	}
	var x float64
	var y float64
	var z float64
	count := 0.0
	for _, index := range indexes {
		if tracks[index].FirstPosition == nil {
			continue
		}
		pos := tracks[index].FirstPosition
		x += float64(pos.X)
		y += float64(pos.Y)
		z += float64(pos.Z)
		count++
	}
	if count == 0 {
		return Vector3{}, false
	}
	return Vector3{
		X: float32(x / count),
		Y: float32(y / count),
		Z: float32(z / count),
	}, true
}

func assignMovementGroup(tracks []MovementTrack, group int, indexes []int) {
	for _, index := range indexes {
		tracks[index].StartGroup = group
	}
}

func assignMovementTeamGuess(tracks []MovementTrack, indexes []int, teamIndex int, role TeamRole) {
	for _, index := range indexes {
		tracks[index].TeamIndexGuess = intPtr(teamIndex)
		tracks[index].TeamRoleGuess = role
	}
}

func distanceToTrackStart(left MovementTrack, right MovementTrack) float64 {
	if left.FirstPosition == nil || right.FirstPosition == nil {
		return math.Inf(1)
	}
	return vectorDistance(*left.FirstPosition, *right.FirstPosition)
}

func vectorDistance(left Vector3, right Vector3) float64 {
	dx := float64(left.X - right.X)
	dy := float64(left.Y - right.Y)
	dz := float64(left.Z - right.Z)
	return math.Sqrt((dx * dx) + (dy * dy) + (dz * dz))
}

func minFloat32(left float32, right float32) float32 {
	if left < right {
		return left
	}
	return right
}

func maxFloat32(left float32, right float32) float32 {
	if left > right {
		return left
	}
	return right
}

func rotationDegreesVector(rotation Vector3) Vector3 {
	return Vector3{
		X: radiansToWrappedDegrees(rotation.X),
		Y: radiansToWrappedDegrees(rotation.Y),
		Z: radiansToWrappedDegrees(rotation.Z),
	}
}

func radiansToWrappedDegrees(value float32) float32 {
	wrapped := wrapRadians(float64(value))
	return float32((wrapped * 180) / math.Pi)
}

func wrapRadians(value float64) float64 {
	fullTurn := math.Pi * 2
	wrapped := math.Mod(value, fullTurn)
	if wrapped <= -math.Pi {
		wrapped += fullTurn
	}
	if wrapped > math.Pi {
		wrapped -= fullTurn
	}
	return wrapped
}

func movementTrackPositionJumpCount(track MovementTrack) int {
	positions := movementTrackChangedPositions(track)
	if len(positions) < 3 {
		return 0
	}
	steps := make([]float64, 0, len(positions)-1)
	for i := 1; i < len(positions); i++ {
		step := vectorDistance(positions[i-1], positions[i])
		if step > 0.001 {
			steps = append(steps, step)
		}
	}
	return movementJumpCount(steps, 1.5, 8, 25)
}

func movementTrackRotationJumpCount(track MovementTrack) int {
	rotations := movementTrackChangedRotations(track)
	if len(rotations) < 3 {
		return 0
	}
	axes := [][]float64{
		movementUnwrappedRadians(rotations, func(value Vector3) float64 { return float64(value.X) }),
		movementUnwrappedRadians(rotations, func(value Vector3) float64 { return float64(value.Y) }),
		movementUnwrappedRadians(rotations, func(value Vector3) float64 { return float64(value.Z) }),
	}
	steps := make([]float64, 0, len(rotations)-1)
	for i := 1; i < len(rotations); i++ {
		step := maxFloat64(
			absFloat64(axes[0][i]-axes[0][i-1]),
			absFloat64(axes[1][i]-axes[1][i-1]),
			absFloat64(axes[2][i]-axes[2][i-1]),
		)
		if step > 0.0001 {
			steps = append(steps, step)
		}
	}
	return movementJumpCount(steps, 0.2, 8, 12)
}

func movementTrackRotationSpanDegrees(track MovementTrack) float64 {
	rotations := movementTrackChangedRotations(track)
	if len(rotations) < 2 {
		return 0
	}
	spanRadians := 0.0
	for _, values := range [][]float64{
		movementUnwrappedRadians(rotations, func(value Vector3) float64 { return float64(value.X) }),
		movementUnwrappedRadians(rotations, func(value Vector3) float64 { return float64(value.Y) }),
		movementUnwrappedRadians(rotations, func(value Vector3) float64 { return float64(value.Z) }),
	} {
		minValue := values[0]
		maxValue := values[0]
		for _, value := range values[1:] {
			if value < minValue {
				minValue = value
			}
			if value > maxValue {
				maxValue = value
			}
		}
		span := maxValue - minValue
		if span > spanRadians {
			spanRadians = span
		}
	}
	return spanRadians * 180 / math.Pi
}

func movementTrackChangedPositions(track MovementTrack) []Vector3 {
	positions := make([]Vector3, 0, track.PositionSamples)
	for _, sample := range track.Samples {
		if sample.Changed != "position" || sample.Position == nil {
			continue
		}
		positions = append(positions, *sample.Position)
	}
	return positions
}

func movementTrackChangedRotations(track MovementTrack) []Vector3 {
	rotations := make([]Vector3, 0, track.RotationSamples)
	for _, sample := range track.Samples {
		if sample.Changed != "rotation" || sample.Rotation == nil {
			continue
		}
		rotations = append(rotations, *sample.Rotation)
	}
	return rotations
}

func movementUnwrappedRadians(values []Vector3, axis func(Vector3) float64) []float64 {
	if len(values) == 0 {
		return nil
	}
	out := make([]float64, len(values))
	out[0] = axis(values[0])
	for i := 1; i < len(values); i++ {
		current := axis(values[i])
		prev := out[i-1]
		for current-prev > math.Pi {
			current -= math.Pi * 2
		}
		for prev-current > math.Pi {
			current += math.Pi * 2
		}
		out[i] = current
	}
	return out
}

func movementJumpCount(steps []float64, floor float64, multiplier float64, fallback float64) int {
	if len(steps) < 3 {
		return 0
	}
	sorted := append([]float64(nil), steps...)
	sort.Float64s(sorted)
	baseline := sorted[int(float64(len(sorted)-1)*0.90)]
	if baseline < floor {
		baseline = floor
	}
	threshold := baseline * multiplier
	if threshold < fallback {
		threshold = fallback
	}
	jumps := 0
	for _, step := range steps {
		if step > threshold {
			jumps++
		}
	}
	return jumps
}

func maxFloat64(values ...float64) float64 {
	if len(values) == 0 {
		return 0
	}
	best := values[0]
	for _, value := range values[1:] {
		if value > best {
			best = value
		}
	}
	return best
}

func guessMovementTrackPlayers(header Header, feedback []MatchUpdate, tracks []MovementTrack) {
	if len(tracks) == 0 || len(feedback) == 0 {
		return
	}
	roleTracks := movementTrackIndexesByRole(tracks)
	usedTracks := map[int]bool{}
	usedPlayers := map[string]bool{}
	kills := movementKillFeed(feedback)
	for _, role := range []TeamRole{Attack, Defense} {
		indexes := append([]int(nil), roleTracks[role]...)
		sort.Slice(indexes, func(i, j int) bool {
			if tracks[indexes[i]].LastOffset == tracks[indexes[j]].LastOffset {
				return tracks[indexes[i]].ActorID < tracks[indexes[j]].ActorID
			}
			return tracks[indexes[i]].LastOffset < tracks[indexes[j]].LastOffset
		})
		deaths := movementDeathsByRole(header, kills, role)
		for i := 0; i < len(deaths) && i < len(indexes); i++ {
			assignMovementPlayerGuess(&tracks[indexes[i]], deaths[i], "death-order")
			usedTracks[indexes[i]] = true
			usedPlayers[deaths[i].Username] = true
		}
	}
	for _, kill := range kills {
		targetTrackIndex, targetMapped := movementTrackIndexByPlayerName(tracks, kill.Target)
		if !targetMapped {
			continue
		}
		killer, ok := movementPlayerByUsername(header, kill.Username)
		if !ok || usedPlayers[kill.Username] {
			continue
		}
		role := movementPlayerRole(header, killer)
		candidates := make([]int, 0)
		for _, index := range roleTracks[role] {
			if usedTracks[index] {
				continue
			}
			candidates = append(candidates, index)
		}
		if len(candidates) == 0 {
			continue
		}
		killerTrackIndex, ok := movementNearestTrackAtOffset(tracks, candidates, tracks[targetTrackIndex], kill.offset)
		if !ok {
			continue
		}
		assignMovementPlayerGuess(&tracks[killerTrackIndex], killer, "kill-proximity")
		usedTracks[killerTrackIndex] = true
		usedPlayers[killer.Username] = true
	}
	for _, role := range []TeamRole{Attack, Defense} {
		remainingTracks := make([]int, 0)
		for _, index := range roleTracks[role] {
			if !usedTracks[index] {
				remainingTracks = append(remainingTracks, index)
			}
		}
		remainingPlayers := movementUnassignedPlayersByRole(header, role, usedPlayers)
		if len(remainingTracks) == 1 && len(remainingPlayers) == 1 {
			assignMovementPlayerGuess(&tracks[remainingTracks[0]], remainingPlayers[0], "sole-survivor")
			usedTracks[remainingTracks[0]] = true
			usedPlayers[remainingPlayers[0].Username] = true
		}
	}
}

func movementKillFeed(feedback []MatchUpdate) []MatchUpdate {
	kills := make([]MatchUpdate, 0)
	for _, update := range feedback {
		if update.Type != Kill || update.Target == "" || update.offset == 0 {
			continue
		}
		kills = append(kills, update)
	}
	sort.Slice(kills, func(i, j int) bool {
		if kills[i].offset == kills[j].offset {
			return kills[i].TimeInSeconds > kills[j].TimeInSeconds
		}
		return kills[i].offset < kills[j].offset
	})
	return kills
}

func movementDeathsByRole(header Header, kills []MatchUpdate, role TeamRole) []Player {
	players := make([]Player, 0)
	seen := map[string]bool{}
	for _, kill := range kills {
		player, ok := movementPlayerByUsername(header, kill.Target)
		if !ok || seen[player.Username] || movementPlayerRole(header, player) != role {
			continue
		}
		players = append(players, player)
		seen[player.Username] = true
	}
	return players
}

func movementTrackIndexesByRole(tracks []MovementTrack) map[TeamRole][]int {
	result := map[TeamRole][]int{}
	for i, track := range tracks {
		if track.TeamRoleGuess == "" {
			continue
		}
		result[track.TeamRoleGuess] = append(result[track.TeamRoleGuess], i)
	}
	return result
}

func movementPlayerByUsername(header Header, username string) (Player, bool) {
	for _, player := range header.Players {
		if player.Username == username {
			return player, true
		}
	}
	return Player{}, false
}

func movementPlayerRole(header Header, player Player) TeamRole {
	if player.TeamIndex < 0 || player.TeamIndex >= len(header.Teams) {
		return ""
	}
	return header.Teams[player.TeamIndex].Role
}

func movementTrackIndexByPlayerName(tracks []MovementTrack, username string) (int, bool) {
	for i, track := range tracks {
		if track.PlayerNameGuess == username {
			return i, true
		}
	}
	return 0, false
}

func movementTrackAliveAtOffset(track MovementTrack, offset int) bool {
	return track.FirstOffset <= offset && track.LastOffset >= offset
}

func movementNearestTrackAtOffset(tracks []MovementTrack, indexes []int, target MovementTrack, offset int) (int, bool) {
	targetPosition := movementTrackPositionAtOffset(target, offset)
	if targetPosition == nil {
		return 0, false
	}
	bestIndex := -1
	bestDistance := math.Inf(1)
	for _, index := range indexes {
		position := movementTrackPositionAtOffset(tracks[index], offset)
		if position == nil {
			continue
		}
		distance := vectorDistance(*position, *targetPosition)
		if distance < bestDistance {
			bestDistance = distance
			bestIndex = index
		}
	}
	if bestIndex == -1 {
		return 0, false
	}
	return bestIndex, true
}

func movementTrackPositionAtOffset(track MovementTrack, offset int) *Vector3 {
	var latest *Vector3
	for _, sample := range track.Samples {
		if sample.Offset > offset {
			break
		}
		if sample.Position != nil {
			pos := *sample.Position
			latest = &pos
		}
	}
	return latest
}

func movementUnassignedPlayersByRole(header Header, role TeamRole, used map[string]bool) []Player {
	players := make([]Player, 0)
	for _, player := range header.Players {
		if used[player.Username] || movementPlayerRole(header, player) != role {
			continue
		}
		players = append(players, player)
	}
	return players
}

func assignMovementPlayerGuess(track *MovementTrack, player Player, source string) {
	track.PlayerNameGuess = player.Username
	track.PlayerGuessSource = source
	track.OperatorGuess = player.Operator
	track.SpawnGuess = player.Spawn
}
func intPtr(value int) *int {
	v := value
	return &v
}
