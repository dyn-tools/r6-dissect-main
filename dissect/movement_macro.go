package dissect

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type MovementMacroSearch struct {
	MacroPath       string                           `json:"macroPath,omitempty"`
	BestYaw         *MovementMacroTimelineCandidate  `json:"bestYaw,omitempty"`
	BestPitch       *MovementMacroTimelineCandidate  `json:"bestPitch,omitempty"`
	YawCandidates   []MovementMacroTimelineCandidate `json:"yawCandidates,omitempty"`
	PitchCandidates []MovementMacroTimelineCandidate `json:"pitchCandidates,omitempty"`
}

type MovementMacroTimelineCandidate struct {
	Source                  string  `json:"source,omitempty"`
	PropID                  string  `json:"propID,omitempty"`
	ActorID                 string  `json:"actorID,omitempty"`
	VectorOffset            int     `json:"vectorOffset,omitempty"`
	AxisA                   string  `json:"axisA,omitempty"`
	AxisB                   string  `json:"axisB,omitempty"`
	Alignment               string  `json:"alignment,omitempty"`
	TimeShiftMilliseconds   int     `json:"timeShiftMilliseconds,omitempty"`
	ByteShift               int     `json:"byteShift,omitempty"`
	Transform               string  `json:"transform,omitempty"`
	GlobalShiftMilliseconds int     `json:"globalShiftMilliseconds,omitempty"`
	WindowMatches           int     `json:"windowMatches,omitempty"`
	CoveragePercent         float64 `json:"coveragePercent,omitempty"`
	Score                   float64 `json:"score"`
}

type movementMacroSearchResult struct {
	summary              *MovementMacroSearch
	timelineByActor      map[string]map[int]float64
	pitchTimelineByActor map[string]map[int]float64
}

type movementMacroFile struct {
	Meta     movementMacroMeta      `json:"meta"`
	Commands []movementMacroCommand `json:"commands"`
}

type movementMacroMeta struct {
	YawCalibration movementMacroYawCalibration `json:"yawCalibration"`
}

type movementMacroYawCalibration struct {
	Full360Dots       float64 `json:"full360Dots"`
	Full360DotsSigned float64 `json:"full360DotsSigned"`
}

type movementMacroCommand struct {
	Label    string      `json:"label"`
	Type     string      `json:"type"`
	At       interface{} `json:"at"`
	Duration interface{} `json:"duration"`
	DX       float64     `json:"dx"`
	DY       float64     `json:"dy"`
	Key      string      `json:"key"`
	Keys     []string    `json:"keys"`
}

type movementMacroWindow struct {
	label              string
	kind               string
	startMs            int
	endMs              int
	dx                 float64
	dy                 float64
	expectedYawDegrees float64
}

type movementMacroInterval struct {
	startMs int
	endMs   int
}

type movementMacroWindowStat struct {
	window movementMacroWindow
	mean   float64
	span   float64
	slope  float64
}

type movementMacroTimedSample struct {
	index     int
	elapsedMs int
}

type movementMacroCandidateEval struct {
	candidate MovementMacroTimelineCandidate
	timeline  map[int]float64
}

type movementMacroRankedStream struct {
	stream    movementDirectionStream
	priority  int
	sampleCnt int
}

func movementMacroSearchWithSidecar(options MovementOptions, header Header, buf []byte, start int, zeroActorPrefixes map[string]int, primary []MovementTrack) movementMacroSearchResult {
	if len(primary) != 1 {
		return movementMacroSearchResult{}
	}
	macroPath, windows, ok := movementLoadMacroSidecar(options.SourcePath)
	if !ok {
		return movementMacroSearchResult{}
	}
	base := primary[0]
	if len(base.Samples) == 0 {
		return movementMacroSearchResult{}
	}
	hasYawWindows := movementMacroHasAxisWindows(windows, "yaw")
	hasPitchWindows := movementMacroHasAxisWindows(windows, "pitch")
	clockEntries, _, _, _, _ := movementRecoveredClockEntries(buf)
	allowedProps := movementDirectionPropAllowlist(header, buf, start, zeroActorPrefixes, base.ActorID)
	streams := movementDirectionTargetedLateOffsetStreams(header, buf, start, zeroActorPrefixes, allowedProps, clockEntries)
	streams = append(streams, movementDirectionQuaternionStreams(header, buf, start, zeroActorPrefixes, allowedProps, clockEntries)...)
	streams = append(streams, movementDirectionQuaternionPitchStreams(header, buf, start, zeroActorPrefixes, allowedProps, clockEntries)...)
	streams = append(streams, movementDirectionTargetedAlignedWordDeltaStreams(header, buf, start, zeroActorPrefixes, allowedProps, clockEntries)...)
	streams = append(streams, movementDirectionTargetedScalarPairStreams(streams, allowedProps, movementDirectionMinSampleCount(header))...)
	streams = append(streams, movementDirectionStreams(header, buf, start, zeroActorPrefixes, allowedProps, clockEntries)...)
	streams = append(streams, movementDirectionTargetVectorStreams(header, buf, start, zeroActorPrefixes, allowedProps, clockEntries, base)...)
	streams = movementMacroUniqueDirectionStreams(streams)
	var bestYaw *MovementMacroTimelineCandidate
	var yawTimeline map[int]float64
	var yawCandidates []MovementMacroTimelineCandidate
	if hasYawWindows {
		yawStreams := movementMacroShortlistStreams(streams, "yaw")
		bestYaw, yawTimeline, yawCandidates = movementMacroBestTimeline(base, windows, yawStreams, "yaw")
	}
	if bestYaw != nil && len(yawTimeline) > 0 {
		yawTimeline = movementMacroShiftTimelineByMilliseconds(base, yawTimeline, bestYaw.GlobalShiftMilliseconds)
		if lockedYaw, ok := movementMacroLockYawTimeline(base, windows, yawTimeline); ok {
			originalScore, _ := movementMacroTimelineScore(base, windows, yawTimeline, "yaw", 0)
			lockedScore, _ := movementMacroTimelineScore(base, windows, lockedYaw, "yaw", 0)
			if lockedScore >= originalScore {
				yawTimeline = lockedYaw
				bestYaw.Transform += "+macro-locked"
			}
		}
		if normalizedYaw, ok := movementMacroAffineNormalizeYawTimeline(base, windows, yawTimeline, 0); ok {
			originalScore, _ := movementMacroTimelineScore(base, windows, yawTimeline, "yaw", 0)
			normalizedScore, _ := movementMacroTimelineScore(base, windows, normalizedYaw, "yaw", 0)
			if normalizedScore > originalScore {
				yawTimeline = normalizedYaw
				bestYaw.Transform += "+macro-affine"
			}
		}
	}
	pitchSourceStreams := append([]movementDirectionStream(nil), streams...)
	if bestYaw != nil {
		pitchSourceStreams = append(pitchSourceStreams, movementMacroTriadPitchStreamsForProp(streams, bestYaw.PropID, movementDirectionMinSampleCount(header))...)
	}
	var bestPitch *MovementMacroTimelineCandidate
	var pitchTimeline map[int]float64
	var pitchCandidates []MovementMacroTimelineCandidate
	if hasPitchWindows {
		pitchStreams := movementMacroShortlistStreams(pitchSourceStreams, "pitch")
		bestPitch, pitchTimeline, pitchCandidates = movementMacroBestTimeline(base, windows, pitchStreams, "pitch")
	}
	if bestYaw == nil && bestPitch == nil {
		return movementMacroSearchResult{}
	}
	if bestPitch != nil && len(pitchTimeline) > 0 {
		pitchTimeline = movementMacroShiftTimelineByMilliseconds(base, pitchTimeline, bestPitch.GlobalShiftMilliseconds)
		pitchTimeline = movementMacroStabilizePitchTimeline(base, windows, pitchTimeline)
	}
	if len(yawTimeline) > 0 && len(pitchTimeline) > 0 {
		yawTimeline, pitchTimeline = movementMacroJointRefineYawPitch(base, windows, yawTimeline, pitchTimeline)
	}
	if len(yawTimeline) > 0 {
		yawTimeline = movementMacroStabilizeYawTimeline(base, windows, yawTimeline)
	}
	if len(pitchTimeline) > 0 {
		pitchTimeline = movementMacroStabilizePitchTimeline(base, windows, pitchTimeline)
	}
	result := movementMacroSearchResult{
		summary: &MovementMacroSearch{
			MacroPath:       macroPath,
			BestYaw:         bestYaw,
			BestPitch:       bestPitch,
			YawCandidates:   yawCandidates,
			PitchCandidates: pitchCandidates,
		},
		timelineByActor:      map[string]map[int]float64{},
		pitchTimelineByActor: map[string]map[int]float64{},
	}
	if bestYaw != nil && len(yawTimeline) > 0 {
		result.timelineByActor[base.ActorID] = yawTimeline
	}
	if bestPitch != nil && len(pitchTimeline) > 0 {
		result.pitchTimelineByActor[base.ActorID] = pitchTimeline
	}
	return result
}

func movementMacroUniqueDirectionStreams(streams []movementDirectionStream) []movementDirectionStream {
	if len(streams) <= 1 {
		return streams
	}
	seen := map[string]bool{}
	unique := make([]movementDirectionStream, 0, len(streams))
	for _, stream := range streams {
		key := strings.Join([]string{
			stream.source,
			normalizeMovementPropID(stream.propID),
			strings.TrimSpace(stream.actorID),
			strconv.Itoa(stream.vectorOffset),
			strings.TrimSpace(stream.axisA),
			strings.TrimSpace(stream.axisB),
		}, "|")
		if seen[key] {
			continue
		}
		seen[key] = true
		unique = append(unique, stream)
	}
	return unique
}

func movementMacroHasAxisWindows(windows []movementMacroWindow, axis string) bool {
	for _, window := range windows {
		switch axis {
		case "yaw":
			if movementMacroKindMatches(window.kind, "yaw") || window.kind == "combined" || window.kind == "move+combined" {
				return true
			}
		case "pitch":
			if movementMacroKindMatches(window.kind, "pitch") || window.kind == "combined" || window.kind == "move+combined" {
				return true
			}
		}
	}
	return false
}

func movementMergeMacroSearch(direction movementDirectionSearchResult, macro movementMacroSearchResult, primary []MovementTrack) movementDirectionSearchResult {
	if macro.summary == nil || len(primary) != 1 {
		return direction
	}
	base := primary[0]
	if direction.timelineByActor == nil {
		direction.timelineByActor = map[string]map[int]float64{}
	}
	if direction.derivedByActor == nil {
		direction.derivedByActor = map[string]map[int]float64{}
	}
	if direction.pitchTimelineByActor == nil {
		direction.pitchTimelineByActor = map[string]map[int]float64{}
	}
	if macroTimeline, ok := macro.timelineByActor[base.ActorID]; ok {
		direction.timelineByActor[base.ActorID] = macroTimeline
		delete(direction.derivedByActor, base.ActorID)
	} else {
		delete(direction.timelineByActor, base.ActorID)
		delete(direction.derivedByActor, base.ActorID)
	}
	if macroPitch, ok := macro.pitchTimelineByActor[base.ActorID]; ok {
		direction.pitchTimelineByActor[base.ActorID] = macroPitch
	} else {
		delete(direction.pitchTimelineByActor, base.ActorID)
	}
	return direction
}

func movementLoadMacroSidecar(sourcePath string) (string, []movementMacroWindow, bool) {
	sourcePath = strings.TrimSpace(sourcePath)
	if sourcePath == "" {
		return "", nil, false
	}
	ext := strings.ToLower(filepath.Ext(sourcePath))
	if ext != ".rec" {
		return "", nil, false
	}
	macroPath := strings.TrimSuffix(sourcePath, ext) + ".txt"
	data, err := os.ReadFile(macroPath)
	if err != nil {
		return "", nil, false
	}
	var parsed movementMacroFile
	if err := json.Unmarshal(data, &parsed); err != nil {
		return "", nil, false
	}
	windows := movementNormalizeMacroWindowsWithCalibration(parsed.Commands, movementMacroYawCalibrationDots(parsed.Meta))
	if len(windows) == 0 {
		return "", nil, false
	}
	return macroPath, windows, true
}

func movementNormalizeMacroWindows(commands []movementMacroCommand) []movementMacroWindow {
	return movementNormalizeMacroWindowsWithCalibration(commands, 0)
}

func movementNormalizeMacroWindowsWithCalibration(commands []movementMacroCommand, yawFull360Dots float64) []movementMacroWindow {
	windows := make([]movementMacroWindow, 0, len(commands))
	movementIntervals := make([]movementMacroInterval, 0, len(commands))
	mouseIntervals := make([]movementMacroInterval, 0, len(commands))
	for _, command := range commands {
		startMs, ok := movementParseMacroMilliseconds(command.At)
		if !ok {
			continue
		}
		durationMs, ok := movementParseMacroMilliseconds(command.Duration)
		if !ok {
			durationMs = 0
		}
		keys := append([]string(nil), command.Keys...)
		if strings.TrimSpace(command.Key) != "" {
			keys = append(keys, command.Key)
		}
		if !movementMacroHasMovementKeys(keys) {
			continue
		}
		movementIntervals = append(movementIntervals, movementMacroInterval{
			startMs: startMs,
			endMs:   startMs + durationMs,
		})
	}
	for index, command := range commands {
		startMs, ok := movementParseMacroMilliseconds(command.At)
		if !ok {
			continue
		}
		durationMs, ok := movementParseMacroMilliseconds(command.Duration)
		if !ok {
			durationMs = 0
		}
		keys := append([]string(nil), command.Keys...)
		if strings.TrimSpace(command.Key) != "" {
			keys = append(keys, command.Key)
		}
		kind := movementMacroWindowKind(command.Type, command.DX, command.DY, movementMacroWindowOverlapsMovement(startMs, startMs+durationMs, movementIntervals), keys)
		if kind == "" {
			continue
		}
		label := strings.TrimSpace(command.Label)
		if label == "" {
			label = "command-" + strconv.Itoa(index+1)
		}
		windows = append(windows, movementMacroWindow{
			label:              label,
			kind:               kind,
			startMs:            startMs,
			endMs:              startMs + durationMs,
			dx:                 command.DX,
			dy:                 command.DY,
			expectedYawDegrees: movementMacroExpectedYawDegrees(kind, command.DX, yawFull360Dots),
		})
		if command.Type == "mousemove" {
			mouseIntervals = append(mouseIntervals, movementMacroInterval{
				startMs: startMs,
				endMs:   startMs + durationMs,
			})
		}
	}
	moveWindows := movementMacroMoveOnlyWindows(movementIntervals, mouseIntervals)
	windows = append(windows, moveWindows...)
	sort.Slice(windows, func(i, j int) bool {
		if windows[i].startMs == windows[j].startMs {
			return windows[i].endMs < windows[j].endMs
		}
		return windows[i].startMs < windows[j].startMs
	})
	merged := make([]movementMacroInterval, 0, len(windows))
	for _, window := range windows {
		if len(merged) == 0 || window.startMs > merged[len(merged)-1].endMs {
			merged = append(merged, movementMacroInterval{
				startMs: window.startMs,
				endMs:   window.endMs,
			})
			continue
		}
		if window.endMs > merged[len(merged)-1].endMs {
			merged[len(merged)-1].endMs = window.endMs
		}
	}
	idle := make([]movementMacroWindow, 0, len(windows))
	for index := 0; index+1 < len(merged); index++ {
		gapStart := merged[index].endMs
		gapEnd := merged[index+1].startMs
		if gapEnd-gapStart < 300 {
			continue
		}
		idle = append(idle, movementMacroWindow{
			label:   "idle-" + strconv.Itoa(index+1),
			kind:    "idle",
			startMs: gapStart,
			endMs:   gapEnd,
		})
	}
	return append(windows, idle...)
}

func movementParseMacroMilliseconds(value interface{}) (int, bool) {
	switch typed := value.(type) {
	case float64:
		return int(math.Round(typed)), true
	case string:
		trimmed := strings.TrimSpace(typed)
		if strings.HasSuffix(trimmed, "ms") {
			trimmed = strings.TrimSuffix(trimmed, "ms")
		}
		parsed, err := strconv.ParseFloat(trimmed, 64)
		if err != nil {
			return 0, false
		}
		return int(math.Round(parsed)), true
	default:
		return 0, false
	}
}

func movementMacroYawCalibrationDots(meta movementMacroMeta) float64 {
	if meta.YawCalibration.Full360Dots > 0 {
		return meta.YawCalibration.Full360Dots
	}
	if meta.YawCalibration.Full360DotsSigned != 0 {
		return math.Abs(meta.YawCalibration.Full360DotsSigned)
	}
	return 0
}

func movementMacroWindowKind(commandType string, dx float64, dy float64, movementActive bool, keys []string) string {
	if commandType == "mousemove" {
		switch {
		case dx != 0 && dy == 0:
			if movementActive {
				return "move+yaw"
			}
			return "yaw"
		case dy != 0 && dx == 0:
			if movementActive {
				return "move+pitch"
			}
			return "pitch"
		case dx != 0 && dy != 0:
			if movementActive {
				return "move+combined"
			}
			return "combined"
		}
	}
	return ""
}

func movementMacroExpectedYawDegrees(kind string, dx float64, yawFull360Dots float64) float64 {
	if yawFull360Dots <= 0 {
		return 0
	}
	if kind != "yaw" && kind != "move+yaw" {
		return 0
	}
	if dx == 0 {
		return 0
	}
	return (dx / yawFull360Dots) * 360
}

func movementMacroHasMovementKeys(keys []string) bool {
	for _, key := range keys {
		switch strings.ToLower(strings.TrimSpace(key)) {
		case "w", "a", "s", "d", "shift", "ctrl", "space":
			return true
		}
	}
	return false
}

func movementMacroWindowOverlapsMovement(startMs int, endMs int, intervals []movementMacroInterval) bool {
	for _, interval := range intervals {
		if interval.endMs < startMs || interval.startMs > endMs {
			continue
		}
		return true
	}
	return false
}

func movementMacroMoveOnlyWindows(movementIntervals []movementMacroInterval, mouseIntervals []movementMacroInterval) []movementMacroWindow {
	windows := make([]movementMacroWindow, 0, len(movementIntervals))
	counter := 0
	for _, movement := range movementIntervals {
		segments := []movementMacroInterval{movement}
		for _, mouse := range mouseIntervals {
			next := make([]movementMacroInterval, 0, len(segments))
			for _, segment := range segments {
				next = append(next, movementMacroSubtractInterval(segment, mouse)...)
			}
			segments = next
			if len(segments) == 0 {
				break
			}
		}
		for _, segment := range segments {
			if segment.endMs-segment.startMs < 150 {
				continue
			}
			counter++
			windows = append(windows, movementMacroWindow{
				label:   "move-" + strconv.Itoa(counter),
				kind:    "move",
				startMs: segment.startMs,
				endMs:   segment.endMs,
			})
		}
	}
	return windows
}

func movementMacroSubtractInterval(base movementMacroInterval, cut movementMacroInterval) []movementMacroInterval {
	if cut.endMs <= base.startMs || cut.startMs >= base.endMs {
		return []movementMacroInterval{base}
	}
	out := make([]movementMacroInterval, 0, 2)
	if cut.startMs > base.startMs {
		out = append(out, movementMacroInterval{
			startMs: base.startMs,
			endMs:   minInt(cut.startMs, base.endMs),
		})
	}
	if cut.endMs < base.endMs {
		out = append(out, movementMacroInterval{
			startMs: maxInt(cut.endMs, base.startMs),
			endMs:   base.endMs,
		})
	}
	return out
}

func movementMacroKindMatches(kind string, base string) bool {
	if kind == base {
		return true
	}
	return strings.HasSuffix(kind, "+"+base)
}

func movementMacroBestTimeline(track MovementTrack, windows []movementMacroWindow, streams []movementDirectionStream, kind string) (*MovementMacroTimelineCandidate, map[int]float64, []MovementMacroTimelineCandidate) {
	if len(track.Samples) == 0 || len(windows) == 0 {
		return nil, nil, nil
	}
	bestScore := -1e9
	var best MovementMacroTimelineCandidate
	var bestTimeline map[int]float64
	summaries := make([]MovementMacroTimelineCandidate, 0, 64)
	coarseGlobalShifts := []int{-2000, -1500, -1000, -500, 0, 500, 1000, 1500, 2000}
	for _, stream := range streams {
		if !movementMacroStreamAllowed(stream) {
			continue
		}
		alignments := movementMacroAlignmentsForStream(stream)
		transforms := movementMacroTransforms(kind)
		for _, alignment := range alignments {
			for _, transform := range transforms {
				timeline, ok := movementDirectionProjectStreamTimelineWithAlignment(track, stream, alignment, transform.fn)
				if !ok {
					continue
				}
				seenShifts := map[int]bool{}
				bestLocalShift := 0
				bestLocalScore := -1e9
				evaluateShift := func(globalShift int) {
					if seenShifts[globalShift] {
						return
					}
					seenShifts[globalShift] = true
					recordCandidate := func(candidateTimeline map[int]float64, transformName string) {
						score, matches := movementMacroTimelineScore(track, windows, candidateTimeline, kind, globalShift)
						if matches == 0 {
							return
						}
						summary := MovementMacroTimelineCandidate{
							Source:                  stream.source,
							PropID:                  stream.propID,
							ActorID:                 stream.actorID,
							VectorOffset:            stream.vectorOffset,
							AxisA:                   stream.axisA,
							AxisB:                   stream.axisB,
							Alignment:               alignment.Alignment,
							TimeShiftMilliseconds:   alignment.TimeShiftMilliseconds,
							ByteShift:               alignment.ByteShift,
							Transform:               transformName,
							GlobalShiftMilliseconds: globalShift,
							WindowMatches:           matches,
							CoveragePercent:         movementPercent(len(candidateTimeline), len(track.Samples)),
							Score:                   score,
						}
						summaries = append(summaries, summary)
						if score > bestLocalScore {
							bestLocalScore = score
							bestLocalShift = globalShift
						}
						if score <= bestScore {
							return
						}
						bestScore = score
						bestTimeline = candidateTimeline
						best = summary
					}
					recordCandidate(timeline, transform.name)
					if kind == "yaw" {
						if normalizedTimeline, ok := movementMacroAffineNormalizeYawTimeline(track, windows, timeline, globalShift); ok {
							recordCandidate(normalizedTimeline, transform.name+"+macro-affine")
						}
					}
				}
				for _, globalShift := range coarseGlobalShifts {
					evaluateShift(globalShift)
				}
				for globalShift := bestLocalShift - 400; globalShift <= bestLocalShift+400; globalShift += 100 {
					evaluateShift(globalShift)
				}
			}
		}
	}
	sort.Slice(summaries, func(i, j int) bool {
		if summaries[i].Score == summaries[j].Score {
			if summaries[i].CoveragePercent == summaries[j].CoveragePercent {
				if summaries[i].Source == summaries[j].Source {
					return summaries[i].VectorOffset < summaries[j].VectorOffset
				}
				return summaries[i].Source < summaries[j].Source
			}
			return summaries[i].CoveragePercent > summaries[j].CoveragePercent
		}
		return summaries[i].Score > summaries[j].Score
	})
	if len(summaries) > 5 {
		summaries = summaries[:5]
	}
	minScore := 0.5
	if kind == "yaw" {
		minScore = -25
	}
	if bestScore < minScore || len(bestTimeline) == 0 {
		return nil, nil, summaries
	}
	return &best, bestTimeline, summaries
}

func movementMacroStreamAllowed(stream movementDirectionStream) bool {
	return strings.Contains(stream.source, "late-") ||
		strings.Contains(stream.source, "quat") ||
		strings.Contains(stream.source, "word-") ||
		strings.Contains(stream.source, "pair") ||
		strings.Contains(stream.source, "vector") ||
		strings.Contains(stream.source, "target-pos") ||
		strings.Contains(stream.source, "triad")
}

func movementMacroShortlistStreams(streams []movementDirectionStream, kind string) []movementDirectionStream {
	rankedStreams := make([]movementMacroRankedStream, 0, len(streams))
	seen := map[string]bool{}
	for _, stream := range streams {
		if !movementMacroStreamAllowed(stream) {
			continue
		}
		key := strings.Join([]string{
			stream.source,
			stream.propID,
			stream.actorID,
			strconv.Itoa(stream.vectorOffset),
			stream.axisA,
			stream.axisB,
		}, "|")
		if seen[key] {
			continue
		}
		seen[key] = true
		priority := len(stream.samples)
		priority += movementMacroStreamPriority(stream, kind)
		rankedStreams = append(rankedStreams, movementMacroRankedStream{
			stream:    stream,
			priority:  priority,
			sampleCnt: len(stream.samples),
		})
	}
	sort.Slice(rankedStreams, func(i, j int) bool {
		if rankedStreams[i].priority == rankedStreams[j].priority {
			return rankedStreams[i].sampleCnt > rankedStreams[j].sampleCnt
		}
		return rankedStreams[i].priority > rankedStreams[j].priority
	})
	limit := 256
	if kind == "pitch" {
		limit = 512
	}
	if kind == "yaw" {
		rankedStreams = movementMacroBalancedYawShortlist(rankedStreams, limit)
	} else if kind == "pitch" {
		rankedStreams = movementMacroBalancedPitchShortlist(rankedStreams, limit)
	} else if len(rankedStreams) > limit {
		rankedStreams = rankedStreams[:limit]
	}
	out := make([]movementDirectionStream, 0, len(rankedStreams))
	for _, item := range rankedStreams {
		out = append(out, item.stream)
	}
	return out
}

func movementMacroBalancedPitchShortlist(rankedStreams []movementMacroRankedStream, limit int) []movementMacroRankedStream {
	if len(rankedStreams) <= limit {
		return rankedStreams
	}
	caps := map[string]int{
		"quat":        96,
		"late-scalar": 128,
		"pair":        96,
		"word":        48,
		"other":       48,
	}
	counts := map[string]int{}
	selected := make([]movementMacroRankedStream, 0, limit)
	overflow := make([]movementMacroRankedStream, 0, len(rankedStreams))
	for _, item := range rankedStreams {
		family := movementMacroPitchStreamFamily(item.stream)
		if counts[family] < caps[family] && len(selected) < limit {
			selected = append(selected, item)
			counts[family]++
			continue
		}
		overflow = append(overflow, item)
	}
	for _, item := range overflow {
		if len(selected) >= limit {
			break
		}
		selected = append(selected, item)
	}
	return selected
}

func movementMacroBalancedYawShortlist(rankedStreams []movementMacroRankedStream, limit int) []movementMacroRankedStream {
	if len(rankedStreams) <= limit {
		return rankedStreams
	}
	caps := map[string]int{
		"late_pair_single": 48,
		"late_pair_fused":  32,
		"late_scalar":      28,
		"quat":             48,
		"word":             20,
		"vector":           24,
		"target":           20,
		"other":            16,
	}
	counts := map[string]int{}
	selected := make([]movementMacroRankedStream, 0, limit)
	overflow := make([]movementMacroRankedStream, 0, len(rankedStreams))
	for _, item := range rankedStreams {
		family := movementMacroYawStreamFamily(item.stream)
		if counts[family] < caps[family] && len(selected) < limit {
			selected = append(selected, item)
			counts[family]++
			continue
		}
		overflow = append(overflow, item)
	}
	for _, item := range overflow {
		if len(selected) >= limit {
			break
		}
		selected = append(selected, item)
	}
	return selected
}

func movementMacroPitchStreamFamily(stream movementDirectionStream) string {
	switch {
	case strings.Contains(stream.source, "quat"):
		return "quat"
	case strings.Contains(stream.source, "triad"):
		return "pair"
	case strings.Contains(stream.source, "pair"):
		return "pair"
	case strings.Contains(stream.source, "word"):
		return "word"
	case strings.Contains(stream.source, "late-s16") || strings.Contains(stream.source, "late-u16") || strings.Contains(stream.source, "late-f32"):
		return "late-scalar"
	default:
		return "other"
	}
}

func movementMacroYawStreamFamily(stream movementDirectionStream) string {
	switch {
	case strings.Contains(stream.source, "scalar-pair-double"):
		if strings.Contains(stream.source, "actor-fused") || strings.Contains(stream.actorID, "fused") {
			return "late_pair_fused"
		}
		return "late_pair_single"
	case strings.Contains(stream.source, "late-s16") || strings.Contains(stream.source, "late-u16") || strings.Contains(stream.source, "late-f32"):
		return "late_scalar"
	case strings.Contains(stream.source, "quat"):
		return "quat"
	case strings.Contains(stream.source, "word"):
		return "word"
	case strings.Contains(stream.source, "target-pos"):
		return "target"
	case strings.Contains(stream.source, "vector"):
		return "vector"
	default:
		return "other"
	}
}

func movementMacroStreamPriority(stream movementDirectionStream, kind string) int {
	priority := 0
	if strings.Contains(stream.source, "late-") {
		priority += 100000
	}
	if strings.Contains(stream.source, "quat") {
		priority += 50000
	}
	if strings.Contains(stream.source, "integrated") {
		priority += 10000
	}
	if kind == "yaw" {
		if strings.Contains(stream.source, "scalar-pair-double") {
			priority += 70000
		}
		if strings.Contains(stream.source, "late-s16") {
			priority += 60000
		}
		if strings.Contains(stream.source, "actor-fused") {
			priority += 30000
		}
		if strings.Contains(stream.source, "f32") {
			priority -= 40000
		}
		if strings.Contains(stream.source, "vector") {
			priority += 25000
		}
		if strings.Contains(stream.source, "target-pos") {
			priority += 30000
		}
		return priority
	}
	if kind != "pitch" {
		return priority
	}
	if strings.Contains(stream.source, "pitch") {
		priority += 120000
	}
	if strings.Contains(stream.source, "quat") {
		priority += 90000
	}
	if strings.Contains(stream.source, "pair") {
		priority += 60000
	}
	if strings.Contains(stream.source, "triad") {
		priority += 80000
	}
	if strings.Contains(stream.source, "actor-fused") {
		priority += 30000
	}
	if strings.Contains(stream.source, "prop-fused") {
		priority += 20000
	}
	if strings.Contains(stream.source, "delta-integrated") {
		priority += 15000
	}
	if strings.Contains(stream.source, "late-f32") {
		priority += 20000
	}
	return priority
}

type movementMacroScalarTriadShared struct {
	offsets       []int
	sampleIndexes []int
	milliseconds  []int
	hasTime       []bool
	a             []float64
	b             []float64
	c             []float64
}

func movementMacroTriadPitchStreamsForProp(streams []movementDirectionStream, propID string, minSamples int) []movementDirectionStream {
	normalizedProp := normalizeMovementPropID(propID)
	if normalizedProp == "" {
		return nil
	}
	type groupKey struct {
		sourceBase string
		propID     string
		actorID    string
	}
	grouped := map[groupKey][]movementDirectionStream{}
	for _, stream := range streams {
		if len(stream.samples) < minSamples || !movementDirectionSourceCanScalarPair(stream.source) {
			continue
		}
		streamProp := normalizeMovementPropID(stream.propID)
		if streamProp == "" || streamProp != normalizedProp {
			continue
		}
		key := groupKey{
			sourceBase: movementDirectionScalarSourceBase(stream.source),
			propID:     streamProp,
			actorID:    stream.actorID,
		}
		grouped[key] = append(grouped[key], stream)
	}
	out := make([]movementDirectionStream, 0, 64)
	for key, fragments := range grouped {
		sort.Slice(fragments, func(i, j int) bool {
			if len(fragments[i].samples) == len(fragments[j].samples) {
				return fragments[i].vectorOffset < fragments[j].vectorOffset
			}
			return len(fragments[i].samples) > len(fragments[j].samples)
		})
		if len(fragments) > 10 {
			fragments = fragments[:10]
		}
		sort.Slice(fragments, func(i, j int) bool {
			return fragments[i].vectorOffset < fragments[j].vectorOffset
		})
		for i := 0; i < len(fragments); i++ {
			for j := i + 1; j < len(fragments); j++ {
				for k := j + 1; k < len(fragments); k++ {
					group := []movementDirectionStream{fragments[i], fragments[j], fragments[k]}
					for zIndex := 0; zIndex < 3; zIndex++ {
						hLeft := group[(zIndex+1)%3]
						hRight := group[(zIndex+2)%3]
						zStream := group[zIndex]
						shared := movementMacroScalarTriadSamples(hLeft.samples, hRight.samples, zStream.samples, minSamples)
						if len(shared.offsets) < minSamples {
							continue
						}
						for _, signZ := range []float64{1, -1} {
							samples := make([]movementDirectionAngleSample, 0, len(shared.offsets))
							for index := range shared.offsets {
								horizontal := math.Hypot(shared.a[index], shared.b[index])
								if horizontal < 0.001 {
									continue
								}
								angle := math.Atan2(signZ*shared.c[index], horizontal) * 180 / math.Pi
								samples = append(samples, movementDirectionAngleSample{
									offset:       shared.offsets[index],
									sampleIndex:  shared.sampleIndexes[index],
									milliseconds: shared.milliseconds[index],
									hasTime:      shared.hasTime[index],
									angle:        angle,
								})
							}
							if len(samples) < minSamples {
								continue
							}
							out = append(out, movementDirectionStream{
								source:       key.sourceBase + "-scalar-triad-pitch",
								propID:       key.propID,
								actorID:      key.actorID,
								vectorOffset: zStream.vectorOffset,
								axisA:        "h(" + strconv.Itoa(hLeft.vectorOffset) + "," + strconv.Itoa(hRight.vectorOffset) + ")",
								axisB:        signedAxisName("o"+strconv.Itoa(zStream.vectorOffset), signZ),
								samples:      samples,
							})
						}
					}
				}
			}
		}
	}
	return movementFuseActorSplitDirectionStreams(out, minSamples)
}

func movementMacroScalarTriadSamples(left []movementDirectionAngleSample, right []movementDirectionAngleSample, vertical []movementDirectionAngleSample, minSamples int) movementMacroScalarTriadShared {
	if len(left) == 0 || len(right) == 0 || len(vertical) == 0 {
		return movementMacroScalarTriadShared{}
	}
	baseType := 0
	base := left
	if len(right) < len(base) {
		base = right
		baseType = 1
	}
	if len(vertical) < len(base) {
		base = vertical
		baseType = 2
	}
	out := movementMacroScalarTriadShared{
		offsets:       make([]int, 0, minInt(len(base), minInt(len(left), minInt(len(right), len(vertical))))),
		sampleIndexes: make([]int, 0, minInt(len(base), minInt(len(left), minInt(len(right), len(vertical))))),
		milliseconds:  make([]int, 0, minInt(len(base), minInt(len(left), minInt(len(right), len(vertical))))),
		hasTime:       make([]bool, 0, minInt(len(base), minInt(len(left), minInt(len(right), len(vertical))))),
		a:             make([]float64, 0, minInt(len(base), minInt(len(left), minInt(len(right), len(vertical))))),
		b:             make([]float64, 0, minInt(len(base), minInt(len(left), minInt(len(right), len(vertical))))),
		c:             make([]float64, 0, minInt(len(base), minInt(len(left), minInt(len(right), len(vertical))))),
	}
	for _, sample := range base {
		var a, b, c float64
		switch baseType {
		case 0:
			otherB, ok := movementInterpolatedAngleSampleAtOffset(right, sample.offset, 32768)
			if !ok {
				continue
			}
			otherC, ok := movementInterpolatedAngleSampleAtOffset(vertical, sample.offset, 32768)
			if !ok {
				continue
			}
			a = sample.angle
			b = otherB.angle
			c = otherC.angle
		case 1:
			otherA, ok := movementInterpolatedAngleSampleAtOffset(left, sample.offset, 32768)
			if !ok {
				continue
			}
			otherC, ok := movementInterpolatedAngleSampleAtOffset(vertical, sample.offset, 32768)
			if !ok {
				continue
			}
			a = otherA.angle
			b = sample.angle
			c = otherC.angle
		default:
			otherA, ok := movementInterpolatedAngleSampleAtOffset(left, sample.offset, 32768)
			if !ok {
				continue
			}
			otherB, ok := movementInterpolatedAngleSampleAtOffset(right, sample.offset, 32768)
			if !ok {
				continue
			}
			a = otherA.angle
			b = otherB.angle
			c = sample.angle
		}
		out.offsets = append(out.offsets, sample.offset)
		out.sampleIndexes = append(out.sampleIndexes, sample.sampleIndex)
		out.milliseconds = append(out.milliseconds, sample.milliseconds)
		out.hasTime = append(out.hasTime, sample.hasTime)
		out.a = append(out.a, a)
		out.b = append(out.b, b)
		out.c = append(out.c, c)
	}
	if len(out.offsets) < minSamples {
		return movementMacroScalarTriadShared{}
	}
	return out
}

func movementMacroAlignmentsForStream(stream movementDirectionStream) []MovementDirectionCandidate {
	alignments := []MovementDirectionCandidate{}
	add := func(candidate MovementDirectionCandidate) {
		for _, existing := range alignments {
			if existing.Alignment == candidate.Alignment &&
				existing.ByteShift == candidate.ByteShift &&
				existing.TimeShiftMilliseconds == candidate.TimeShiftMilliseconds {
				return
			}
		}
		alignments = append(alignments, candidate)
	}
	if movementDirectionTimedSampleCount(stream.samples) >= 8 {
		for _, shift := range []int{-70, -35, 0, 35, 70} {
			add(MovementDirectionCandidate{
				Alignment:             "time",
				TimeShiftMilliseconds: shift,
			})
		}
	}
	for _, shift := range []int{-32, 0, 32} {
		add(MovementDirectionCandidate{
			Alignment: "progress",
			ByteShift: shift,
		})
	}
	for _, shift := range []int{0} {
		add(MovementDirectionCandidate{
			Alignment: "offset",
			ByteShift: shift,
		})
	}
	return alignments
}

type movementMacroTransform struct {
	name string
	fn   func(float64) float64
}

func movementMacroTransforms(kind string) []movementMacroTransform {
	if kind == "pitch" {
		return []movementMacroTransform{
			{name: "fold", fn: func(angle float64) float64 { return movementFoldPitchDegrees(angle) }},
			{name: "fold-neg", fn: func(angle float64) float64 { return movementFoldPitchDegrees(-angle) }},
			{name: "raw", fn: func(angle float64) float64 { return movementWrapDegrees(angle) }},
			{name: "raw-neg", fn: func(angle float64) float64 { return movementWrapDegrees(-angle) }},
			{name: "fold+90", fn: func(angle float64) float64 { return movementFoldPitchDegrees(movementWrapDegrees(angle + 90)) }},
			{name: "fold-90", fn: func(angle float64) float64 { return movementFoldPitchDegrees(movementWrapDegrees(angle - 90)) }},
		}
	}
	return []movementMacroTransform{
		{name: "raw", fn: func(angle float64) float64 { return movementWrapDegrees(angle) }},
		{name: "neg", fn: func(angle float64) float64 { return movementWrapDegrees(-angle) }},
	}
}

func movementMacroTimelineScore(track MovementTrack, windows []movementMacroWindow, timeline map[int]float64, kind string, globalShift int) (float64, int) {
	if len(track.Samples) == 0 || len(timeline) == 0 {
		return -1, 0
	}
	score := movementPercent(len(timeline), len(track.Samples)) / 100
	matches := 0
	yawLeakSpan := 0.0
	pitchSignalSpan := 0.0
	yawStats := make([]movementMacroWindowStat, 0, 16)
	pitchStats := make([]movementMacroWindowStat, 0, 16)
	moveStats := make([]movementMacroWindowStat, 0, 16)
	idleStats := make([]movementMacroWindowStat, 0, 16)
	for _, window := range windows {
		values := movementMacroWindowValues(track, timeline, window, globalShift)
		if len(values) < 3 {
			continue
		}
		windowSpan := movementHeadingSequenceSpan(values)
		slope := values[len(values)-1] - values[0]
		mean := movementMacroMean(values)
		minValue, maxValue := movementMacroMinMax(values)
		stat := movementMacroWindowStat{
			window: window,
			mean:   mean,
			span:   windowSpan,
			slope:  slope,
		}
		switch window.kind {
		case "idle":
			idleStats = append(idleStats, stat)
			score -= math.Min(windowSpan, 120) / 35
		case "move":
			moveStats = append(moveStats, stat)
			score -= math.Min(windowSpan, 120) / 20
		case "yaw":
			yawStats = append(yawStats, stat)
			if kind == "yaw" {
				matches++
				pitchSignalSpan += windowSpan
				score += math.Min(windowSpan, 180) / 18
				if signFloat64(slope) == -signFloat64(window.dx) {
					score += 2
				} else if signFloat64(slope) != 0 {
					score -= 2
				}
				if windowSpan < 5 {
					score -= 3
				}
				score += movementMacroExpectedYawDeltaScore(window, slope)
				score += movementMacroExpectedYawSpanScore(window, windowSpan)
			} else {
				yawLeakSpan += windowSpan
				score -= math.Min(windowSpan, 120) / 8
			}
		case "move+yaw":
			yawStats = append(yawStats, stat)
			if kind == "yaw" {
				matches++
				pitchSignalSpan += windowSpan
				score += math.Min(windowSpan, 180) / 20
				if signFloat64(slope) == -signFloat64(window.dx) {
					score += 2
				} else if signFloat64(slope) != 0 {
					score -= 2
				}
				if windowSpan < 5 {
					score -= 3
				}
				score += movementMacroExpectedYawDeltaScore(window, slope)
				score += movementMacroExpectedYawSpanScore(window, windowSpan)
			} else {
				yawLeakSpan += windowSpan
				score -= math.Min(windowSpan, 120) / 8
			}
		case "pitch":
			pitchStats = append(pitchStats, stat)
			if kind == "pitch" {
				matches++
				pitchSignalSpan += windowSpan
				if minValue < -95 || maxValue > 95 {
					score -= 12
				}
				if windowSpan > 120 {
					score -= 10
				}
				score += math.Min(windowSpan, 120) / 14
				if signFloat64(slope) == -signFloat64(window.dy) {
					score += 2
				} else if signFloat64(slope) != 0 {
					score -= 2
				}
				if windowSpan < 3 {
					score -= 3
				}
			} else {
				yawLeakSpan += windowSpan
				score -= math.Min(windowSpan, 120) / 8
			}
		case "move+pitch":
			pitchStats = append(pitchStats, stat)
			if kind == "pitch" {
				matches++
				pitchSignalSpan += windowSpan
				if minValue < -95 || maxValue > 95 {
					score -= 12
				}
				if windowSpan > 120 {
					score -= 10
				}
				score += math.Min(windowSpan, 120) / 16
				if signFloat64(slope) == -signFloat64(window.dy) {
					score += 2
				} else if signFloat64(slope) != 0 {
					score -= 2
				}
				if windowSpan < 3 {
					score -= 3
				}
			} else {
				yawLeakSpan += windowSpan
				score -= math.Min(windowSpan, 120) / 8
			}
		case "combined":
			if kind == "yaw" && window.dx != 0 {
				score += math.Min(windowSpan, 180) / 40
			}
			if kind == "pitch" && window.dy != 0 {
				score += math.Min(windowSpan, 120) / 40
			}
		case "move+combined":
			if kind == "yaw" && window.dx != 0 {
				score += math.Min(windowSpan, 180) / 48
			}
			if kind == "pitch" && window.dy != 0 {
				score += math.Min(windowSpan, 120) / 48
			}
		}
	}
	if kind == "pitch" && pitchSignalSpan > 0 && yawLeakSpan > 0 {
		score -= yawLeakSpan / math.Max(pitchSignalSpan, 1)
	}
	if kind == "yaw" && pitchSignalSpan > 0 && yawLeakSpan > 0 {
		score -= yawLeakSpan / math.Max(pitchSignalSpan, 1)
	}
	if kind == "yaw" {
		score += movementMacroSequenceScore(yawStats, func(window movementMacroWindow) float64 {
			return -signFloat64(window.dx)
		})
		score += movementMacroDistinctLevelScore(yawStats, 4, 4)
		score += movementMacroCommandSequenceFitScore(yawStats, func(window movementMacroWindow) float64 {
			return -window.dx
		})
		score += movementMacroSpanConsistencyScore(yawStats, 160)
		score -= movementMacroAverageSpan(pitchStats) / 3
		score -= movementMacroLeakScore(pitchStats)
		score -= movementMacroSuppressionPenalty(moveStats, 12)
		score -= movementMacroSuppressionPenalty(idleStats, 16)
		score += movementMacroSignalSeparationScore(yawStats, pitchStats)
	} else {
		score += movementMacroSequenceScore(pitchStats, func(window movementMacroWindow) float64 {
			return -signFloat64(window.dy)
		})
		score += movementMacroDistinctLevelScore(pitchStats, 3, 5)
		score += movementMacroCommandSequenceFitScore(pitchStats, func(window movementMacroWindow) float64 {
			return -window.dy
		})
		score += movementMacroSpanConsistencyScore(pitchStats, 110)
		score -= movementMacroAverageSpan(yawStats) / 3
		score -= movementMacroLeakScore(yawStats)
		score -= movementMacroSuppressionPenalty(moveStats, 10)
		score -= movementMacroSuppressionPenalty(idleStats, 14)
		score += movementMacroSignalSeparationScore(pitchStats, yawStats)
	}
	return score, matches
}

func movementMacroWindowValues(track MovementTrack, timeline map[int]float64, window movementMacroWindow, globalShift int) []float64 {
	indexes := movementMacroWindowSampleIndexes(track, window, globalShift)
	if len(indexes) == 0 {
		return nil
	}
	values := make([]float64, 0, len(indexes))
	for _, sampleIndex := range indexes {
		value, ok := timeline[sampleIndex]
		if !ok {
			continue
		}
		values = append(values, value)
	}
	return movementMacroUnwrapValues(values)
}

func movementMacroWindowSampleIndexes(track MovementTrack, window movementMacroWindow, globalShift int) []int {
	startMs := window.startMs + globalShift
	endMs := window.endMs + globalShift
	if endMs < startMs {
		return nil
	}
	replayStart, ok := movementMacroReplayStartMilliseconds(track)
	if !ok {
		return nil
	}
	indexes := make([]int, 0, 16)
	for sampleIndex, sample := range track.Samples {
		if sample.TimeInSeconds == nil {
			continue
		}
		elapsedMs := replayStart - int(math.Round(*sample.TimeInSeconds*1000))
		if elapsedMs < startMs || elapsedMs > endMs {
			continue
		}
		indexes = append(indexes, sampleIndex)
	}
	return indexes
}

func movementMacroReplayStartMilliseconds(track MovementTrack) (int, bool) {
	for _, sample := range track.Samples {
		if sample.TimeInSeconds == nil {
			continue
		}
		return int(math.Round(*sample.TimeInSeconds * 1000)), true
	}
	return 0, false
}

func movementMacroShiftTimelineByMilliseconds(track MovementTrack, timeline map[int]float64, globalShift int) map[int]float64 {
	if len(timeline) == 0 || globalShift == 0 {
		return timeline
	}
	replayStart, ok := movementMacroReplayStartMilliseconds(track)
	if !ok {
		return timeline
	}
	timed := make([]movementMacroTimedSample, 0, len(track.Samples))
	for index, sample := range track.Samples {
		if sample.TimeInSeconds == nil {
			continue
		}
		timed = append(timed, movementMacroTimedSample{
			index:     index,
			elapsedMs: replayStart - int(math.Round(*sample.TimeInSeconds*1000)),
		})
	}
	if len(timed) == 0 {
		return timeline
	}
	out := make(map[int]float64, len(timeline))
	for sampleIndex, value := range timeline {
		if sampleIndex < 0 || sampleIndex >= len(track.Samples) {
			continue
		}
		sample := track.Samples[sampleIndex]
		if sample.TimeInSeconds == nil {
			out[sampleIndex] = value
			continue
		}
		elapsed := replayStart - int(math.Round(*sample.TimeInSeconds*1000))
		targetElapsed := elapsed - globalShift
		targetIndex, ok := movementMacroNearestSampleIndexAtElapsed(timed, targetElapsed)
		if !ok {
			out[sampleIndex] = value
			continue
		}
		out[targetIndex] = value
	}
	return out
}

func movementMacroNearestSampleIndexAtElapsed(samples []movementMacroTimedSample, targetElapsed int) (int, bool) {
	if len(samples) == 0 {
		return 0, false
	}
	index := sort.Search(len(samples), func(i int) bool {
		return samples[i].elapsedMs >= targetElapsed
	})
	if index <= 0 {
		return samples[0].index, true
	}
	if index >= len(samples) {
		return samples[len(samples)-1].index, true
	}
	left := samples[index-1]
	right := samples[index]
	if targetElapsed-left.elapsedMs <= right.elapsedMs-targetElapsed {
		return left.index, true
	}
	return right.index, true
}

func movementMacroUnwrapValues(values []float64) []float64 {
	if len(values) == 0 {
		return nil
	}
	out := make([]float64, len(values))
	out[0] = values[0]
	for index := 1; index < len(values); index++ {
		current := values[index]
		delta := movementWrapDegrees(current - out[index-1])
		out[index] = out[index-1] + delta
	}
	return out
}

func movementMacroMean(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sum := 0.0
	for _, value := range values {
		sum += value
	}
	return sum / float64(len(values))
}

func movementMacroMinMax(values []float64) (float64, float64) {
	if len(values) == 0 {
		return 0, 0
	}
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
	return minValue, maxValue
}

func movementMacroSequenceScore(stats []movementMacroWindowStat, expectedSign func(movementMacroWindow) float64) float64 {
	if len(stats) < 2 {
		return 0
	}
	ordered := append([]movementMacroWindowStat(nil), stats...)
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].window.startMs == ordered[j].window.startMs {
			return ordered[i].window.endMs < ordered[j].window.endMs
		}
		return ordered[i].window.startMs < ordered[j].window.startMs
	})
	unwrappedMeans := make([]float64, len(ordered))
	unwrappedMeans[0] = ordered[0].mean
	score := 0.0
	for index := 1; index < len(ordered); index++ {
		unwrappedMeans[index] = unwrappedMeans[index-1] + movementWrapDegrees(ordered[index].mean-unwrappedMeans[index-1])
		delta := unwrappedMeans[index] - unwrappedMeans[index-1]
		expected := expectedSign(ordered[index].window)
		magnitude := math.Abs(delta)
		switch {
		case expected == 0:
		case magnitude < 0.5:
			score -= 4
		case signFloat64(delta) == expected:
			score += 2 + (math.Min(magnitude, 120) / 10)
		default:
			score -= 2 + (math.Min(magnitude, 120) / 12)
		}
		if ordered[index].span <= 3 {
			score += 0.75
		} else if ordered[index].span > 30 {
			score -= 1.5
		}
	}
	return score
}

func movementMacroLeakScore(stats []movementMacroWindowStat) float64 {
	if len(stats) == 0 {
		return 0
	}
	score := 0.0
	for _, stat := range stats {
		score += math.Min(stat.span, 120) / 30
		if math.Abs(stat.slope) > 5 {
			score += 0.5
		}
	}
	return score
}

func movementMacroDistinctLevelScore(stats []movementMacroWindowStat, tolerance float64, minExpected int) float64 {
	if len(stats) == 0 {
		return 0
	}
	ordered := append([]movementMacroWindowStat(nil), stats...)
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].window.startMs == ordered[j].window.startMs {
			return ordered[i].window.endMs < ordered[j].window.endMs
		}
		return ordered[i].window.startMs < ordered[j].window.startMs
	})
	levels := []float64{}
	for _, stat := range ordered {
		value := stat.mean
		matched := false
		for _, existing := range levels {
			if math.Abs(movementWrapDegrees(value-existing)) <= tolerance {
				matched = true
				break
			}
		}
		if !matched {
			levels = append(levels, value)
		}
	}
	if len(levels) < minExpected {
		return -4 * float64(minExpected-len(levels))
	}
	return math.Min(float64(len(levels)-minExpected), 4)
}

func movementMacroSignalSeparationScore(signal []movementMacroWindowStat, leak []movementMacroWindowStat) float64 {
	signalMean := movementMacroAverageSpan(signal)
	leakMean := movementMacroAverageSpan(leak)
	if signalMean <= 0 {
		return -8
	}
	if leakMean <= 0 {
		return math.Min(signalMean/4, 6)
	}
	ratio := signalMean / leakMean
	switch {
	case ratio >= 2:
		return math.Min(ratio, 4)
	case ratio >= 1.25:
		return ratio
	default:
		return -60 * (1.5 - ratio)
	}
}

func movementMacroCommandSequenceFitScore(stats []movementMacroWindowStat, expectedDelta func(movementMacroWindow) float64) float64 {
	if len(stats) < 3 {
		return 0
	}
	ordered := append([]movementMacroWindowStat(nil), stats...)
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].window.startMs == ordered[j].window.startMs {
			return ordered[i].window.endMs < ordered[j].window.endMs
		}
		return ordered[i].window.startMs < ordered[j].window.startMs
	})
	targets := make([]float64, 0, len(ordered))
	values := make([]float64, 0, len(ordered))
	cumulative := 0.0
	for _, stat := range ordered {
		delta := expectedDelta(stat.window)
		if delta == 0 {
			continue
		}
		cumulative += delta
		targets = append(targets, cumulative)
		values = append(values, stat.mean)
	}
	if len(targets) < 3 {
		return 0
	}
	values = movementMacroUnwrapValues(values)
	correlation := movementMacroPearsonCorrelation(targets, values)
	if math.IsNaN(correlation) {
		return -8
	}
	scale, intercept, ok := movementMacroLinearFit(targets, values)
	if !ok || math.Abs(scale) < 0.001 {
		return -8
	}
	meanAbsError := 0.0
	meanTargetStep := 0.0
	for index := range targets {
		fitted := intercept + (scale * targets[index])
		meanAbsError += math.Abs(values[index] - fitted)
		if index == 0 {
			meanTargetStep += math.Abs(targets[index])
		} else {
			meanTargetStep += math.Abs(targets[index] - targets[index-1])
		}
	}
	meanAbsError /= float64(len(targets))
	meanTargetStep /= float64(len(targets))
	if meanTargetStep < 1 {
		meanTargetStep = 1
	}
	errorRatio := meanAbsError / meanTargetStep
	score := correlation * 16
	score -= errorRatio * 8
	if correlation >= 0.9 {
		score += 3
	}
	if errorRatio <= 0.35 {
		score += 3
	}
	return score
}

func movementMacroLinearFit(xs []float64, ys []float64) (float64, float64, bool) {
	if len(xs) != len(ys) || len(xs) < 2 {
		return 0, 0, false
	}
	meanX := movementMacroMean(xs)
	meanY := movementMacroMean(ys)
	varXX := 0.0
	covXY := 0.0
	for index := range xs {
		dx := xs[index] - meanX
		dy := ys[index] - meanY
		varXX += dx * dx
		covXY += dx * dy
	}
	if varXX == 0 {
		return 0, 0, false
	}
	scale := covXY / varXX
	intercept := meanY - (scale * meanX)
	return scale, intercept, true
}

func movementMacroPearsonCorrelation(xs []float64, ys []float64) float64 {
	if len(xs) != len(ys) || len(xs) < 2 {
		return math.NaN()
	}
	meanX := movementMacroMean(xs)
	meanY := movementMacroMean(ys)
	varXX := 0.0
	varYY := 0.0
	covXY := 0.0
	for index := range xs {
		dx := xs[index] - meanX
		dy := ys[index] - meanY
		varXX += dx * dx
		varYY += dy * dy
		covXY += dx * dy
	}
	if varXX == 0 || varYY == 0 {
		return math.NaN()
	}
	return covXY / math.Sqrt(varXX*varYY)
}

func movementMacroAverageSpan(stats []movementMacroWindowStat) float64 {
	if len(stats) == 0 {
		return 0
	}
	total := 0.0
	for _, stat := range stats {
		total += stat.span
	}
	return total / float64(len(stats))
}

func movementMacroSpanConsistencyScore(stats []movementMacroWindowStat, maxHealthySpan float64) float64 {
	if len(stats) < 2 {
		return 0
	}
	spans := make([]float64, 0, len(stats))
	flatPenalty := 0.0
	overshootPenalty := 0.0
	for _, stat := range stats {
		spans = append(spans, stat.span)
		if stat.span < 4 {
			flatPenalty += 2.5
		}
		if stat.span > maxHealthySpan {
			overshootPenalty += math.Min((stat.span-maxHealthySpan)/20, 4)
		}
	}
	meanSpan := movementMacroMean(spans)
	if meanSpan <= 0 {
		return -(flatPenalty + overshootPenalty + 4)
	}
	variance := 0.0
	for _, span := range spans {
		delta := span - meanSpan
		variance += delta * delta
	}
	variance /= float64(len(spans))
	stddev := math.Sqrt(variance)
	score := 4 - math.Min((stddev/math.Max(meanSpan, 5))*5, 8)
	return score - flatPenalty - overshootPenalty
}

func movementMacroSuppressionPenalty(stats []movementMacroWindowStat, divisor float64) float64 {
	if len(stats) == 0 {
		return 0
	}
	penalty := 0.0
	for _, stat := range stats {
		penalty += math.Min(stat.span, 180) / divisor
		if math.Abs(stat.slope) > 5 {
			penalty += 1
		}
	}
	return penalty / float64(len(stats))
}

func movementMacroExpectedYawDeltaScore(window movementMacroWindow, slope float64) float64 {
	if window.expectedYawDegrees == 0 {
		return 0
	}
	expected := math.Abs(window.expectedYawDegrees)
	observed := math.Abs(slope)
	if expected < 1 {
		return 0
	}
	errorDegrees := math.Abs(observed - expected)
	score := 6 - math.Min(errorDegrees/6, 12)
	switch {
	case observed < expected*0.5:
		score -= 6
	case observed > expected*1.75:
		score -= 8
	case observed > expected*1.25:
		score -= 3
	}
	if errorDegrees <= 8 {
		score += 3
	} else if errorDegrees <= 18 {
		score += 1
	}
	return score
}

func movementMacroExpectedYawSpanScore(window movementMacroWindow, span float64) float64 {
	if window.expectedYawDegrees == 0 {
		return 0
	}
	expected := math.Abs(window.expectedYawDegrees)
	observed := math.Abs(span)
	if expected < 1 {
		return 0
	}
	errorDegrees := math.Abs(observed - expected)
	score := 5 - math.Min(errorDegrees/4, 24)
	switch {
	case observed > expected*3:
		score -= 18
	case observed > expected*2.25:
		score -= 12
	case observed > expected*1.5:
		score -= 6
	case observed < expected*0.5:
		score -= 5
	}
	if errorDegrees <= 6 {
		score += 3
	} else if errorDegrees <= 12 {
		score += 1
	}
	return score
}

func movementMacroAffineNormalizeYawTimeline(track MovementTrack, windows []movementMacroWindow, timeline map[int]float64, globalShift int) (map[int]float64, bool) {
	targets := make([]float64, 0, 16)
	values := make([]float64, 0, 16)
	cumulative := 0.0
	orderedWindows := append([]movementMacroWindow(nil), windows...)
	sort.Slice(orderedWindows, func(i, j int) bool {
		if orderedWindows[i].startMs == orderedWindows[j].startMs {
			return orderedWindows[i].endMs < orderedWindows[j].endMs
		}
		return orderedWindows[i].startMs < orderedWindows[j].startMs
	})
	for _, window := range orderedWindows {
		if !movementMacroKindMatches(window.kind, "yaw") || window.expectedYawDegrees == 0 {
			continue
		}
		windowValues := movementMacroWindowValues(track, timeline, window, globalShift)
		if len(windowValues) < 2 {
			continue
		}
		cumulative += -window.expectedYawDegrees
		targets = append(targets, cumulative)
		values = append(values, movementMacroMean(windowValues))
	}
	if len(targets) < 4 {
		return nil, false
	}
	values = movementMacroUnwrapValues(values)
	scale, intercept, ok := movementMacroLinearFit(targets, values)
	if !ok || math.Abs(scale) < 0.05 {
		return nil, false
	}
	orderedSamples := make([]int, 0, len(timeline))
	for sampleIndex := range timeline {
		orderedSamples = append(orderedSamples, sampleIndex)
	}
	sort.Ints(orderedSamples)
	unwrapped := make([]float64, 0, len(orderedSamples))
	for _, sampleIndex := range orderedSamples {
		unwrapped = append(unwrapped, timeline[sampleIndex])
	}
	unwrapped = movementMacroUnwrapValues(unwrapped)
	out := make(map[int]float64, len(timeline))
	for index, sampleIndex := range orderedSamples {
		normalized := (unwrapped[index] - intercept) / scale
		out[sampleIndex] = movementWrapDegrees(normalized)
	}
	return out, true
}

func movementMacroLockYawTimeline(track MovementTrack, windows []movementMacroWindow, timeline map[int]float64) (map[int]float64, bool) {
	if len(track.Samples) == 0 || len(windows) == 0 || len(timeline) == 0 {
		return nil, false
	}
	replayStart, ok := movementMacroReplayStartMilliseconds(track)
	if !ok {
		return nil, false
	}
	type yawSegment struct {
		startMs     int
		endMs       int
		startTarget float64
		endTarget   float64
	}
	orderedWindows := append([]movementMacroWindow(nil), windows...)
	sort.Slice(orderedWindows, func(i, j int) bool {
		if orderedWindows[i].startMs == orderedWindows[j].startMs {
			return orderedWindows[i].endMs < orderedWindows[j].endMs
		}
		return orderedWindows[i].startMs < orderedWindows[j].startMs
	})
	haveExpected := false
	for _, window := range orderedWindows {
		if movementMacroKindMatches(window.kind, "yaw") && window.expectedYawDegrees != 0 {
			haveExpected = true
			break
		}
	}
	if !haveExpected {
		return nil, false
	}
	baseline, ok := movementMacroReferenceLevel(track, orderedWindows, timeline, "idle", "move", "yaw", "move+yaw")
	if !ok {
		baseline = movementMacroTimelineMean(timeline)
	}
	segments := make([]yawSegment, 0, len(orderedWindows))
	currentTarget := baseline
	for _, window := range orderedWindows {
		segment := yawSegment{
			startMs:     window.startMs,
			endMs:       window.endMs,
			startTarget: currentTarget,
			endTarget:   currentTarget,
		}
		if movementMacroKindMatches(window.kind, "yaw") && window.expectedYawDegrees != 0 {
			segment.endTarget = movementWrapDegrees(currentTarget - window.expectedYawDegrees)
			currentTarget = segment.endTarget
		}
		segments = append(segments, segment)
	}
	if len(segments) == 0 {
		return nil, false
	}
	out := make(map[int]float64, len(track.Samples))
	segmentIndex := 0
	lastTarget := segments[0].startTarget
	for sampleIndex, sample := range track.Samples {
		if sample.TimeInSeconds == nil {
			continue
		}
		elapsedMs := replayStart - int(math.Round(*sample.TimeInSeconds*1000))
		for segmentIndex < len(segments) && elapsedMs > segments[segmentIndex].endMs {
			lastTarget = segments[segmentIndex].endTarget
			segmentIndex++
		}
		if segmentIndex >= len(segments) {
			out[sampleIndex] = lastTarget
			continue
		}
		segment := segments[segmentIndex]
		if elapsedMs < segment.startMs {
			out[sampleIndex] = lastTarget
			continue
		}
		if segment.endMs <= segment.startMs || segment.startTarget == segment.endTarget {
			out[sampleIndex] = segment.endTarget
			continue
		}
		progress := float64(elapsedMs-segment.startMs) / float64(segment.endMs-segment.startMs)
		if progress < 0 {
			progress = 0
		} else if progress > 1 {
			progress = 1
		}
		out[sampleIndex] = movementCircularLerpDegrees(segment.startTarget, segment.endTarget, progress)
	}
	for index, segment := range segments {
		if index < 0 || index >= len(orderedWindows) {
			continue
		}
		window := orderedWindows[index]
		indexes := movementMacroWindowSampleIndexes(track, window, 0)
		if len(indexes) == 0 {
			continue
		}
		if movementMacroKindMatches(window.kind, "yaw") && window.expectedYawDegrees != 0 && len(indexes) > 1 {
			progresses, ok := movementMacroWindowProgressFromTimeline(indexes, timeline)
			if !ok {
				denominator := float64(len(indexes) - 1)
				progresses = make([]float64, len(indexes))
				for samplePos := range indexes {
					progresses[samplePos] = float64(samplePos) / denominator
				}
			}
			for samplePos, sampleIndex := range indexes {
				progress := progresses[samplePos]
				out[sampleIndex] = movementCircularLerpDegrees(segment.startTarget, segment.endTarget, progress)
			}
			continue
		}
		for _, sampleIndex := range indexes {
			out[sampleIndex] = segment.endTarget
		}
	}
	return out, len(out) > 0
}

func movementCircularLerpDegrees(start float64, end float64, t float64) float64 {
	delta := movementWrapDegrees(end - start)
	return movementWrapDegrees(start + (delta * t))
}

func movementMacroWindowProgressFromTimeline(indexes []int, timeline map[int]float64) ([]float64, bool) {
	if len(indexes) < 2 {
		return nil, false
	}
	values := make([]float64, 0, len(indexes))
	filteredIndexes := make([]int, 0, len(indexes))
	for _, sampleIndex := range indexes {
		value, ok := timeline[sampleIndex]
		if !ok {
			continue
		}
		values = append(values, value)
		filteredIndexes = append(filteredIndexes, sampleIndex)
	}
	if len(filteredIndexes) < 2 {
		return nil, false
	}
	unwrapped := movementMacroUnwrapValues(values)
	total := unwrapped[len(unwrapped)-1] - unwrapped[0]
	if math.Abs(total) < 1e-6 {
		return nil, false
	}
	progressByIndex := make(map[int]float64, len(filteredIndexes))
	for idx, sampleIndex := range filteredIndexes {
		progress := (unwrapped[idx] - unwrapped[0]) / total
		if progress < 0 {
			progress = 0
		} else if progress > 1 {
			progress = 1
		}
		progressByIndex[sampleIndex] = progress
	}
	progresses := make([]float64, len(indexes))
	last := 0.0
	haveLast := false
	for idx, sampleIndex := range indexes {
		progress, ok := progressByIndex[sampleIndex]
		if !ok {
			if len(indexes) == 1 {
				progress = 1
			} else {
				progress = float64(idx) / float64(len(indexes)-1)
			}
		}
		if haveLast && progress < last {
			progress = last
		}
		progresses[idx] = progress
		last = progress
		haveLast = true
	}
	progresses[len(progresses)-1] = 1
	return progresses, true
}

func movementMacroRefineYawWithPitch(track MovementTrack, windows []movementMacroWindow, yawTimeline map[int]float64, pitchTimeline map[int]float64) map[int]float64 {
	if len(yawTimeline) == 0 || len(pitchTimeline) == 0 {
		return yawTimeline
	}
	pitchBaseline, ok := movementMacroReferenceLevel(track, windows, pitchTimeline, "yaw", "idle")
	if !ok {
		pitchBaseline = movementMacroTimelineMean(pitchTimeline)
	}
	bestTimeline := yawTimeline
	bestScore := movementMacroYawIsolationScore(track, windows, yawTimeline)
	bestAlpha := 0.0
	for alpha := -1.00; alpha <= 1.0001; alpha += 0.05 {
		candidate := movementMacroMixYawWithPitch(yawTimeline, pitchTimeline, pitchBaseline, alpha)
		score := movementMacroYawIsolationScore(track, windows, candidate)
		if score > bestScore {
			bestScore = score
			bestTimeline = candidate
			bestAlpha = alpha
		}
	}
	for alpha := bestAlpha - 0.10; alpha <= bestAlpha+0.1001; alpha += 0.01 {
		candidate := movementMacroMixYawWithPitch(yawTimeline, pitchTimeline, pitchBaseline, alpha)
		score := movementMacroYawIsolationScore(track, windows, candidate)
		if score > bestScore {
			bestScore = score
			bestTimeline = candidate
		}
	}
	return bestTimeline
}

func movementMacroJointRefineYawPitch(track MovementTrack, windows []movementMacroWindow, yawTimeline map[int]float64, pitchTimeline map[int]float64) (map[int]float64, map[int]float64) {
	if len(yawTimeline) == 0 || len(pitchTimeline) == 0 {
		return yawTimeline, pitchTimeline
	}
	yawBaseline, ok := movementMacroReferenceLevel(track, windows, yawTimeline, "idle", "pitch")
	if !ok {
		yawBaseline = movementMacroTimelineMean(yawTimeline)
	}
	pitchBaseline, ok := movementMacroReferenceLevel(track, windows, pitchTimeline, "idle", "yaw")
	if !ok {
		pitchBaseline = movementMacroTimelineMean(pitchTimeline)
	}
	bestYaw := yawTimeline
	bestPitch := pitchTimeline
	baseMetrics := movementMacroJointMetricsForTimelines(track, windows, yawTimeline, pitchTimeline)
	bestScore := movementMacroJointTimelineScore(track, windows, yawTimeline, pitchTimeline, baseMetrics)
	bestAY := 1.0
	bestAP := 0.0
	bestPY := 0.0
	bestPP := 1.0
	for _, ay := range []float64{0.5, 0.75, 1.0, 1.25, 1.5} {
		for _, ap := range []float64{-1.0, -0.75, -0.5, -0.25, 0, 0.25, 0.5, 0.75, 1.0} {
			for _, py := range []float64{-1.0, -0.75, -0.5, -0.25, 0, 0.25, 0.5, 0.75, 1.0} {
				for _, pp := range []float64{0.5, 0.75, 1.0, 1.25, 1.5} {
					candidateYaw, candidatePitch := movementMacroApplyJointTransform(yawTimeline, pitchTimeline, yawBaseline, pitchBaseline, ay, ap, py, pp)
					score := movementMacroJointTimelineScore(track, windows, candidateYaw, candidatePitch, baseMetrics)
					if score <= bestScore {
						continue
					}
					bestScore = score
					bestYaw = candidateYaw
					bestPitch = candidatePitch
					bestAY = ay
					bestAP = ap
					bestPY = py
					bestPP = pp
				}
			}
		}
	}
	for ay := bestAY - 0.20; ay <= bestAY+0.2001; ay += 0.05 {
		if ay <= 0 {
			continue
		}
		for ap := bestAP - 0.30; ap <= bestAP+0.3001; ap += 0.05 {
			for py := bestPY - 0.30; py <= bestPY+0.3001; py += 0.05 {
				for pp := bestPP - 0.20; pp <= bestPP+0.2001; pp += 0.05 {
					if pp <= 0 {
						continue
					}
					candidateYaw, candidatePitch := movementMacroApplyJointTransform(yawTimeline, pitchTimeline, yawBaseline, pitchBaseline, ay, ap, py, pp)
					score := movementMacroJointTimelineScore(track, windows, candidateYaw, candidatePitch, baseMetrics)
					if score <= bestScore {
						continue
					}
					bestScore = score
					bestYaw = candidateYaw
					bestPitch = candidatePitch
				}
			}
		}
	}
	return bestYaw, bestPitch
}

func movementMacroApplyJointTransform(yawTimeline map[int]float64, pitchTimeline map[int]float64, yawBaseline float64, pitchBaseline float64, ay float64, ap float64, py float64, pp float64) (map[int]float64, map[int]float64) {
	indexes := map[int]bool{}
	for sampleIndex := range yawTimeline {
		indexes[sampleIndex] = true
	}
	for sampleIndex := range pitchTimeline {
		indexes[sampleIndex] = true
	}
	outYaw := make(map[int]float64, len(indexes))
	outPitch := make(map[int]float64, len(indexes))
	for sampleIndex := range indexes {
		yawValue, yawOK := yawTimeline[sampleIndex]
		if !yawOK {
			yawValue = yawBaseline
		}
		pitchValue, pitchOK := pitchTimeline[sampleIndex]
		if !pitchOK {
			pitchValue = pitchBaseline
		}
		yawCentered := movementWrapDegrees(yawValue - yawBaseline)
		pitchCentered := pitchValue - pitchBaseline
		if yawOK || pitchOK {
			outYaw[sampleIndex] = movementWrapDegrees(yawBaseline + (ay * yawCentered) + (ap * pitchCentered))
			outPitch[sampleIndex] = movementFoldPitchDegrees(pitchBaseline + (py * yawCentered) + (pp * pitchCentered))
		}
	}
	return outYaw, outPitch
}

type movementMacroJointMetrics struct {
	yawSignal   float64
	pitchSignal float64
	yawLeak     float64
	pitchLeak   float64
	idleYaw     float64
	idlePitch   float64
}

func movementMacroJointMetricsForTimelines(track MovementTrack, windows []movementMacroWindow, yawTimeline map[int]float64, pitchTimeline map[int]float64) movementMacroJointMetrics {
	yawSignal, _ := movementMacroAverageWindowSpanByKind(track, windows, yawTimeline, "yaw")
	pitchSignal, _ := movementMacroAverageWindowSpanByKind(track, windows, pitchTimeline, "pitch")
	yawLeak, _ := movementMacroAverageWindowSpanByKind(track, windows, yawTimeline, "pitch")
	pitchLeak, _ := movementMacroAverageWindowSpanByKind(track, windows, pitchTimeline, "yaw")
	idleYaw, _ := movementMacroAverageWindowSpanByKind(track, windows, yawTimeline, "idle")
	idlePitch, _ := movementMacroAverageWindowSpanByKind(track, windows, pitchTimeline, "idle")
	return movementMacroJointMetrics{
		yawSignal:   yawSignal,
		pitchSignal: pitchSignal,
		yawLeak:     yawLeak,
		pitchLeak:   pitchLeak,
		idleYaw:     idleYaw,
		idlePitch:   idlePitch,
	}
}

func movementMacroJointTimelineScore(track MovementTrack, windows []movementMacroWindow, yawTimeline map[int]float64, pitchTimeline map[int]float64, base movementMacroJointMetrics) float64 {
	yawScore, yawMatches := movementMacroTimelineScore(track, windows, yawTimeline, "yaw", 0)
	pitchScore, pitchMatches := movementMacroTimelineScore(track, windows, pitchTimeline, "pitch", 0)
	if yawMatches == 0 || pitchMatches == 0 {
		return -1e9
	}
	metrics := movementMacroJointMetricsForTimelines(track, windows, yawTimeline, pitchTimeline)
	if base.yawSignal > 0 && metrics.yawSignal < base.yawSignal*0.80 {
		return -1e9
	}
	if base.pitchSignal > 0 && metrics.pitchSignal < base.pitchSignal*0.80 {
		return -1e9
	}
	return yawScore + pitchScore +
		(math.Min(metrics.yawSignal, 180) / 14) +
		(math.Min(metrics.pitchSignal, 120) / 10) -
		(metrics.yawLeak / 1.25) -
		(metrics.pitchLeak / 1.25) -
		(metrics.idleYaw / 4) -
		(metrics.idlePitch / 4) +
		((metrics.yawSignal - base.yawSignal) / 8) +
		((metrics.pitchSignal - base.pitchSignal) / 8) -
		((metrics.yawLeak - base.yawLeak) / 4) -
		((metrics.pitchLeak - base.pitchLeak) / 4)
}

func movementMacroMixYawWithPitch(yawTimeline map[int]float64, pitchTimeline map[int]float64, pitchBaseline float64, alpha float64) map[int]float64 {
	if alpha == 0 {
		return yawTimeline
	}
	out := make(map[int]float64, len(yawTimeline))
	for sampleIndex, yawValue := range yawTimeline {
		pitchValue, ok := pitchTimeline[sampleIndex]
		if !ok {
			out[sampleIndex] = yawValue
			continue
		}
		out[sampleIndex] = movementWrapDegrees(yawValue + (alpha * (pitchValue - pitchBaseline)))
	}
	return out
}

func movementMacroYawIsolationScore(track MovementTrack, windows []movementMacroWindow, timeline map[int]float64) float64 {
	score, matches := movementMacroTimelineScore(track, windows, timeline, "yaw", 0)
	if matches == 0 {
		return -1e9
	}
	yawSignal, _ := movementMacroAverageWindowSpanByKind(track, windows, timeline, "yaw")
	pitchLeak, _ := movementMacroAverageWindowSpanByKind(track, windows, timeline, "pitch")
	score += math.Min(yawSignal, 180) / 18
	score -= pitchLeak / 2
	return score
}

func movementMacroBestBaseShift(track MovementTrack, windows []movementMacroWindow, yawTimeline map[int]float64, pitchTimeline map[int]float64) int {
	bestShift := 0
	bestScore := -1e9
	for shift := -1500; shift <= 1500; shift += 250 {
		shifted := movementMacroShiftWindows(windows, shift)
		score, matches := movementMacroTimelineScore(track, shifted, yawTimeline, "yaw", 0)
		if matches == 0 {
			continue
		}
		yawSignal, _ := movementMacroAverageWindowSpanByKind(track, shifted, yawTimeline, "yaw")
		yawLeak, _ := movementMacroAverageWindowSpanByKind(track, shifted, yawTimeline, "pitch")
		idleYaw, _ := movementMacroAverageWindowSpanByKind(track, shifted, yawTimeline, "idle")
		score += math.Min(yawSignal, 180) / 12
		score -= yawLeak
		score -= idleYaw / 4
		if len(pitchTimeline) > 0 {
			pitchScore, pitchMatches := movementMacroTimelineScore(track, shifted, pitchTimeline, "pitch", 0)
			if pitchMatches > 0 {
				pitchSignal, _ := movementMacroAverageWindowSpanByKind(track, shifted, pitchTimeline, "pitch")
				pitchLeak, _ := movementMacroAverageWindowSpanByKind(track, shifted, pitchTimeline, "yaw")
				score += pitchScore
				score += math.Min(pitchSignal, 120) / 12
				score -= pitchLeak / 2
			}
		}
		if score > bestScore {
			bestScore = score
			bestShift = shift
		}
	}
	return bestShift
}

func movementMacroShiftWindows(windows []movementMacroWindow, shift int) []movementMacroWindow {
	if shift == 0 || len(windows) == 0 {
		return windows
	}
	shifted := make([]movementMacroWindow, len(windows))
	copy(shifted, windows)
	for index := range shifted {
		shifted[index].startMs += shift
		shifted[index].endMs += shift
	}
	return shifted
}

func movementMacroAverageWindowSpanByKind(track MovementTrack, windows []movementMacroWindow, timeline map[int]float64, kind string) (float64, int) {
	total := 0.0
	count := 0
	for _, window := range windows {
		if !movementMacroKindMatches(window.kind, kind) {
			continue
		}
		values := movementMacroWindowValues(track, timeline, window, 0)
		if len(values) < 2 {
			continue
		}
		total += movementHeadingSequenceSpan(values)
		count++
	}
	if count == 0 {
		return 0, 0
	}
	return total / float64(count), count
}

func movementMacroReferenceLevel(track MovementTrack, windows []movementMacroWindow, timeline map[int]float64, kinds ...string) (float64, bool) {
	values := make([]float64, 0, 64)
	for _, window := range windows {
		matched := false
		for _, kind := range kinds {
			if movementMacroKindMatches(window.kind, kind) {
				matched = true
				break
			}
		}
		if !matched {
			continue
		}
		values = append(values, movementMacroWindowValues(track, timeline, window, 0)...)
	}
	if len(values) == 0 {
		return 0, false
	}
	return movementMacroMean(values), true
}

func movementMacroTimelineMean(timeline map[int]float64) float64 {
	if len(timeline) == 0 {
		return 0
	}
	values := make([]float64, 0, len(timeline))
	for _, value := range timeline {
		values = append(values, value)
	}
	return movementMacroMean(values)
}

func movementMacroStabilizePitchTimeline(track MovementTrack, windows []movementMacroWindow, timeline map[int]float64) map[int]float64 {
	if len(timeline) == 0 {
		return timeline
	}
	baseline, ok := movementMacroReferenceLevel(track, windows, timeline, "yaw", "move+yaw", "move", "idle")
	if !ok {
		baseline = movementMacroTimelineMean(timeline)
	}
	ordered := make([]int, 0, len(timeline))
	for sampleIndex := range timeline {
		ordered = append(ordered, sampleIndex)
	}
	sort.Ints(ordered)
	out := make(map[int]float64, len(timeline))
	last := 0.0
	haveLast := false
	for _, sampleIndex := range ordered {
		value := movementFoldPitchDegrees(timeline[sampleIndex])
		candidates := []float64{
			value,
			-value,
		}
		best := candidates[0]
		bestScore := math.MaxFloat64
		for _, candidate := range candidates {
			score := math.Abs(candidate - baseline)
			if haveLast {
				score = (math.Abs(candidate-last) * 2) + (math.Abs(candidate-baseline) * 0.25)
			}
			if score < bestScore {
				bestScore = score
				best = candidate
			}
		}
		out[sampleIndex] = movementFoldPitchDegrees(best)
		last = out[sampleIndex]
		haveLast = true
	}
	for _, window := range windows {
		if !movementMacroKindMatches(window.kind, "yaw") && window.kind != "move" && window.kind != "idle" {
			continue
		}
		indexes := movementMacroWindowSampleIndexes(track, window, 0)
		if len(indexes) == 0 {
			continue
		}
		values := make([]float64, 0, len(indexes))
		for _, sampleIndex := range indexes {
			value, ok := out[sampleIndex]
			if !ok {
				continue
			}
			values = append(values, movementFoldPitchDegrees(value))
		}
		if len(values) == 0 {
			continue
		}
		target := movementFoldPitchDegrees(movementMacroMean(values))
		for _, sampleIndex := range indexes {
			if _, ok := out[sampleIndex]; !ok {
				continue
			}
			value := target
			out[sampleIndex] = value
		}
	}
	return out
}

func movementMacroStabilizeYawTimeline(track MovementTrack, windows []movementMacroWindow, timeline map[int]float64) map[int]float64 {
	if len(timeline) == 0 {
		return timeline
	}
	out := make(map[int]float64, len(timeline))
	for sampleIndex, value := range timeline {
		out[sampleIndex] = movementWrapDegrees(value)
	}
	for _, window := range windows {
		if !movementMacroKindMatches(window.kind, "pitch") && window.kind != "move" && window.kind != "idle" {
			continue
		}
		indexes := movementMacroWindowSampleIndexes(track, window, 0)
		if len(indexes) == 0 {
			continue
		}
		values := make([]float64, 0, len(indexes))
		for _, sampleIndex := range indexes {
			value, ok := out[sampleIndex]
			if !ok {
				continue
			}
			values = append(values, value)
		}
		if len(values) == 0 {
			continue
		}
		target := movementWrapDegrees(circularMeanDegrees(values))
		for _, sampleIndex := range indexes {
			if _, ok := out[sampleIndex]; !ok {
				continue
			}
			value := target
			out[sampleIndex] = value
		}
	}
	return out
}

func signFloat64(value float64) float64 {
	switch {
	case value > 0:
		return 1
	case value < 0:
		return -1
	default:
		return 0
	}
}
