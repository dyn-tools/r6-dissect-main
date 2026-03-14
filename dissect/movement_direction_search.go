package dissect

import (
	"encoding/binary"
	"encoding/hex"
	"math"
	"sort"
	"strconv"
	"strings"
)

type MovementDirectionSearch struct {
	TrackActorID           string                       `json:"trackActorID"`
	TrackLabel             string                       `json:"trackLabel,omitempty"`
	BestDenseCandidate     *MovementDirectionCandidate  `json:"bestDenseCandidate,omitempty"`
	BestSameActorCandidate *MovementDirectionCandidate  `json:"bestSameActorCandidate,omitempty"`
	BestAnchorCandidate    *MovementDirectionFusedTrack `json:"bestAnchorCandidate,omitempty"`
	BestFusedCandidate     *MovementDirectionFusedTrack `json:"bestFusedCandidate,omitempty"`
	DensePropInspection    *MovementDirectionPropProbe  `json:"densePropInspection,omitempty"`
	SameActorPropProbe     *MovementDirectionPropProbe  `json:"sameActorPropProbe,omitempty"`
	Candidates             []MovementDirectionCandidate `json:"candidates,omitempty"`
}

type MovementDirectionPropProbe struct {
	PropID     string                          `json:"propID"`
	LaneGroups []MovementDirectionLaneGroup    `json:"laneGroups,omitempty"`
	ActorOrder []MovementDirectionActorSegment `json:"actorOrder,omitempty"`
}

type MovementDirectionLaneGroup struct {
	Source        string                          `json:"source"`
	Plane         string                          `json:"plane"`
	VectorOffset  int                             `json:"vectorOffset"`
	TotalSamples  int                             `json:"totalSamples"`
	ActorCount    int                             `json:"actorCount"`
	BestCandidate *MovementDirectionCandidate     `json:"bestCandidate,omitempty"`
	Actors        []MovementDirectionActorSegment `json:"actors,omitempty"`
}

type MovementDirectionActorSegment struct {
	ActorID               string  `json:"actorID"`
	SampleCount           int     `json:"sampleCount"`
	FirstOffset           int     `json:"firstOffset"`
	LastOffset            int     `json:"lastOffset"`
	FirstMilliseconds     *int    `json:"firstMilliseconds,omitempty"`
	LastMilliseconds      *int    `json:"lastMilliseconds,omitempty"`
	FirstAngle            float64 `json:"firstAngle,omitempty"`
	LastAngle             float64 `json:"lastAngle,omitempty"`
	GapFromPreviousOffset int     `json:"gapFromPreviousOffset,omitempty"`
}

type MovementDirectionCandidate struct {
	Source                string  `json:"source,omitempty"`
	PropID                string  `json:"propID"`
	ActorID               string  `json:"actorID"`
	VectorOffset          int     `json:"vectorOffset"`
	Plane                 string  `json:"plane"`
	Alignment             string  `json:"alignment,omitempty"`
	ByteShift             int     `json:"byteShift,omitempty"`
	TimeShiftMilliseconds int     `json:"timeShiftMilliseconds,omitempty"`
	AngleScale            float64 `json:"angleScale,omitempty"`
	HeadingOffsetDegrees  float64 `json:"headingOffsetDegrees"`
	SampleCount           int     `json:"sampleCount"`
	MeanErrorDegrees      float64 `json:"meanErrorDegrees"`
	MedianErrorDegrees    float64 `json:"medianErrorDegrees"`
	P90ErrorDegrees       float64 `json:"p90ErrorDegrees"`
	MeanCosineAgreement   float64 `json:"meanCosineAgreement"`
	MakesSense            bool    `json:"makesSense,omitempty"`
}

type movementDirectionAngleSample struct {
	offset       int
	sampleIndex  int
	milliseconds int
	hasTime      bool
	angle        float64
}

type MovementDirectionFusedTrack struct {
	Source              string   `json:"source,omitempty"`
	PropIDs             []string `json:"propIDs,omitempty"`
	ActorIDs            []string `json:"actorIDs,omitempty"`
	VectorOffsets       []int    `json:"vectorOffsets,omitempty"`
	Planes              []string `json:"planes,omitempty"`
	AlignmentModes      []string `json:"alignmentModes,omitempty"`
	CandidateCount      int      `json:"candidateCount"`
	UsedPacketCount     int      `json:"usedPacketCount"`
	SampleCount         int      `json:"sampleCount"`
	CoveragePercent     float64  `json:"coveragePercent,omitempty"`
	MeanErrorDegrees    float64  `json:"meanErrorDegrees"`
	MedianErrorDegrees  float64  `json:"medianErrorDegrees"`
	P90ErrorDegrees     float64  `json:"p90ErrorDegrees"`
	MeanCosineAgreement float64  `json:"meanCosineAgreement"`
	MakesSense          bool     `json:"makesSense,omitempty"`
	CandidateKeys       []string `json:"-"`
}

type movementDirectionPrediction struct {
	sampleIndex     int
	offset          int
	movementDegrees float64
	viewingDegrees  float64
}

type movementDirectionPositionPoint struct {
	sampleIndex   int
	offset        int
	position      Vector3
	timeInSeconds *float64
}

type movementDirectionCandidateEval struct {
	candidate  MovementDirectionCandidate
	stream     movementDirectionStream
	prediction []movementDirectionPrediction
	packetKeys map[string]bool
}

type movementDirectionScoredStream struct {
	stream    movementDirectionStream
	unwrapped []movementDirectionAngleSample
	candidate MovementDirectionCandidate
	rawCount  int
}

type movementDirectionAnchorResult struct {
	track      *MovementDirectionFusedTrack
	derived    map[int]float64
	packetKeys map[string]bool
	actors     map[string]bool
}

type movementDirectionSearchResult struct {
	summary             *MovementDirectionSearch
	derivedByActor      map[string]map[int]float64
	timelineByActor     map[string]map[int]float64
	pitchTimelineByActor map[string]map[int]float64
	supplementalPackets map[string]bool
	supplementalActors  map[string]bool
}

func movementDirectionSearch(header Header, buf []byte, start int, zeroActorPrefixes map[string]int, primary []MovementTrack) *MovementDirectionSearch {
	return movementDirectionSearchWithDerived(header, buf, start, zeroActorPrefixes, primary).summary
}

func movementDirectionSearchWithDerived(header Header, buf []byte, start int, zeroActorPrefixes map[string]int, primary []MovementTrack) movementDirectionSearchResult {
	if len(primary) == 0 {
		return movementDirectionSearchResult{}
	}
	base := primary[0]
	movementSamples := movementTrackMovementAngles(base)
	minSamples := movementDirectionMinSampleCount(header)
	if len(movementSamples) < minSamples {
		return movementDirectionSearchResult{}
	}
	allowedProps := movementDirectionPropAllowlist(header, buf, start, zeroActorPrefixes, base.ActorID)
	clockEntries, _, _, _, _ := movementRecoveredClockEntries(buf)
	streams := movementDirectionStreams(header, buf, start, zeroActorPrefixes, allowedProps, clockEntries)
	if len(header.Players) == 1 {
		streams = append(streams, movementDirectionPropFusedDeltaStreams(streams, minSamples)...)
		streams = append(streams, movementDirectionTargetVectorStreams(header, buf, start, zeroActorPrefixes, allowedProps, clockEntries, base)...)
	}
	inspectionStreams := append([]movementDirectionStream(nil), streams...)
	scored := make([]movementDirectionScoredStream, 0, len(streams))
	for _, stream := range streams {
		unwrapped := movementUnwrapAngleSamples(stream.samples)
		candidate, ok := movementDirectionCandidateAtShift(movementSamples, stream, unwrapped, 0, minSamples)
		if ok {
			scored = append(scored, movementDirectionScoredStream{
				stream:    stream,
				unwrapped: unwrapped,
				candidate: candidate,
				rawCount:  len(stream.samples),
			})
		}
	}
	if len(scored) == 0 {
		return movementDirectionSearchResult{}
	}
	sort.Slice(scored, func(i, j int) bool {
		return movementDirectionCandidateBetter(scored[i].candidate, scored[j].candidate)
	})
	maxScored := 64
	if len(header.Players) == 1 {
		maxScored = 1024
	}
	if len(scored) > maxScored {
		scored = movementDirectionShortlistScored(scored, len(header.Players) == 1, maxScored)
	}
	candidates := make([]MovementDirectionCandidate, 0, len(scored))
	evals := make([]movementDirectionCandidateEval, 0, len(scored))
	for _, item := range scored {
		candidate, ok := movementDirectionCandidateForStream(movementSamples, item.stream, item.unwrapped, item.candidate, minSamples)
		if ok {
			candidates = append(candidates, candidate)
			predictions, packetKeys := movementDirectionPredictionsForCandidate(movementSamples, item.stream, item.unwrapped, candidate)
			if len(predictions) >= 24 {
				evals = append(evals, movementDirectionCandidateEval{
					candidate:  candidate,
					stream:     item.stream,
					prediction: predictions,
					packetKeys: packetKeys,
				})
			}
		}
	}
	if len(candidates) == 0 {
		return movementDirectionSearchResult{}
	}
	if len(header.Players) == 1 {
		targetedProps := map[string]bool{}
		if dense := movementBestDenseDirectionCandidate(candidates, len(movementSamples)); dense != nil {
			targetedProps[dense.PropID] = true
		}
		if sameActor := movementBestSameActorDirectionCandidate(candidates, base.ActorID, minSamples); sameActor != nil {
			targetedProps[sameActor.PropID] = true
		}
		if len(targetedProps) > 0 {
			extraStreams := movementDirectionTargetedAlignedWordDeltaStreams(header, buf, start, zeroActorPrefixes, targetedProps, clockEntries)
			inspectionStreams = append(inspectionStreams, extraStreams...)
			for _, stream := range extraStreams {
				unwrapped := movementUnwrapAngleSamples(stream.samples)
				seed, ok := movementDirectionCandidateAtShift(movementSamples, stream, unwrapped, 0, minSamples)
				if !ok {
					continue
				}
				candidate, ok := movementDirectionCandidateForStream(movementSamples, stream, unwrapped, seed, minSamples)
				if !ok {
					continue
				}
				candidates = append(candidates, candidate)
				predictions, packetKeys := movementDirectionPredictionsForCandidate(movementSamples, stream, unwrapped, candidate)
				if len(predictions) >= 24 {
					evals = append(evals, movementDirectionCandidateEval{
						candidate:  candidate,
						stream:     stream,
						prediction: predictions,
						packetKeys: packetKeys,
					})
				}
			}
		}
		if len(targetedProps) > 0 {
			lateStreams := movementDirectionTargetedLateOffsetStreams(header, buf, start, zeroActorPrefixes, targetedProps, clockEntries)
			inspectionStreams = append(inspectionStreams, lateStreams...)
			for _, stream := range lateStreams {
				unwrapped := movementUnwrapAngleSamples(stream.samples)
				seed, ok := movementDirectionCandidateAtShift(movementSamples, stream, unwrapped, 0, minSamples)
				if !ok {
					continue
				}
				candidate, ok := movementDirectionCandidateForStream(movementSamples, stream, unwrapped, seed, minSamples)
				if !ok {
					continue
				}
				candidates = append(candidates, candidate)
				predictions, packetKeys := movementDirectionPredictionsForCandidate(movementSamples, stream, unwrapped, candidate)
				if len(predictions) >= 24 {
					evals = append(evals, movementDirectionCandidateEval{
						candidate:  candidate,
						stream:     stream,
						prediction: predictions,
						packetKeys: packetKeys,
					})
				}
			}
		}
		if len(targetedProps) > 0 {
			pairStreams := movementDirectionTargetedScalarPairStreams(inspectionStreams, targetedProps, minSamples)
			inspectionStreams = append(inspectionStreams, pairStreams...)
			for _, stream := range pairStreams {
				unwrapped := movementUnwrapAngleSamples(stream.samples)
				seed, ok := movementDirectionCandidateAtShift(movementSamples, stream, unwrapped, 0, minSamples)
				if !ok {
					continue
				}
				candidate, ok := movementDirectionCandidateForStream(movementSamples, stream, unwrapped, seed, minSamples)
				if !ok {
					continue
				}
				candidates = append(candidates, candidate)
				predictions, packetKeys := movementDirectionPredictionsForCandidate(movementSamples, stream, unwrapped, candidate)
				if len(predictions) >= 24 {
					evals = append(evals, movementDirectionCandidateEval{
						candidate:  candidate,
						stream:     stream,
						prediction: predictions,
						packetKeys: packetKeys,
					})
				}
			}
		}
	}
	sort.Slice(candidates, func(i, j int) bool {
		return movementDirectionCandidateBetter(candidates[i], candidates[j])
	})
	bestDense := movementBestDenseDirectionCandidate(candidates, len(movementSamples))
	bestSameActor := movementBestSameActorDirectionCandidate(candidates, base.ActorID, minSamples)
	bestFused, derived, supplementalPackets, supplementalActors := movementBestFusedDirectionCandidate(movementSamples, evals, base.ActorID, minSamples)
	bestDenseAligned, denseAlignedDerived, denseAlignedPackets, denseAlignedActors := movementBestDensePropAlignedDirectionCandidate(movementSamples, evals, bestDense, bestSameActor)
	if movementDirectionPreferDenseFallback(fusedTrackMetrics(bestFused), fusedTrackMetrics(bestDenseAligned)) {
		bestFused = bestDenseAligned
		derived = denseAlignedDerived
		supplementalPackets = denseAlignedPackets
		supplementalActors = denseAlignedActors
	}
	bestDenseFallback, denseDerived, densePackets, denseActors := movementBestDenseFallbackDirectionCandidate(movementSamples, evals, bestDense, bestSameActor)
	if movementDirectionPreferDenseFallback(fusedTrackMetrics(bestFused), fusedTrackMetrics(bestDenseFallback)) {
		bestFused = bestDenseFallback
		derived = denseDerived
		supplementalPackets = densePackets
		supplementalActors = denseActors
	}
	bestDenseLocal, denseLocalDerived, denseLocalPackets, denseLocalActors := movementBestDenseLocalFragmentDirectionCandidate(movementSamples, evals, bestDense, bestSameActor)
	if movementDirectionPreferDenseFallback(fusedTrackMetrics(bestFused), fusedTrackMetrics(bestDenseLocal)) {
		bestFused = bestDenseLocal
		derived = denseLocalDerived
		supplementalPackets = denseLocalPackets
		supplementalActors = denseLocalActors
	}
	densePropID := ""
	if bestDense != nil {
		densePropID = bestDense.PropID
	}
	bestAnchor := movementBestAnchorExtendedDirectionCandidate(movementSamples, streams, evals, bestSameActor, densePropID, minSamples)
	if bestAnchor.track != nil && movementDirectionFusionImproves(fusedTrackMetrics(bestFused), fusedTrackMetrics(bestAnchor.track)) {
		bestFused = bestAnchor.track
		derived = bestAnchor.derived
		supplementalPackets = bestAnchor.packetKeys
		supplementalActors = bestAnchor.actors
	}
	maxCandidates := 16
	if len(header.Players) == 1 {
		maxCandidates = 2048
	}
	if len(candidates) > maxCandidates {
		candidates = candidates[:maxCandidates]
	}
	var denseProbe *MovementDirectionPropProbe
	if bestDense != nil {
		denseProbe = movementDirectionPropProbeForProp(inspectionStreams, candidates, bestDense.PropID)
	}
	var sameActorProbe *MovementDirectionPropProbe
	if bestSameActor != nil && (bestDense == nil || bestSameActor.PropID != bestDense.PropID) {
		sameActorProbe = movementDirectionPropProbeForProp(inspectionStreams, candidates, bestSameActor.PropID)
	}
	timelineByActor := map[string]map[int]float64{}
	if timeline, ok := movementDirectionBestProjectedTimeline(base, candidates, evals, bestFused, bestSameActor); ok {
		timelineByActor[base.ActorID] = timeline
	}
	pitchTimelineByActor := map[string]map[int]float64{}
	if len(header.Players) == 1 {
		if yawTimeline, ok := timelineByActor[base.ActorID]; ok && bestDense != nil {
			if pitchTimeline, ok := movementDirectionBestProjectedPitchTimeline(base, inspectionStreams, bestDense, bestSameActor, yawTimeline); ok {
				pitchTimelineByActor[base.ActorID] = pitchTimeline
			}
		}
	}
	return movementDirectionSearchResult{
		summary: &MovementDirectionSearch{
			TrackActorID:           base.ActorID,
			TrackLabel:             base.Label,
			BestDenseCandidate:     bestDense,
			BestSameActorCandidate: bestSameActor,
			BestAnchorCandidate:    bestAnchor.track,
			BestFusedCandidate:     bestFused,
			DensePropInspection:    denseProbe,
			SameActorPropProbe:     sameActorProbe,
			Candidates:             candidates,
		},
		derivedByActor:       map[string]map[int]float64{base.ActorID: derived},
		timelineByActor:      timelineByActor,
		pitchTimelineByActor: pitchTimelineByActor,
		supplementalPackets:  supplementalPackets,
		supplementalActors:   supplementalActors,
	}
}

func movementDirectionProjectCandidateTimeline(track MovementTrack, evals []movementDirectionCandidateEval, candidate MovementDirectionCandidate) (map[int]float64, bool) {
	eval, ok := movementDirectionFindEval(evals, candidate)
	if !ok {
		return nil, false
	}
	unwrapped := movementUnwrapAngleSamples(eval.stream.samples)
	if len(unwrapped) == 0 || len(track.Samples) == 0 {
		return nil, false
	}
	out := map[int]float64{}
	switch candidate.Alignment {
	case "time":
		for sampleIndex, sample := range track.Samples {
			if sample.TimeInSeconds == nil {
				continue
			}
			milliseconds := int(math.Round(*sample.TimeInSeconds * 1000))
			angle, ok := movementInterpolatedAngleAtMilliseconds(unwrapped, milliseconds+candidate.TimeShiftMilliseconds, 1200)
			if !ok {
				continue
			}
			out[sampleIndex] = movementWrapDegrees((angle * candidate.AngleScale) + candidate.HeadingOffsetDegrees)
		}
	case "progress":
		denom := maxInt(len(track.Samples)-1, 1)
		progress := progressShift(candidate.ByteShift, len(track.Samples), len(unwrapped))
		for sampleIndex := range track.Samples {
			angle, ok := movementInterpolatedAngleAtProgress(unwrapped, (float64(sampleIndex)/float64(denom))+progress)
			if !ok {
				continue
			}
			out[sampleIndex] = movementWrapDegrees((angle * candidate.AngleScale) + candidate.HeadingOffsetDegrees)
		}
	default:
		for sampleIndex, sample := range track.Samples {
			angle, ok := movementInterpolatedAngleAtOffset(unwrapped, sample.Offset+candidate.ByteShift, 32768)
			if !ok {
				continue
			}
			out[sampleIndex] = movementWrapDegrees((angle * candidate.AngleScale) + candidate.HeadingOffsetDegrees)
		}
	}
	if movementPercent(len(out), len(track.Samples)) < 10 {
		return nil, false
	}
	return out, true
}

func movementDirectionProjectStreamTimelineWithAlignment(track MovementTrack, stream movementDirectionStream, alignment MovementDirectionCandidate, transform func(float64) float64) (map[int]float64, bool) {
	unwrapped := movementUnwrapAngleSamples(stream.samples)
	if len(unwrapped) == 0 || len(track.Samples) == 0 {
		return nil, false
	}
	out := map[int]float64{}
	switch alignment.Alignment {
	case "time":
		for sampleIndex, sample := range track.Samples {
			if sample.TimeInSeconds == nil {
				continue
			}
			milliseconds := int(math.Round(*sample.TimeInSeconds * 1000))
			angle, ok := movementInterpolatedAngleAtMilliseconds(unwrapped, milliseconds+alignment.TimeShiftMilliseconds, 1200)
			if !ok {
				continue
			}
			out[sampleIndex] = transform(angle)
		}
	case "progress":
		denom := maxInt(len(track.Samples)-1, 1)
		progress := progressShift(alignment.ByteShift, len(track.Samples), len(unwrapped))
		for sampleIndex := range track.Samples {
			angle, ok := movementInterpolatedAngleAtProgress(unwrapped, (float64(sampleIndex)/float64(denom))+progress)
			if !ok {
				continue
			}
			out[sampleIndex] = transform(angle)
		}
	default:
		for sampleIndex, sample := range track.Samples {
			angle, ok := movementInterpolatedAngleAtOffset(unwrapped, sample.Offset+alignment.ByteShift, 32768)
			if !ok {
				continue
			}
			out[sampleIndex] = transform(angle)
		}
	}
	if movementPercent(len(out), len(track.Samples)) < 10 {
		return nil, false
	}
	return out, true
}

func movementDirectionProjectFusedTimeline(track MovementTrack, evals []movementDirectionCandidateEval, fused *MovementDirectionFusedTrack) (map[int]float64, bool) {
	if fused == nil || len(fused.CandidateKeys) == 0 || len(track.Samples) == 0 {
		return nil, false
	}
	selected := make([]movementDirectionCandidateEval, 0, len(fused.CandidateKeys))
	byKey := map[string]movementDirectionCandidateEval{}
	for _, eval := range evals {
		byKey[movementDirectionCandidateEvalKey(eval.candidate)] = eval
	}
	for _, key := range fused.CandidateKeys {
		eval, ok := byKey[key]
		if !ok {
			continue
		}
		selected = append(selected, eval)
	}
	if len(selected) == 0 {
		return nil, false
	}
	type projectedEval struct {
		eval         movementDirectionCandidateEval
		timeline     map[int]float64
		globalWeight float64
	}
	projected := make([]projectedEval, 0, len(selected))
	for _, eval := range selected {
		timeline, ok := movementDirectionProjectCandidateTimeline(track, evals, eval.candidate)
		if !ok {
			continue
		}
		projected = append(projected, projectedEval{
			eval:         eval,
			timeline:     timeline,
			globalWeight: movementDirectionDenseLocalSeedScore(eval.candidate),
		})
	}
	if len(projected) == 0 {
		return nil, false
	}
	out := map[int]float64{}
	lastKey := ""
	lastAngle := 0.0
	hasLast := false
	for sampleIndex := range track.Samples {
		bestIndex := -1
		bestScore := 0.0
		bestAngle := 0.0
		for index, candidate := range projected {
			angle, ok := candidate.timeline[sampleIndex]
			if !ok {
				continue
			}
			score := candidate.globalWeight
			key := movementDirectionCandidateEvalKey(candidate.eval.candidate)
			if key == lastKey {
				score += 0.06
			}
			if hasLast {
				jump := math.Abs(movementWrapDegrees(angle - lastAngle))
				score -= jump / 720
			}
			if bestIndex == -1 || score > bestScore {
				bestIndex = index
				bestScore = score
				bestAngle = angle
			}
		}
		if bestIndex == -1 {
			continue
		}
		if bestScore < 0.15 {
			continue
		}
		out[sampleIndex] = bestAngle
		lastKey = movementDirectionCandidateEvalKey(projected[bestIndex].eval.candidate)
		lastAngle = bestAngle
		hasLast = true
	}
	if movementPercent(len(out), len(track.Samples)) < 10 {
		return nil, false
	}
	movementDirectionInterpolateDerivedHeadingGaps(movementTrackAngleSamples(track), out, 8)
	return out, true
}

func movementDirectionBestProjectedTimeline(track MovementTrack, candidates []MovementDirectionCandidate, evals []movementDirectionCandidateEval, fused *MovementDirectionFusedTrack, sameActor *MovementDirectionCandidate) (map[int]float64, bool) {
	bestTimeline := map[int]float64(nil)
	bestScore := -1.0
	seen := map[string]bool{}
	stationary := movementTrackStationarySampleIndexes(track)
	anchorTimeline := map[int]float64(nil)
	if sameActor != nil && sameActor.MakesSense {
		if timeline, ok := movementDirectionProjectCandidateTimeline(track, evals, *sameActor); ok {
			anchorTimeline = timeline
		}
	}
	if timeline, ok := movementDirectionProjectFusedTimeline(track, evals, fused); ok {
		score := movementDirectionProjectedTimelineScore(track, timeline, nil, stationary, anchorTimeline)
		if score > bestScore {
			bestTimeline = timeline
			bestScore = score
		}
	}
	if sameActor != nil && sameActor.MakesSense {
		if anchorTimeline != nil {
			score := movementDirectionProjectedTimelineScore(track, anchorTimeline, sameActor, stationary, anchorTimeline)
			if score > bestScore {
				bestTimeline = anchorTimeline
				bestScore = score
			}
		}
		seen[movementDirectionCandidateKey(*sameActor)] = true
	}
	for _, candidate := range candidates {
		key := movementDirectionCandidateKey(candidate)
		if seen[key] {
			continue
		}
		seen[key] = true
		if !candidate.MakesSense {
			continue
		}
		if candidate.SampleCount < 24 {
			continue
		}
		if candidate.Alignment != "time" && !strings.Contains(candidate.Source, "late-") {
			continue
		}
		timeline, ok := movementDirectionProjectCandidateTimeline(track, evals, candidate)
		if !ok {
			continue
		}
		score := movementDirectionProjectedTimelineScore(track, timeline, &candidate, stationary, anchorTimeline)
		if score > bestScore {
			bestTimeline = timeline
			bestScore = score
		}
	}
	if timeline, ok := movementDirectionProjectImplicitQuaternionTimeline(track); ok {
		score := movementDirectionProjectedTimelineScore(track, timeline, nil, stationary, anchorTimeline) + 12
		if score > bestScore {
			bestTimeline = timeline
			bestScore = score
		}
	}
	if bestTimeline == nil {
		return nil, false
	}
	return bestTimeline, true
}

func movementDirectionBestProjectedPitchTimeline(track MovementTrack, streams []movementDirectionStream, bestDense *MovementDirectionCandidate, bestSameActor *MovementDirectionCandidate, yawTimeline map[int]float64) (map[int]float64, bool) {
	if len(track.Samples) == 0 || len(streams) == 0 || bestDense == nil || len(yawTimeline) == 0 {
		return nil, false
	}
	stationary := movementTrackStationarySampleIndexes(track)
	bestScore := -1.0
	var bestTimeline map[int]float64
	for _, stream := range streams {
		if !movementDirectionPitchStreamAllowed(stream) {
			continue
		}
		alignments := movementDirectionPitchAlignmentsForStream(stream, bestDense, bestSameActor)
		for _, alignment := range alignments {
			for _, variant := range movementDirectionPitchTransforms() {
				timeline, ok := movementDirectionProjectStreamTimelineWithAlignment(track, stream, alignment, variant)
				if !ok {
					continue
				}
				score := movementDirectionProjectedPitchTimelineScore(track, yawTimeline, timeline, stationary)
				if score > bestScore {
					bestScore = score
					bestTimeline = timeline
				}
			}
		}
	}
	if bestTimeline == nil || bestScore < 0.15 {
		return nil, false
	}
	return bestTimeline, true
}

func movementDirectionPitchAlignmentsForStream(stream movementDirectionStream, bestDense *MovementDirectionCandidate, bestSameActor *MovementDirectionCandidate) []MovementDirectionCandidate {
	alignments := []MovementDirectionCandidate{}
	add := func(candidate MovementDirectionCandidate) {
		for _, existing := range alignments {
			if existing.Alignment == candidate.Alignment &&
				existing.ByteShift == candidate.ByteShift &&
				existing.TimeShiftMilliseconds == candidate.TimeShiftMilliseconds &&
				existing.PropID == candidate.PropID &&
				existing.ActorID == candidate.ActorID {
				return
			}
		}
		alignments = append(alignments, candidate)
	}
	if bestDense != nil && bestDense.PropID == stream.propID {
		if bestDense.ActorID == stream.actorID || bestDense.ActorID == "fused" || stream.actorID == "fused" {
			add(*bestDense)
		}
	}
	if bestSameActor != nil && bestSameActor.PropID == stream.propID {
		if bestSameActor.ActorID == stream.actorID || bestSameActor.ActorID == "fused" || stream.actorID == "fused" {
			add(*bestSameActor)
		}
	}
	if movementDirectionTimedSampleCount(stream.samples) >= 8 {
		for _, shift := range []int{-140, -105, -70, -35, 0, 35, 70, 105, 140} {
			add(MovementDirectionCandidate{
				PropID:                stream.propID,
				ActorID:               stream.actorID,
				Alignment:             "time",
				TimeShiftMilliseconds: shift,
			})
		}
	}
	for _, shift := range []int{-32, 0, 32} {
		add(MovementDirectionCandidate{
			PropID:    stream.propID,
			ActorID:   stream.actorID,
			Alignment: "progress",
			ByteShift: shift,
		})
	}
	for _, shift := range []int{-16384, 0, 16384} {
		add(MovementDirectionCandidate{
			PropID:    stream.propID,
			ActorID:   stream.actorID,
			Alignment: "offset",
			ByteShift: shift,
		})
	}
	return alignments
}

func movementDirectionPitchStreamAllowed(stream movementDirectionStream) bool {
	if strings.Contains(stream.source, "target-pos") || strings.Contains(stream.source, "vector-xy") || strings.Contains(stream.source, "vector-neg-xy") {
		return false
	}
	return strings.Contains(stream.source, "late-") || strings.Contains(stream.source, "quat")
}

func movementDirectionTimedSampleCount(samples []movementDirectionAngleSample) int {
	count := 0
	for _, sample := range samples {
		if sample.hasTime {
			count++
		}
	}
	return count
}

func movementDirectionPitchTransforms() []func(float64) float64 {
	return []func(float64) float64{
		func(angle float64) float64 { return movementFoldPitchDegrees(angle) },
		func(angle float64) float64 { return movementFoldPitchDegrees(-angle) },
		func(angle float64) float64 { return movementFoldPitchDegrees(movementWrapDegrees(angle + 90)) },
		func(angle float64) float64 { return movementFoldPitchDegrees(movementWrapDegrees(angle - 90)) },
	}
}

func movementDirectionProjectedPitchTimelineScore(track MovementTrack, yawTimeline map[int]float64, pitchTimeline map[int]float64, stationary map[int]bool) float64 {
	if len(track.Samples) < 2 || len(yawTimeline) == 0 || len(pitchTimeline) == 0 {
		return -1
	}
	coverage := movementPercent(len(pitchTimeline), len(track.Samples))
	if coverage < 10 {
		return -1
	}
	coveredPairs := 0
	yawStableReward := 0.0
	yawLeakPenalty := 0.0
	pitchSignalReward := 0.0
	minPitch := 0.0
	maxPitch := 0.0
	hasPitch := false
	for sampleIndex := 0; sampleIndex+1 < len(track.Samples); sampleIndex++ {
		if !stationary[sampleIndex] && !stationary[sampleIndex+1] {
			continue
		}
		yawA, okYawA := yawTimeline[sampleIndex]
		yawB, okYawB := yawTimeline[sampleIndex+1]
		pitchA, okPitchA := pitchTimeline[sampleIndex]
		pitchB, okPitchB := pitchTimeline[sampleIndex+1]
		if !okYawA || !okYawB || !okPitchA || !okPitchB {
			continue
		}
		if !hasPitch {
			minPitch = pitchA
			maxPitch = pitchA
			hasPitch = true
		}
		if pitchA < minPitch {
			minPitch = pitchA
		}
		if pitchA > maxPitch {
			maxPitch = pitchA
		}
		if pitchB < minPitch {
			minPitch = pitchB
		}
		if pitchB > maxPitch {
			maxPitch = pitchB
		}
		yawDelta := math.Abs(movementWrapDegrees(yawB - yawA))
		pitchDelta := math.Abs(pitchB - pitchA)
		coveredPairs++
		switch {
		case yawDelta >= 8 && pitchDelta <= 1.5:
			yawStableReward += 1.5
		case yawDelta >= 8 && pitchDelta >= 4:
			yawLeakPenalty += 2
		case yawDelta <= 2 && pitchDelta >= 3:
			pitchSignalReward += 1.25
		case yawDelta <= 2 && pitchDelta >= 1:
			pitchSignalReward += 0.25
		}
	}
	if coveredPairs < 8 || !hasPitch {
		return -1
	}
	pitchSpan := maxPitch - minPitch
	score := coverage / 100
	score += yawStableReward / float64(coveredPairs)
	score += pitchSignalReward / float64(coveredPairs)
	score -= yawLeakPenalty / float64(coveredPairs)
	score += math.Min(pitchSpan, 120) / 240
	if pitchSpan < 8 {
		score -= 0.5
	}
	return score
}

func movementDirectionProjectImplicitQuaternionTimeline(track MovementTrack) (map[int]float64, bool) {
	if len(track.Samples) == 0 {
		return nil, false
	}
	out := map[int]float64{}
	for sampleIndex, sample := range track.Samples {
		if sample.Rotation == nil {
			continue
		}
		qx := float64(sample.Rotation.X)
		qy := float64(sample.Rotation.Y)
		qz := float64(sample.Rotation.Z)
		ww := 1 - ((qx * qx) + (qy * qy) + (qz * qz))
		if ww < 0 {
			if ww > -0.01 {
				ww = 0
			} else {
				continue
			}
		}
		quat := movementQuaternion{X: float32(qx), Y: float32(qy), Z: float32(qz), W: float32(math.Sqrt(ww))}
		world := movementRotateVector(quat, Vector3{Z: 1})
		heading := math.Atan2(float64(world.Y), float64(world.X)) * 180 / math.Pi
		out[sampleIndex] = movementWrapDegrees(heading)
	}
	if movementPercent(len(out), len(track.Samples)) < 50 {
		return nil, false
	}
	return out, true
}

func movementDirectionProjectedTimelineScore(track MovementTrack, timeline map[int]float64, candidate *MovementDirectionCandidate, stationary map[int]bool, anchorTimeline map[int]float64) float64 {
	if len(track.Samples) == 0 || len(timeline) == 0 {
		return -1
	}
	coverage := movementPercent(len(timeline), len(track.Samples))
	if coverage <= 0 {
		return -1
	}
	changes := 0
	hasPrevious := false
	previous := 0.0
	minValue := 0.0
	maxValue := 0.0
	for sampleIndex := range track.Samples {
		angle, ok := timeline[sampleIndex]
		if !ok {
			continue
		}
		if !hasPrevious {
			previous = angle
			minValue = angle
			maxValue = angle
			hasPrevious = true
			continue
		}
		current := angle
		for current-previous > 180 {
			current -= 360
		}
		for current-previous < -180 {
			current += 360
		}
		if math.Abs(current-previous) >= 1 {
			changes++
		}
		if current < minValue {
			minValue = current
		}
		if current > maxValue {
			maxValue = current
		}
		previous = current
	}
	span := math.Abs(maxValue - minValue)
	score := coverage*4 + math.Min(span, 1440)/12 + math.Min(float64(changes), float64(len(track.Samples)))/3
	if candidate != nil {
		score += candidate.MeanCosineAgreement * 50
		score -= candidate.MeanErrorDegrees / 6
		if candidate.Alignment == "time" {
			score += 10
		}
		if strings.Contains(candidate.Source, "late-") {
			score += 8
		}
		if candidate.ActorID == track.ActorID {
			score += 4
		}
	}
	if coverage < 25 {
		score -= 40
	}
	if span < 45 {
		score -= 30
	}
	if changes < 16 {
		score -= 20
	}
	stationaryCoverage, stationarySpan, stationaryChanges := movementDirectionStationaryTimelineMetrics(track, timeline, stationary)
	score += stationaryCoverage * 5
	score += math.Min(stationarySpan, 1440) / 8
	score += math.Min(float64(stationaryChanges), float64(len(track.Samples))) / 2
	if stationaryCoverage < 10 {
		score -= 35
	}
	if stationarySpan < 60 {
		score -= 25
	}
	if stationaryChanges < 12 {
		score -= 20
	}
	if overlap, meanError, meanCosine := movementDirectionTimelineOverlapMetrics(timeline, anchorTimeline); overlap >= 8 {
		score += meanCosine * 80
		score -= meanError / 4
		if overlap >= 24 {
			score += 15
		}
	} else if anchorTimeline != nil {
		score -= 10
	}
	return score
}

func movementTrackStationarySampleIndexes(track MovementTrack) map[int]bool {
	if len(track.Samples) < 2 {
		return nil
	}
	stationary := map[int]bool{}
	var previousPosition *Vector3
	var previousTime *float64
	var previousIndex int
	for sampleIndex, sample := range track.Samples {
		if sample.Position == nil {
			continue
		}
		if previousPosition != nil {
			dx := float64(sample.Position.X - previousPosition.X)
			dy := float64(sample.Position.Y - previousPosition.Y)
			distance := math.Hypot(dx, dy)
			isStationary := false
			if sample.TimeInSeconds != nil && previousTime != nil {
				deltaTime := math.Abs(*sample.TimeInSeconds - *previousTime)
				if deltaTime > 0 && distance/deltaTime < 0.15 {
					isStationary = true
				}
			} else if distance < 0.015 {
				isStationary = true
			}
			if isStationary {
				stationary[previousIndex] = true
				stationary[sampleIndex] = true
			}
		}
		position := *sample.Position
		previousPosition = &position
		if sample.TimeInSeconds != nil {
			timeValue := *sample.TimeInSeconds
			previousTime = &timeValue
		} else {
			previousTime = nil
		}
		previousIndex = sampleIndex
	}
	return stationary
}

func movementDirectionStationaryTimelineMetrics(track MovementTrack, timeline map[int]float64, stationary map[int]bool) (float64, float64, int) {
	if len(stationary) == 0 || len(timeline) == 0 {
		return 0, 0, 0
	}
	sequence := make([]float64, 0, len(stationary))
	covered := 0
	changes := 0
	hasPrevious := false
	previous := 0.0
	for sampleIndex := range track.Samples {
		if !stationary[sampleIndex] {
			continue
		}
		angle, ok := timeline[sampleIndex]
		if !ok {
			continue
		}
		covered++
		sequence = append(sequence, angle)
		if !hasPrevious {
			previous = angle
			hasPrevious = true
			continue
		}
		current := angle
		for current-previous > 180 {
			current -= 360
		}
		for current-previous < -180 {
			current += 360
		}
		if math.Abs(current-previous) >= 1 {
			changes++
		}
		previous = current
	}
	return movementPercent(covered, len(stationary)), movementHeadingSequenceSpan(sequence), changes
}

func movementDirectionTimelineOverlapMetrics(timeline map[int]float64, anchor map[int]float64) (int, float64, float64) {
	if len(timeline) == 0 || len(anchor) == 0 {
		return 0, 0, 0
	}
	differences := make([]float64, 0, minInt(len(timeline), len(anchor)))
	for sampleIndex, angle := range timeline {
		anchorAngle, ok := anchor[sampleIndex]
		if !ok {
			continue
		}
		differences = append(differences, movementWrapDegrees(anchorAngle-angle))
	}
	if len(differences) == 0 {
		return 0, 0, 0
	}
	offset := circularMeanDegrees(differences)
	sumError := 0.0
	sumCosine := 0.0
	count := 0
	for sampleIndex, angle := range timeline {
		anchorAngle, ok := anchor[sampleIndex]
		if !ok {
			continue
		}
		errorDegrees := math.Abs(movementWrapDegrees(anchorAngle - (angle + offset)))
		sumError += errorDegrees
		sumCosine += math.Cos(errorDegrees * math.Pi / 180)
		count++
	}
	if count == 0 {
		return 0, 0, 0
	}
	return count, sumError / float64(count), sumCosine / float64(count)
}

func movementTrackAngleSamples(track MovementTrack) []movementDirectionAngleSample {
	out := make([]movementDirectionAngleSample, 0, len(track.Samples))
	for sampleIndex, sample := range track.Samples {
		entry := movementDirectionAngleSample{
			sampleIndex: sampleIndex,
			offset:      sample.Offset,
		}
		if sample.TimeInSeconds != nil {
			entry.milliseconds = int(math.Round(*sample.TimeInSeconds * 1000))
			entry.hasTime = true
		}
		out = append(out, entry)
	}
	return out
}

func movementDirectionEvalKeys(evals []movementDirectionCandidateEval, indexes []int) []string {
	keys := map[string]bool{}
	for _, index := range indexes {
		if index < 0 || index >= len(evals) {
			continue
		}
		keys[movementDirectionCandidateEvalKey(evals[index].candidate)] = true
	}
	return movementDirectionSortedKeys(keys)
}

func movementDirectionCandidateKeys(candidates ...*MovementDirectionCandidate) []string {
	keys := map[string]bool{}
	for _, candidate := range candidates {
		if candidate == nil {
			continue
		}
		keys[movementDirectionCandidateEvalKey(*candidate)] = true
	}
	return movementDirectionSortedKeys(keys)
}

func movementDirectionTargetedLateOffsetStreams(header Header, buf []byte, start int, zeroActorPrefixes map[string]int, allowedProps map[string]bool, clockEntries []movementClockEntry) []movementDirectionStream {
	minSamples := movementDirectionMinSampleCount(header)
	type streamKey struct {
		source  string
		propID  string
		actorID string
		offset  int
	}
	streams := map[streamKey][]movementDirectionAngleSample{}
	appendSample := func(key streamKey, angle float64, integrate bool, offset int) {
		streams[key] = append(streams[key], movementDirectionAngleSampleAtOffset(offset, angle, clockEntries))
		if !integrate {
			return
		}
		integratedKey := key
		integratedKey.source = key.source + "-integrated"
		prev := streams[integratedKey]
		nextAngle := angle
		if len(prev) > 0 {
			nextAngle += prev[len(prev)-1].angle
		}
		streams[integratedKey] = append(streams[integratedKey], movementDirectionAngleSampleAtOffset(offset, nextAngle, clockEntries))
	}
	for i := start; i < len(buf); i++ {
		candidate, ok := movementCandidateRecordAtCodeVersion(header.CodeVersion, buf, i, zeroActorPrefixes)
		if !ok || !movementPropPatternAllowed(header.CodeVersion, candidate.prop) {
			continue
		}
		propID := hex.EncodeToString(candidate.prop)
		if len(allowedProps) > 0 && !allowedProps[propID] {
			continue
		}
		actorID := hex.EncodeToString(candidate.actor)
		for rel := 42; rel <= 72; rel += 2 {
			if i+rel+2 > len(buf) {
				continue
			}
			u16 := binary.LittleEndian.Uint16(buf[i+rel : i+rel+2])
			if u16 != 0 {
				degrees := float64(u16) * 360 / 65536
				appendSample(streamKey{source: "late-u16", propID: propID, actorID: actorID, offset: rel}, degrees, false, i)
				appendSample(streamKey{source: "late-u16-neg", propID: propID, actorID: actorID, offset: rel}, -degrees, false, i)
			}
			s16 := int16(u16)
			if s16 != 0 {
				degrees := float64(s16) * 180 / 32768
				appendSample(streamKey{source: "late-s16", propID: propID, actorID: actorID, offset: rel}, degrees, true, i)
				appendSample(streamKey{source: "late-s16-neg", propID: propID, actorID: actorID, offset: rel}, -degrees, true, i)
			}
		}
		for rel := 42; rel <= 68; rel += 4 {
			if i+rel+4 > len(buf) {
				continue
			}
			value := math.Float32frombits(binary.LittleEndian.Uint32(buf[i+rel : i+rel+4]))
			if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) || value == 0 {
				continue
			}
			if absFloat64(float64(value)) <= 360 {
				appendSample(streamKey{source: "late-f32-deg", propID: propID, actorID: actorID, offset: rel}, float64(value), false, i)
				appendSample(streamKey{source: "late-f32-deg-neg", propID: propID, actorID: actorID, offset: rel}, -float64(value), false, i)
			}
			if absFloat64(float64(value)) <= math.Pi*4 {
				degrees := float64(value) * 180 / math.Pi
				appendSample(streamKey{source: "late-f32-rad", propID: propID, actorID: actorID, offset: rel}, degrees, false, i)
				appendSample(streamKey{source: "late-f32-rad-neg", propID: propID, actorID: actorID, offset: rel}, -degrees, false, i)
			}
		}
	}
	out := make([]movementDirectionStream, 0, len(streams))
	for key, samples := range streams {
		if len(samples) < minSamples {
			continue
		}
		out = append(out, movementDirectionStream{
			source:       key.source,
			propID:       key.propID,
			actorID:      key.actorID,
			vectorOffset: key.offset,
			axisA:        key.source,
			samples:      samples,
		})
	}
	return movementDirectionFinalizeSoloScalarStreams(out, minSamples)
}

func movementDirectionTargetVectorStreams(header Header, buf []byte, start int, zeroActorPrefixes map[string]int, allowedProps map[string]bool, clockEntries []movementClockEntry, base MovementTrack) []movementDirectionStream {
	minSamples := movementDirectionMinSampleCount(header)
	type streamKey struct {
		source  string
		propID  string
		actorID string
		offset  int
		axisA   string
		axisB   string
	}
	streams := map[streamKey][]movementDirectionAngleSample{}
	appendAngle := func(key streamKey, angle float64, offset int) {
		streams[key] = append(streams[key], movementDirectionAngleSampleAtOffset(offset, angle, clockEntries))
	}
	appendVectorAngles := func(propID string, actorID string, recordOffset int, vectorOffset int, axis string, vec Vector3, playerPos *Vector3) {
		if !movementDirectionVectorPlausible(vec) {
			return
		}
		if angle, ok := movementDirectionAngleFromXY(float64(vec.X), float64(vec.Y)); ok {
			appendAngle(streamKey{source: "vector-xy", propID: propID, actorID: actorID, offset: vectorOffset, axisA: axis, axisB: "xy"}, angle, recordOffset)
			appendAngle(streamKey{source: "vector-neg-xy", propID: propID, actorID: actorID, offset: vectorOffset, axisA: axis, axisB: "-xy"}, movementWrapDegrees(angle+180), recordOffset)
		}
		if playerPos == nil {
			return
		}
		if angle, ok := movementDirectionAngleFromXY(float64(vec.X-playerPos.X), float64(vec.Y-playerPos.Y)); ok {
			appendAngle(streamKey{source: "target-pos-xy", propID: propID, actorID: actorID, offset: vectorOffset, axisA: axis, axisB: "player"}, angle, recordOffset)
			appendAngle(streamKey{source: "target-pos-neg-xy", propID: propID, actorID: actorID, offset: vectorOffset, axisA: axis, axisB: "-player"}, movementWrapDegrees(angle+180), recordOffset)
		}
	}
	for i := start; i < len(buf); i++ {
		candidate, ok := movementCandidateRecordAtCodeVersion(header.CodeVersion, buf, i, zeroActorPrefixes)
		if !ok || !movementPropPatternAllowed(header.CodeVersion, candidate.prop) {
			continue
		}
		propID := hex.EncodeToString(candidate.prop)
		if len(allowedProps) > 0 && !allowedProps[propID] {
			continue
		}
		actorID := hex.EncodeToString(candidate.actor)
		playerPos := movementTrackPositionAtOffset(base, i)
		appendVectorAngles(propID, actorID, i, 22, "primary", candidate.primary, playerPos)
		if candidate.hasSecondary {
			appendVectorAngles(propID, actorID, i, 31, "secondary", candidate.secondary, playerPos)
			delta := Vector3{
				X: candidate.secondary.X - candidate.primary.X,
				Y: candidate.secondary.Y - candidate.primary.Y,
				Z: candidate.secondary.Z - candidate.primary.Z,
			}
			appendVectorAngles(propID, actorID, i, 31, "secondary-primary", delta, nil)
		}
		if header.CodeVersion < Y11S1Alpha3 {
			i += 37
		}
	}
	out := make([]movementDirectionStream, 0, len(streams))
	for key, samples := range streams {
		if len(samples) < minSamples {
			continue
		}
		out = append(out, movementDirectionStream{
			source:       key.source,
			propID:       key.propID,
			actorID:      key.actorID,
			vectorOffset: key.offset,
			axisA:        key.axisA,
			axisB:        key.axisB,
			samples:      samples,
		})
	}
	return out
}

func movementDirectionVectorPlausible(vec Vector3) bool {
	if !movementVectorHasMeaningfulComponent(vec) {
		return false
	}
	for _, component := range []float32{vec.X, vec.Y, vec.Z} {
		value := float64(component)
		if math.IsNaN(value) || math.IsInf(value, 0) || absFloat64(value) > 100000 {
			return false
		}
	}
	return true
}

func movementDirectionAngleFromXY(x float64, y float64) (float64, bool) {
	if math.IsNaN(x) || math.IsNaN(y) || math.IsInf(x, 0) || math.IsInf(y, 0) {
		return 0, false
	}
	if math.Hypot(x, y) < 0.05 {
		return 0, false
	}
	return math.Atan2(y, x) * 180 / math.Pi, true
}

type movementDirectionStream struct {
	source       string
	propID       string
	actorID      string
	vectorOffset int
	axisA        string
	axisB        string
	samples      []movementDirectionAngleSample
}

func movementDirectionStreams(header Header, buf []byte, start int, zeroActorPrefixes map[string]int, allowedProps map[string]bool, clockEntries []movementClockEntry) []movementDirectionStream {
	minSamples := movementDirectionMinSampleCount(header)
	type streamKey struct {
		propID       string
		actorID      string
		vectorOffset int
		axisA        string
		axisB        string
	}
	streams := map[streamKey][]movementDirectionAngleSample{}
	for i := start; i < len(buf); i++ {
		candidate, ok := movementCandidateRecordAtCodeVersion(header.CodeVersion, buf, i, zeroActorPrefixes)
		if !ok || !movementPropPatternAllowed(header.CodeVersion, candidate.prop) {
			continue
		}
		if header.CodeVersion >= Y11S1Alpha3 && movementY11CandidateIsPosition(candidate) {
			continue
		}
		propID := hex.EncodeToString(candidate.prop)
		if len(allowedProps) > 0 && !allowedProps[propID] {
			continue
		}
		if header.CodeVersion >= Y11S1Alpha3 && movementY11PropFamily(candidate.prop) != 2 {
			continue
		}
		actorID := hex.EncodeToString(candidate.actor)
		for rel := 22; rel <= 31; rel++ {
			value, ok := movementVectorAt(buf, i+rel)
			if !ok || !movementVectorHasMeaningfulComponent(value) || !movementVectorComponentsWithin(value, 2.0) {
				continue
			}
			for _, axes := range []struct {
				nameA string
				nameB string
				readA func(Vector3) float64
				readB func(Vector3) float64
			}{
				{nameA: "x", nameB: "y", readA: func(v Vector3) float64 { return float64(v.X) }, readB: func(v Vector3) float64 { return float64(v.Y) }},
				{nameA: "y", nameB: "x", readA: func(v Vector3) float64 { return float64(v.Y) }, readB: func(v Vector3) float64 { return float64(v.X) }},
				{nameA: "x", nameB: "z", readA: func(v Vector3) float64 { return float64(v.X) }, readB: func(v Vector3) float64 { return float64(v.Z) }},
				{nameA: "z", nameB: "x", readA: func(v Vector3) float64 { return float64(v.Z) }, readB: func(v Vector3) float64 { return float64(v.X) }},
				{nameA: "y", nameB: "z", readA: func(v Vector3) float64 { return float64(v.Y) }, readB: func(v Vector3) float64 { return float64(v.Z) }},
				{nameA: "z", nameB: "y", readA: func(v Vector3) float64 { return float64(v.Z) }, readB: func(v Vector3) float64 { return float64(v.Y) }},
			} {
				for _, signA := range []float64{1, -1} {
					for _, signB := range []float64{1, -1} {
						key := streamKey{
							propID:       propID,
							actorID:      actorID,
							vectorOffset: rel,
							axisA:        signedAxisName(axes.nameA, signA),
							axisB:        signedAxisName(axes.nameB, signB),
						}
						angle := math.Atan2(signB*axes.readB(value), signA*axes.readA(value)) * 180 / math.Pi
						streams[key] = append(streams[key], movementDirectionAngleSampleAtOffset(i, angle, clockEntries))
					}
				}
			}
		}
		if header.CodeVersion < Y11S1Alpha3 {
			i += 37
		}
	}
	out := make([]movementDirectionStream, 0, len(streams))
	for key, samples := range streams {
		if len(samples) < minSamples {
			continue
		}
		out = append(out, movementDirectionStream{
			propID:       key.propID,
			actorID:      key.actorID,
			source:       "vector",
			vectorOffset: key.vectorOffset,
			axisA:        key.axisA,
			axisB:        key.axisB,
			samples:      samples,
		})
	}
	out = append(out, movementDirectionInt16Streams(header, buf, start, zeroActorPrefixes, allowedProps, clockEntries)...)
	out = append(out, movementDirectionFloatStreams(header, buf, start, zeroActorPrefixes, allowedProps, clockEntries)...)
	if len(header.Players) == 1 {
		out = append(out, movementDirectionFloat64Streams(header, buf, start, zeroActorPrefixes, allowedProps, clockEntries)...)
		out = append(out, movementDirectionFloat64PairStreams(header, buf, start, zeroActorPrefixes, allowedProps, clockEntries)...)
		out = append(out, movementDirectionInt16PairStreams(header, buf, start, zeroActorPrefixes, allowedProps, clockEntries)...)
		out = append(out, movementDirectionShiftedFloatQuaternionStreams(header, buf, start, zeroActorPrefixes, allowedProps, clockEntries)...)
		out = append(out, movementDirectionPackedQuaternionStreams(header, buf, start, zeroActorPrefixes, allowedProps, clockEntries)...)
		out = append(out, movementDirectionPackedImplicitQuaternionStreams(header, buf, start, zeroActorPrefixes, allowedProps, clockEntries)...)
	}
	if len(header.Players) == 1 {
		out = append(out, movementDirectionQuaternionStreams(header, buf, start, zeroActorPrefixes, allowedProps, clockEntries)...)
		out = movementFuseActorSplitDirectionStreams(out, minSamples)
	}
	return out
}

func movementDirectionQuaternionStreams(header Header, buf []byte, start int, zeroActorPrefixes map[string]int, allowedProps map[string]bool, clockEntries []movementClockEntry) []movementDirectionStream {
	minSamples := movementDirectionMinSampleCount(header)
	type streamKey struct {
		source  string
		propID  string
		actorID string
		offset  int
		model   string
		forward string
	}
	streams := map[streamKey][]movementDirectionAngleSample{}
	models := movementImplicitQuaternionModels()
	windowModels := make([]movementImplicitQuaternionModel, 0, len(models))
	for _, model := range models {
		if model.mode == "implicit_w" {
			windowModels = append(windowModels, model)
		}
	}
	for i := start; i < len(buf); i++ {
		candidate, ok := movementCandidateRecordAtCodeVersion(header.CodeVersion, buf, i, zeroActorPrefixes)
		if !ok || !movementPropPatternAllowed(header.CodeVersion, candidate.prop) {
			continue
		}
		if header.CodeVersion >= Y11S1Alpha3 && movementY11CandidateIsPosition(candidate) {
			continue
		}
		propID := hex.EncodeToString(candidate.prop)
		if len(allowedProps) > 0 && !allowedProps[propID] {
			continue
		}
		actorID := hex.EncodeToString(candidate.actor)
		if candidate.hasQuaternion {
			for _, forward := range movementDirectionForwardAxes() {
				angle := movementQuaternionForwardAngle(candidate.quaternion, forward.axis)
				key := streamKey{source: "quat-22", propID: propID, actorID: actorID, offset: 22, model: "xyzw", forward: forward.name}
				streams[key] = append(streams[key], movementDirectionAngleSampleAtOffset(i, angle, clockEntries))
			}
		}
		if candidate.hasSecondary && movementVectorLooksImplicitQuaternionXYZ(candidate.secondary) && movementVectorLooksZeroLike(candidate.primary, 0.1) {
			for _, model := range models {
				quat, ok := movementY11CandidateQuaternionWithModel(candidate, model)
				if !ok {
					continue
				}
				modelKey := movementImplicitQuaternionModelKey(model)
				for _, forward := range movementDirectionForwardAxes() {
					angle := movementQuaternionForwardAngle(quat, forward.axis)
					key := streamKey{source: "quat-secondary", propID: propID, actorID: actorID, offset: 31, model: modelKey, forward: forward.name}
					streams[key] = append(streams[key], movementDirectionAngleSampleAtOffset(i, angle, clockEntries))
				}
			}
		}
		for rel := 22; rel <= 31; rel++ {
			value, ok := movementVectorAt(buf, i+rel)
			if !ok || !movementVectorLooksImplicitQuaternionXYZ(value) {
				continue
			}
			windowCandidate := movementCandidate{
				primary:      Vector3{},
				secondary:    value,
				hasSecondary: true,
			}
			for _, model := range windowModels {
				quat, ok := movementY11CandidateQuaternionWithModel(windowCandidate, model)
				if !ok {
					continue
				}
				modelKey := movementImplicitQuaternionModelKey(model)
				for _, forward := range movementDirectionForwardAxes() {
					angle := movementQuaternionForwardAngle(quat, forward.axis)
					key := streamKey{source: "quat-window", propID: propID, actorID: actorID, offset: rel, model: modelKey, forward: forward.name}
					streams[key] = append(streams[key], movementDirectionAngleSampleAtOffset(i, angle, clockEntries))
				}
			}
		}
		if header.CodeVersion < Y11S1Alpha3 {
			i += 37
		}
	}
	out := make([]movementDirectionStream, 0, len(streams))
	for key, samples := range streams {
		if len(samples) < minSamples {
			continue
		}
		out = append(out, movementDirectionStream{
			source:       key.source + ":" + key.model,
			propID:       key.propID,
			actorID:      key.actorID,
			vectorOffset: key.offset,
			axisA:        key.forward,
			samples:      samples,
		})
	}
	if len(header.Players) == 1 {
		return movementDirectionFinalizeSoloScalarStreams(out, minSamples)
	}
	return out
}

func movementDirectionForwardAxes() []struct {
	name string
	axis Vector3
} {
	return []struct {
		name string
		axis Vector3
	}{
		{name: "+x", axis: Vector3{X: 1}},
		{name: "-x", axis: Vector3{X: -1}},
		{name: "+y", axis: Vector3{Y: 1}},
		{name: "-y", axis: Vector3{Y: -1}},
		{name: "+z", axis: Vector3{Z: 1}},
		{name: "-z", axis: Vector3{Z: -1}},
	}
}

func movementQuaternionForwardAngle(quat movementQuaternion, axis Vector3) float64 {
	world := movementRotateVector(quat, axis)
	return math.Atan2(float64(world.Y), float64(world.X)) * 180 / math.Pi
}

func movementQuaternionForwardPitchAngle(quat movementQuaternion, axis Vector3) float64 {
	world := movementRotateVector(quat, axis)
	return math.Atan2(float64(world.Z), math.Hypot(float64(world.X), float64(world.Y))) * 180 / math.Pi
}

func movementDirectionQuaternionPitchStreams(header Header, buf []byte, start int, zeroActorPrefixes map[string]int, allowedProps map[string]bool, clockEntries []movementClockEntry) []movementDirectionStream {
	minSamples := movementDirectionMinSampleCount(header)
	type streamKey struct {
		source  string
		propID  string
		actorID string
		offset  int
		model   string
		forward string
	}
	streams := map[streamKey][]movementDirectionAngleSample{}
	models := movementImplicitQuaternionModels()
	windowModels := make([]movementImplicitQuaternionModel, 0, len(models))
	for _, model := range models {
		if model.mode == "implicit_w" {
			windowModels = append(windowModels, model)
		}
	}
	for i := start; i < len(buf); i++ {
		candidate, ok := movementCandidateRecordAtCodeVersion(header.CodeVersion, buf, i, zeroActorPrefixes)
		if !ok || !movementPropPatternAllowed(header.CodeVersion, candidate.prop) {
			continue
		}
		if header.CodeVersion >= Y11S1Alpha3 && movementY11CandidateIsPosition(candidate) {
			continue
		}
		propID := hex.EncodeToString(candidate.prop)
		if len(allowedProps) > 0 && !allowedProps[propID] {
			continue
		}
		actorID := hex.EncodeToString(candidate.actor)
		if candidate.hasQuaternion {
			for _, forward := range movementDirectionForwardAxes() {
				angle := movementQuaternionForwardPitchAngle(candidate.quaternion, forward.axis)
				key := streamKey{source: "quat-22-pitch", propID: propID, actorID: actorID, offset: 22, model: "xyzw", forward: forward.name}
				streams[key] = append(streams[key], movementDirectionAngleSampleAtOffset(i, angle, clockEntries))
			}
		}
		if candidate.hasSecondary && movementVectorLooksImplicitQuaternionXYZ(candidate.secondary) && movementVectorLooksZeroLike(candidate.primary, 0.1) {
			for _, model := range models {
				quat, ok := movementY11CandidateQuaternionWithModel(candidate, model)
				if !ok {
					continue
				}
				modelKey := movementImplicitQuaternionModelKey(model)
				for _, forward := range movementDirectionForwardAxes() {
					angle := movementQuaternionForwardPitchAngle(quat, forward.axis)
					key := streamKey{source: "quat-secondary-pitch", propID: propID, actorID: actorID, offset: 31, model: modelKey, forward: forward.name}
					streams[key] = append(streams[key], movementDirectionAngleSampleAtOffset(i, angle, clockEntries))
				}
			}
		}
		for rel := 22; rel <= 31; rel++ {
			value, ok := movementVectorAt(buf, i+rel)
			if !ok || !movementVectorLooksImplicitQuaternionXYZ(value) {
				continue
			}
			windowCandidate := movementCandidate{
				primary:      Vector3{},
				secondary:    value,
				hasSecondary: true,
			}
			for _, model := range windowModels {
				quat, ok := movementY11CandidateQuaternionWithModel(windowCandidate, model)
				if !ok {
					continue
				}
				modelKey := movementImplicitQuaternionModelKey(model)
				for _, forward := range movementDirectionForwardAxes() {
					angle := movementQuaternionForwardPitchAngle(quat, forward.axis)
					key := streamKey{source: "quat-window-pitch", propID: propID, actorID: actorID, offset: rel, model: modelKey, forward: forward.name}
					streams[key] = append(streams[key], movementDirectionAngleSampleAtOffset(i, angle, clockEntries))
				}
			}
		}
		if header.CodeVersion < Y11S1Alpha3 {
			i += 37
		}
	}
	out := make([]movementDirectionStream, 0, len(streams))
	for key, samples := range streams {
		if len(samples) < minSamples {
			continue
		}
		out = append(out, movementDirectionStream{
			source:       key.source + ":" + key.model,
			propID:       key.propID,
			actorID:      key.actorID,
			vectorOffset: key.offset,
			axisA:        key.forward,
			samples:      samples,
		})
	}
	if len(header.Players) == 1 {
		return movementDirectionFinalizeSoloScalarStreams(out, minSamples)
	}
	return out
}

func movementDirectionInt16Streams(header Header, buf []byte, start int, zeroActorPrefixes map[string]int, allowedProps map[string]bool, clockEntries []movementClockEntry) []movementDirectionStream {
	minSamples := movementDirectionMinSampleCount(header)
	solo := len(header.Players) == 1
	type streamKey struct {
		source  string
		propID  string
		actorID string
		offset  int
	}
	streams := map[streamKey][]movementDirectionAngleSample{}
	integrated := map[streamKey]float64{}
	appendSample := func(key streamKey, angle float64, integrate bool, offset int) {
		streams[key] = append(streams[key], movementDirectionAngleSampleAtOffset(offset, angle, clockEntries))
		if !solo || !integrate {
			return
		}
		integratedKey := key
		integratedKey.source = key.source + "-integrated"
		integrated[integratedKey] += angle
		streams[integratedKey] = append(streams[integratedKey], movementDirectionAngleSampleAtOffset(offset, integrated[integratedKey], clockEntries))
	}
	for i := start; i < len(buf); i++ {
		candidate, ok := movementCandidateRecordAtCodeVersion(header.CodeVersion, buf, i, zeroActorPrefixes)
		if !ok || !movementPropPatternAllowed(header.CodeVersion, candidate.prop) {
			continue
		}
		propID := hex.EncodeToString(candidate.prop)
		if len(allowedProps) > 0 && !allowedProps[propID] {
			continue
		}
		if movementVectorHasMeaningfulComponent(candidate.primary) || candidate.hasQuaternion || (candidate.hasSecondary && movementVectorHasMeaningfulComponent(candidate.secondary)) {
			continue
		}
		actorID := hex.EncodeToString(candidate.actor)
		for rel := 22; rel <= 41; rel++ {
			if i+rel+2 > len(buf) {
				continue
			}
			u16 := binary.LittleEndian.Uint16(buf[i+rel : i+rel+2])
			if u16 != 0 {
				degrees := float64(u16) * 360 / 65536
				appendSample(streamKey{source: "u16", propID: propID, actorID: actorID, offset: rel}, degrees, false, i)
				appendSample(streamKey{source: "u16-neg", propID: propID, actorID: actorID, offset: rel}, -degrees, false, i)
			}
			s16 := int16(u16)
			if s16 != 0 {
				degrees := float64(s16) * 180 / 32768
				appendSample(streamKey{source: "s16", propID: propID, actorID: actorID, offset: rel}, degrees, true, i)
				appendSample(streamKey{source: "s16-neg", propID: propID, actorID: actorID, offset: rel}, -degrees, true, i)
			}
		}
		if header.CodeVersion < Y11S1Alpha3 {
			i += 37
		}
	}
	out := make([]movementDirectionStream, 0, len(streams))
	for key, samples := range streams {
		if len(samples) < minSamples {
			continue
		}
		out = append(out, movementDirectionStream{
			source:       key.source,
			propID:       key.propID,
			actorID:      key.actorID,
			vectorOffset: key.offset,
			axisA:        key.source,
			axisB:        "",
			samples:      samples,
		})
	}
	if len(header.Players) == 1 {
		return movementDirectionFinalizeSoloScalarStreams(out, minSamples)
	}
	return out
}

func movementDirectionInt16PairStreams(header Header, buf []byte, start int, zeroActorPrefixes map[string]int, allowedProps map[string]bool, clockEntries []movementClockEntry) []movementDirectionStream {
	minSamples := movementDirectionMinSampleCount(header)
	type streamKey struct {
		source  string
		propID  string
		actorID string
		offsetA int
		offsetB int
		signA   int
		signB   int
	}
	streams := map[streamKey][]movementDirectionAngleSample{}
	for i := start; i < len(buf); i++ {
		candidate, ok := movementCandidateRecordAtCodeVersion(header.CodeVersion, buf, i, zeroActorPrefixes)
		if !ok || !movementPropPatternAllowed(header.CodeVersion, candidate.prop) {
			continue
		}
		propID := hex.EncodeToString(candidate.prop)
		if len(allowedProps) > 0 && !allowedProps[propID] {
			continue
		}
		if movementVectorHasMeaningfulComponent(candidate.primary) || candidate.hasQuaternion || (candidate.hasSecondary && movementVectorHasMeaningfulComponent(candidate.secondary)) {
			continue
		}
		actorID := hex.EncodeToString(candidate.actor)
		for relA := 22; relA <= 37; relA++ {
			if i+relA+2 > len(buf) {
				continue
			}
			rawA := binary.LittleEndian.Uint16(buf[i+relA : i+relA+2])
			for relB := relA + 1; relB <= minInt(relA+12, 39); relB++ {
				if i+relB+2 > len(buf) {
					continue
				}
				rawB := binary.LittleEndian.Uint16(buf[i+relB : i+relB+2])
				if rawA == 0 && rawB == 0 {
					continue
				}
				for _, source := range []struct {
					name  string
					readA func() float64
					readB func() float64
				}{
					{
						name:  "s16-pair",
						readA: func() float64 { return float64(int16(rawA)) },
						readB: func() float64 { return float64(int16(rawB)) },
					},
					{
						name:  "u16-centered-pair",
						readA: func() float64 { return float64(int(rawA) - 32768) },
						readB: func() float64 { return float64(int(rawB) - 32768) },
					},
				} {
					valueA := source.readA()
					valueB := source.readB()
					if valueA == 0 && valueB == 0 {
						continue
					}
					for _, signA := range []int{1, -1} {
						for _, signB := range []int{1, -1} {
							baseAngle := math.Atan2(float64(signB)*valueB, float64(signA)*valueA) * 180 / math.Pi
							for _, variant := range []struct {
								name    string
								degrees float64
							}{
								{name: source.name, degrees: baseAngle},
								{name: source.name + "-double", degrees: baseAngle * 2},
							} {
								key := streamKey{
									source:  variant.name,
									propID:  propID,
									actorID: actorID,
									offsetA: relA,
									offsetB: relB,
									signA:   signA,
									signB:   signB,
								}
								streams[key] = append(streams[key], movementDirectionAngleSampleAtOffset(i, variant.degrees, clockEntries))
							}
						}
					}
				}
			}
		}
		if header.CodeVersion < Y11S1Alpha3 {
			i += 37
		}
	}
	out := make([]movementDirectionStream, 0, len(streams))
	for key, samples := range streams {
		if len(samples) < minSamples {
			continue
		}
		out = append(out, movementDirectionStream{
			source:       key.source,
			propID:       key.propID,
			actorID:      key.actorID,
			vectorOffset: key.offsetA,
			axisA:        signedAxisName("o"+strconv.Itoa(key.offsetA), float64(key.signA)),
			axisB:        signedAxisName("o"+strconv.Itoa(key.offsetB), float64(key.signB)),
			samples:      samples,
		})
	}
	if len(header.Players) == 1 {
		out = movementAppendDeltaIntegratedDirectionStreams(out, minSamples)
	}
	return out
}

func movementDirectionPackedQuaternionStreams(header Header, buf []byte, start int, zeroActorPrefixes map[string]int, allowedProps map[string]bool, clockEntries []movementClockEntry) []movementDirectionStream {
	minSamples := movementDirectionMinSampleCount(header)
	type streamKey struct {
		source  string
		propID  string
		actorID string
		offset  int
		forward string
	}
	streams := map[streamKey][]movementDirectionAngleSample{}
	for i := start; i < len(buf); i++ {
		candidate, ok := movementCandidateRecordAtCodeVersion(header.CodeVersion, buf, i, zeroActorPrefixes)
		if !ok || !movementPropPatternAllowed(header.CodeVersion, candidate.prop) {
			continue
		}
		propID := hex.EncodeToString(candidate.prop)
		if len(allowedProps) > 0 && !allowedProps[propID] {
			continue
		}
		if movementVectorHasMeaningfulComponent(candidate.primary) || candidate.hasQuaternion || (candidate.hasSecondary && movementVectorHasMeaningfulComponent(candidate.secondary)) {
			continue
		}
		actorID := hex.EncodeToString(candidate.actor)
		for rel := 22; rel <= 34; rel++ {
			if i+rel+8 > len(buf) {
				continue
			}
			raws := [4]uint16{}
			for part := 0; part < 4; part++ {
				raws[part] = binary.LittleEndian.Uint16(buf[i+rel+(part*2) : i+rel+(part*2)+2])
			}
			for _, decode := range []struct {
				name string
				read func(uint16) float32
			}{
				{
					name: "quat-s16",
					read: func(raw uint16) float32 { return float32(int16(raw)) / 32767 },
				},
				{
					name: "quat-u16c",
					read: func(raw uint16) float32 { return float32(int(raw)-32768) / 32768 },
				},
			} {
				for _, order := range []struct {
					name   string
					mapper func([4]float32) movementQuaternion
				}{
					{
						name: "xyzw",
						mapper: func(values [4]float32) movementQuaternion {
							return movementQuaternion{X: values[0], Y: values[1], Z: values[2], W: values[3]}
						},
					},
					{
						name: "wxyz",
						mapper: func(values [4]float32) movementQuaternion {
							return movementQuaternion{W: values[0], X: values[1], Y: values[2], Z: values[3]}
						},
					},
				} {
					values := [4]float32{}
					for part := 0; part < 4; part++ {
						values[part] = decode.read(raws[part])
					}
					quat, ok := movementNormalizedPackedQuaternion(order.mapper(values))
					if !ok {
						continue
					}
					for _, forward := range movementDirectionForwardAxes() {
						angle := movementQuaternionForwardAngle(quat, forward.axis)
						key := streamKey{
							source:  decode.name + ":" + order.name,
							propID:  propID,
							actorID: actorID,
							offset:  rel,
							forward: forward.name,
						}
						streams[key] = append(streams[key], movementDirectionAngleSampleAtOffset(i, angle, clockEntries))
					}
				}
			}
		}
		if header.CodeVersion < Y11S1Alpha3 {
			i += 37
		}
	}
	out := make([]movementDirectionStream, 0, len(streams))
	for key, samples := range streams {
		if len(samples) < minSamples {
			continue
		}
		out = append(out, movementDirectionStream{
			source:       key.source,
			propID:       key.propID,
			actorID:      key.actorID,
			vectorOffset: key.offset,
			axisA:        key.forward,
			samples:      samples,
		})
	}
	if len(header.Players) == 1 {
		out = movementAppendDeltaIntegratedDirectionStreams(out, minSamples)
	}
	return out
}

func movementDirectionPackedImplicitQuaternionStreams(header Header, buf []byte, start int, zeroActorPrefixes map[string]int, allowedProps map[string]bool, clockEntries []movementClockEntry) []movementDirectionStream {
	minSamples := movementDirectionMinSampleCount(header)
	type streamKey struct {
		source  string
		propID  string
		actorID string
		offset  int
		forward string
	}
	streams := map[streamKey][]movementDirectionAngleSample{}
	models := movementPackedImplicitQuaternionModels()
	for i := start; i < len(buf); i++ {
		candidate, ok := movementCandidateRecordAtCodeVersion(header.CodeVersion, buf, i, zeroActorPrefixes)
		if !ok || !movementPropPatternAllowed(header.CodeVersion, candidate.prop) {
			continue
		}
		propID := hex.EncodeToString(candidate.prop)
		if len(allowedProps) > 0 && !allowedProps[propID] {
			continue
		}
		if movementVectorHasMeaningfulComponent(candidate.primary) || candidate.hasQuaternion || (candidate.hasSecondary && movementVectorHasMeaningfulComponent(candidate.secondary)) {
			continue
		}
		actorID := hex.EncodeToString(candidate.actor)
		for rel := 22; rel <= 35; rel++ {
			if i+rel+6 > len(buf) {
				continue
			}
			raws := [3]uint16{}
			for part := 0; part < 3; part++ {
				raws[part] = binary.LittleEndian.Uint16(buf[i+rel+(part*2) : i+rel+(part*2)+2])
			}
			for _, decode := range []struct {
				name string
				read func(uint16) float32
			}{
				{
					name: "quat-implicit-s16",
					read: func(raw uint16) float32 { return float32(int16(raw)) / 32767 },
				},
				{
					name: "quat-implicit-u16c",
					read: func(raw uint16) float32 { return float32(int(raw)-32768) / 32768 },
				},
			} {
				value := Vector3{
					X: decode.read(raws[0]),
					Y: decode.read(raws[1]),
					Z: decode.read(raws[2]),
				}
				if !movementVectorLooksImplicitQuaternionXYZ(value) {
					continue
				}
				windowCandidate := movementCandidate{
					primary:      Vector3{},
					secondary:    value,
					hasSecondary: true,
				}
				for _, model := range models {
					quat, ok := movementY11CandidateQuaternionWithModel(windowCandidate, model)
					if !ok {
						continue
					}
					modelKey := decode.name + ":" + movementImplicitQuaternionModelKey(model)
					for _, forward := range movementDirectionForwardAxes() {
						angle := movementQuaternionForwardAngle(quat, forward.axis)
						key := streamKey{
							source:  modelKey,
							propID:  propID,
							actorID: actorID,
							offset:  rel,
							forward: forward.name,
						}
						streams[key] = append(streams[key], movementDirectionAngleSampleAtOffset(i, angle, clockEntries))
					}
				}
			}
		}
		if header.CodeVersion < Y11S1Alpha3 {
			i += 37
		}
	}
	out := make([]movementDirectionStream, 0, len(streams))
	for key, samples := range streams {
		if len(samples) < minSamples {
			continue
		}
		out = append(out, movementDirectionStream{
			source:       key.source,
			propID:       key.propID,
			actorID:      key.actorID,
			vectorOffset: key.offset,
			axisA:        key.forward,
			samples:      samples,
		})
	}
	if len(header.Players) == 1 {
		out = movementAppendDeltaIntegratedDirectionStreams(out, minSamples)
	}
	return out
}

func movementPackedImplicitQuaternionModels() []movementImplicitQuaternionModel {
	orders := [][3]int{
		{0, 1, 2},
		{2, 1, 0},
	}
	signs := [][3]float32{
		{1, 1, 1},
		{1, 1, -1},
		{1, -1, 1},
		{1, -1, -1},
		{-1, 1, 1},
		{-1, 1, -1},
		{-1, -1, 1},
		{-1, -1, -1},
	}
	models := make([]movementImplicitQuaternionModel, 0, len(orders)*len(signs))
	for _, order := range orders {
		for _, sign := range signs {
			models = append(models, movementImplicitQuaternionModel{
				mode:  "implicit_w",
				order: order,
				signs: sign,
			})
		}
	}
	return models
}

func movementNormalizedPackedQuaternion(quat movementQuaternion) (movementQuaternion, bool) {
	normSquared := float64(quat.X*quat.X) + float64(quat.Y*quat.Y) + float64(quat.Z*quat.Z) + float64(quat.W*quat.W)
	if normSquared < 0.25 || normSquared > 1.75 {
		return movementQuaternion{}, false
	}
	norm := float32(math.Sqrt(normSquared))
	if norm == 0 {
		return movementQuaternion{}, false
	}
	normalized := movementQuaternion{
		X: quat.X / norm,
		Y: quat.Y / norm,
		Z: quat.Z / norm,
		W: quat.W / norm,
	}
	if normalized.W < 0 {
		normalized.X = -normalized.X
		normalized.Y = -normalized.Y
		normalized.Z = -normalized.Z
		normalized.W = -normalized.W
	}
	if !movementQuaternionLooksUnit(normalized) {
		return movementQuaternion{}, false
	}
	return normalized, true
}

func movementDirectionShiftedFloatQuaternionStreams(header Header, buf []byte, start int, zeroActorPrefixes map[string]int, allowedProps map[string]bool, clockEntries []movementClockEntry) []movementDirectionStream {
	minSamples := movementDirectionMinSampleCount(header)
	type streamKey struct {
		source  string
		propID  string
		actorID string
		offset  int
		forward string
	}
	streams := map[streamKey][]movementDirectionAngleSample{}
	orders := []struct {
		name   string
		mapper func([4]float32) movementQuaternion
	}{
		{
			name: "xyzw",
			mapper: func(values [4]float32) movementQuaternion {
				return movementQuaternion{X: values[0], Y: values[1], Z: values[2], W: values[3]}
			},
		},
		{
			name: "wxyz",
			mapper: func(values [4]float32) movementQuaternion {
				return movementQuaternion{W: values[0], X: values[1], Y: values[2], Z: values[3]}
			},
		},
	}
	for i := start; i < len(buf); i++ {
		candidate, ok := movementCandidateRecordAtCodeVersion(header.CodeVersion, buf, i, zeroActorPrefixes)
		if !ok || !movementPropPatternAllowed(header.CodeVersion, candidate.prop) {
			continue
		}
		if header.CodeVersion >= Y11S1Alpha3 && movementY11CandidateIsPosition(candidate) {
			continue
		}
		propID := hex.EncodeToString(candidate.prop)
		if len(allowedProps) > 0 && !allowedProps[propID] {
			continue
		}
		actorID := hex.EncodeToString(candidate.actor)
		for rel := 22; rel <= 31; rel++ {
			if i+rel+16 > len(buf) {
				continue
			}
			values := [4]float32{}
			valid := true
			for part := 0; part < 4; part++ {
				value := math.Float32frombits(binary.LittleEndian.Uint32(buf[i+rel+(part*4) : i+rel+(part*4)+4]))
				if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) || absFloat64(float64(value)) > 1.5 {
					valid = false
					break
				}
				values[part] = value
			}
			if !valid {
				continue
			}
			for _, order := range orders {
				quat, ok := movementNormalizedPackedQuaternion(order.mapper(values))
				if !ok {
					continue
				}
				for _, forward := range movementDirectionForwardAxes() {
					angle := movementQuaternionForwardAngle(quat, forward.axis)
					key := streamKey{
						source:  "quat-f32-window:" + order.name,
						propID:  propID,
						actorID: actorID,
						offset:  rel,
						forward: forward.name,
					}
					streams[key] = append(streams[key], movementDirectionAngleSampleAtOffset(i, angle, clockEntries))
				}
			}
		}
		if header.CodeVersion < Y11S1Alpha3 {
			i += 37
		}
	}
	out := make([]movementDirectionStream, 0, len(streams))
	for key, samples := range streams {
		if len(samples) < minSamples {
			continue
		}
		out = append(out, movementDirectionStream{
			source:       key.source,
			propID:       key.propID,
			actorID:      key.actorID,
			vectorOffset: key.offset,
			axisA:        key.forward,
			samples:      samples,
		})
	}
	return movementAppendDeltaIntegratedDirectionStreams(out, minSamples)
}

func movementDirectionFloatStreams(header Header, buf []byte, start int, zeroActorPrefixes map[string]int, allowedProps map[string]bool, clockEntries []movementClockEntry) []movementDirectionStream {
	minSamples := movementDirectionMinSampleCount(header)
	solo := len(header.Players) == 1
	type streamKey struct {
		source  string
		propID  string
		actorID string
		offset  int
	}
	streams := map[streamKey][]movementDirectionAngleSample{}
	integrated := map[streamKey]float64{}
	appendSample := func(key streamKey, angle float64, integrate bool, offset int) {
		streams[key] = append(streams[key], movementDirectionAngleSampleAtOffset(offset, angle, clockEntries))
		if !solo || !integrate {
			return
		}
		integratedKey := key
		integratedKey.source = key.source + "-integrated"
		integrated[integratedKey] += angle
		streams[integratedKey] = append(streams[integratedKey], movementDirectionAngleSampleAtOffset(offset, integrated[integratedKey], clockEntries))
	}
	for i := start; i < len(buf); i++ {
		candidate, ok := movementCandidateRecordAtCodeVersion(header.CodeVersion, buf, i, zeroActorPrefixes)
		if !ok || !movementPropPatternAllowed(header.CodeVersion, candidate.prop) {
			continue
		}
		if header.CodeVersion >= Y11S1Alpha3 && movementY11CandidateIsPosition(candidate) {
			continue
		}
		propID := hex.EncodeToString(candidate.prop)
		if len(allowedProps) > 0 && !allowedProps[propID] {
			continue
		}
		actorID := hex.EncodeToString(candidate.actor)
		for rel := 22; rel <= 41; rel++ {
			if i+rel+4 > len(buf) {
				continue
			}
			value := math.Float32frombits(binary.LittleEndian.Uint32(buf[i+rel : i+rel+4]))
			if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) || value == 0 {
				continue
			}
			if absFloat64(float64(value)) <= math.Pi*4 {
				degrees := float64(value) * 180 / math.Pi
				appendSample(streamKey{source: "f32-rad", propID: propID, actorID: actorID, offset: rel}, degrees, true, i)
				appendSample(streamKey{source: "f32-rad-neg", propID: propID, actorID: actorID, offset: rel}, -degrees, true, i)
			}
			if absFloat64(float64(value)) <= 360 {
				degrees := float64(value)
				appendSample(streamKey{source: "f32-deg", propID: propID, actorID: actorID, offset: rel}, degrees, false, i)
				appendSample(streamKey{source: "f32-deg-neg", propID: propID, actorID: actorID, offset: rel}, -degrees, false, i)
			}
		}
		if header.CodeVersion < Y11S1Alpha3 {
			i += 37
		}
	}
	out := make([]movementDirectionStream, 0, len(streams))
	for key, samples := range streams {
		if len(samples) < minSamples {
			continue
		}
		out = append(out, movementDirectionStream{
			source:       key.source,
			propID:       key.propID,
			actorID:      key.actorID,
			vectorOffset: key.offset,
			axisA:        key.source,
			samples:      samples,
		})
	}
	return out
}

func movementDirectionFloat64Streams(header Header, buf []byte, start int, zeroActorPrefixes map[string]int, allowedProps map[string]bool, clockEntries []movementClockEntry) []movementDirectionStream {
	minSamples := movementDirectionMinSampleCount(header)
	type streamKey struct {
		source  string
		propID  string
		actorID string
		offset  int
	}
	streams := map[streamKey][]movementDirectionAngleSample{}
	integrated := map[streamKey]float64{}
	appendSample := func(key streamKey, angle float64, integrate bool, offset int) {
		streams[key] = append(streams[key], movementDirectionAngleSampleAtOffset(offset, angle, clockEntries))
		if !integrate {
			return
		}
		integratedKey := key
		integratedKey.source = key.source + "-integrated"
		integrated[integratedKey] += angle
		streams[integratedKey] = append(streams[integratedKey], movementDirectionAngleSampleAtOffset(offset, integrated[integratedKey], clockEntries))
	}
	for i := start; i < len(buf); i++ {
		candidate, ok := movementCandidateRecordAtCodeVersion(header.CodeVersion, buf, i, zeroActorPrefixes)
		if !ok || !movementPropPatternAllowed(header.CodeVersion, candidate.prop) {
			continue
		}
		if header.CodeVersion >= Y11S1Alpha3 && movementY11CandidateIsPosition(candidate) {
			continue
		}
		propID := hex.EncodeToString(candidate.prop)
		if len(allowedProps) > 0 && !allowedProps[propID] {
			continue
		}
		actorID := hex.EncodeToString(candidate.actor)
		for rel := 22; rel <= 41; rel++ {
			if i+rel+8 > len(buf) {
				continue
			}
			value := math.Float64frombits(binary.LittleEndian.Uint64(buf[i+rel : i+rel+8]))
			if math.IsNaN(value) || math.IsInf(value, 0) || value == 0 {
				continue
			}
			if absFloat64(value) <= math.Pi*8 {
				degrees := value * 180 / math.Pi
				appendSample(streamKey{source: "f64-rad", propID: propID, actorID: actorID, offset: rel}, degrees, true, i)
				appendSample(streamKey{source: "f64-rad-neg", propID: propID, actorID: actorID, offset: rel}, -degrees, true, i)
			}
			if absFloat64(value) <= 720 {
				degrees := value
				appendSample(streamKey{source: "f64-deg", propID: propID, actorID: actorID, offset: rel}, degrees, false, i)
				appendSample(streamKey{source: "f64-deg-neg", propID: propID, actorID: actorID, offset: rel}, -degrees, false, i)
			}
		}
		if header.CodeVersion < Y11S1Alpha3 {
			i += 37
		}
	}
	out := make([]movementDirectionStream, 0, len(streams))
	for key, samples := range streams {
		if len(samples) < minSamples {
			continue
		}
		out = append(out, movementDirectionStream{
			source:       key.source,
			propID:       key.propID,
			actorID:      key.actorID,
			vectorOffset: key.offset,
			axisA:        key.source,
			samples:      samples,
		})
	}
	return out
}

func movementDirectionFloat32PairStreams(header Header, buf []byte, start int, zeroActorPrefixes map[string]int, allowedProps map[string]bool, clockEntries []movementClockEntry) []movementDirectionStream {
	minSamples := movementDirectionMinSampleCount(header)
	type streamKey struct {
		source  string
		propID  string
		actorID string
		offsetA int
		offsetB int
		signA   int
		signB   int
	}
	streams := map[streamKey][]movementDirectionAngleSample{}
	for i := start; i < len(buf); i++ {
		candidate, ok := movementCandidateRecordAtCodeVersion(header.CodeVersion, buf, i, zeroActorPrefixes)
		if !ok || !movementPropPatternAllowed(header.CodeVersion, candidate.prop) {
			continue
		}
		if header.CodeVersion >= Y11S1Alpha3 && movementY11CandidateIsPosition(candidate) {
			continue
		}
		propID := hex.EncodeToString(candidate.prop)
		if len(allowedProps) > 0 && !allowedProps[propID] {
			continue
		}
		actorID := hex.EncodeToString(candidate.actor)
		for relA := 22; relA <= 38; relA += 4 {
			if i+relA+4 > len(buf) {
				continue
			}
			valueA := math.Float32frombits(binary.LittleEndian.Uint32(buf[i+relA : i+relA+4]))
			if math.IsNaN(float64(valueA)) || math.IsInf(float64(valueA), 0) || valueA == 0 || absFloat64(float64(valueA)) > 1e6 {
				continue
			}
			for relB := relA + 4; relB <= minInt(relA+12, 38); relB += 4 {
				if i+relB+4 > len(buf) {
					continue
				}
				valueB := math.Float32frombits(binary.LittleEndian.Uint32(buf[i+relB : i+relB+4]))
				if math.IsNaN(float64(valueB)) || math.IsInf(float64(valueB), 0) || valueB == 0 || absFloat64(float64(valueB)) > 1e6 {
					continue
				}
				if !movementDirectionFloatPairLooksDirectional(float64(valueA), float64(valueB)) {
					continue
				}
				for _, signA := range []int{1, -1} {
					for _, signB := range []int{1, -1} {
						baseAngle := math.Atan2(float64(signB)*float64(valueB), float64(signA)*float64(valueA)) * 180 / math.Pi
						for _, variant := range []struct {
							name    string
							degrees float64
						}{
							{name: "f32-pair", degrees: baseAngle},
							{name: "f32-pair-double", degrees: baseAngle * 2},
						} {
							key := streamKey{
								source:  variant.name,
								propID:  propID,
								actorID: actorID,
								offsetA: relA,
								offsetB: relB,
								signA:   signA,
								signB:   signB,
							}
							streams[key] = append(streams[key], movementDirectionAngleSampleAtOffset(i, variant.degrees, clockEntries))
						}
					}
				}
			}
		}
		if header.CodeVersion < Y11S1Alpha3 {
			i += 37
		}
	}
	out := make([]movementDirectionStream, 0, len(streams))
	for key, samples := range streams {
		if len(samples) < minSamples {
			continue
		}
		out = append(out, movementDirectionStream{
			source:       key.source,
			propID:       key.propID,
			actorID:      key.actorID,
			vectorOffset: key.offsetA,
			axisA:        signedAxisName("o"+strconv.Itoa(key.offsetA), float64(key.signA)),
			axisB:        signedAxisName("o"+strconv.Itoa(key.offsetB), float64(key.signB)),
			samples:      samples,
		})
	}
	return out
}

func movementDirectionFloatPairLooksDirectional(valueA float64, valueB float64) bool {
	if math.IsNaN(valueA) || math.IsNaN(valueB) || math.IsInf(valueA, 0) || math.IsInf(valueB, 0) {
		return false
	}
	absA := absFloat64(valueA)
	absB := absFloat64(valueB)
	if absA > 4 || absB > 4 {
		return false
	}
	length := math.Hypot(valueA, valueB)
	return length >= 0.1 && length <= 4
}

func movementDirectionTargetedAlignedWordDeltaStreams(header Header, buf []byte, start int, zeroActorPrefixes map[string]int, allowedProps map[string]bool, clockEntries []movementClockEntry) []movementDirectionStream {
	minSamples := movementDirectionMinSampleCount(header)
	type streamKey struct {
		source  string
		propID  string
		actorID string
		offset  int
	}
	type laneKey struct {
		propID  string
		actorID string
		offset  int
	}
	streams := map[streamKey][]movementDirectionAngleSample{}
	cumulative := map[streamKey]float64{}
	prevFloat := map[laneKey]float64{}
	prevInt32 := map[laneKey]int64{}
	prevLow16 := map[laneKey]int64{}
	prevHigh16 := map[laneKey]int64{}
	haveFloat := map[laneKey]bool{}
	haveInt32 := map[laneKey]bool{}
	haveLow16 := map[laneKey]bool{}
	haveHigh16 := map[laneKey]bool{}
	appendDelta := func(source string, propID string, actorID string, rel int, delta float64, offset int) {
		key := streamKey{source: source, propID: propID, actorID: actorID, offset: rel}
		cumulative[key] += delta
		streams[key] = append(streams[key], movementDirectionAngleSampleAtOffset(offset, cumulative[key], clockEntries))
	}
	for i := start; i < len(buf); i++ {
		candidate, ok := movementCandidateRecordAtCodeVersion(header.CodeVersion, buf, i, zeroActorPrefixes)
		if !ok || !movementPropPatternAllowed(header.CodeVersion, candidate.prop) {
			continue
		}
		if header.CodeVersion >= Y11S1Alpha3 && movementY11CandidateIsPosition(candidate) {
			continue
		}
		propID := hex.EncodeToString(candidate.prop)
		if len(allowedProps) > 0 && !allowedProps[propID] {
			continue
		}
		actorID := hex.EncodeToString(candidate.actor)
		for rel := 22; rel <= 38; rel += 4 {
			if i+rel+4 > len(buf) {
				continue
			}
			lane := laneKey{propID: propID, actorID: actorID, offset: rel}
			raw := binary.LittleEndian.Uint32(buf[i+rel : i+rel+4])

			floatValue := math.Float32frombits(raw)
			if !math.IsNaN(float64(floatValue)) && !math.IsInf(float64(floatValue), 0) {
				if haveFloat[lane] {
					delta := float64(floatValue) - prevFloat[lane]
					if movementDirectionDeltaLooksReasonable(delta, 64) {
						appendDelta("f32-word-delta-integrated", propID, actorID, rel, delta, i)
						appendDelta("f32-word-delta-neg-integrated", propID, actorID, rel, -delta, i)
					}
				}
				prevFloat[lane] = float64(floatValue)
				haveFloat[lane] = true
			}

			intValue := int64(int32(raw))
			if haveInt32[lane] {
				delta := intValue - prevInt32[lane]
				if movementDirectionDeltaLooksReasonable(delta, 1<<24) {
					appendDelta("i32-word-delta-integrated", propID, actorID, rel, float64(delta), i)
					appendDelta("i32-word-delta-neg-integrated", propID, actorID, rel, float64(-delta), i)
				}
			}
			prevInt32[lane] = intValue
			haveInt32[lane] = true

			low16 := int64(int16(raw & 0xffff))
			if haveLow16[lane] {
				delta := low16 - prevLow16[lane]
				if movementDirectionDeltaLooksReasonable(delta, 1<<14) {
					appendDelta("s16-word-lo-delta-integrated", propID, actorID, rel, float64(delta), i)
					appendDelta("s16-word-lo-neg-delta-integrated", propID, actorID, rel, float64(-delta), i)
				}
			}
			prevLow16[lane] = low16
			haveLow16[lane] = true

			high16 := int64(int16(raw >> 16))
			if haveHigh16[lane] {
				delta := high16 - prevHigh16[lane]
				if movementDirectionDeltaLooksReasonable(delta, 1<<14) {
					appendDelta("s16-word-hi-delta-integrated", propID, actorID, rel, float64(delta), i)
					appendDelta("s16-word-hi-neg-delta-integrated", propID, actorID, rel, float64(-delta), i)
				}
			}
			prevHigh16[lane] = high16
			haveHigh16[lane] = true
		}
		if header.CodeVersion < Y11S1Alpha3 {
			i += 37
		}
	}
	out := make([]movementDirectionStream, 0, len(streams))
	for key, samples := range streams {
		if len(samples) < minSamples {
			continue
		}
		out = append(out, movementDirectionStream{
			source:       key.source,
			propID:       key.propID,
			actorID:      key.actorID,
			vectorOffset: key.offset,
			axisA:        key.source,
			samples:      samples,
		})
	}
	return movementFuseActorSplitDirectionStreams(out, minSamples)
}

func movementDirectionTargetedScalarPairStreams(streams []movementDirectionStream, allowedProps map[string]bool, minSamples int) []movementDirectionStream {
	type pairKey struct {
		source  string
		propID  string
		actorID string
		offsetA int
		offsetB int
		signA   int
		signB   int
	}
	type groupKey struct {
		sourceBase string
		propID     string
		actorID    string
	}
	type sourceGroupKey struct {
		sourceBase string
		propID     string
	}
	grouped := map[groupKey][]movementDirectionStream{}
	for _, stream := range streams {
		propID := normalizeMovementPropID(stream.propID)
		if propID == "" || (len(allowedProps) > 0 && !allowedProps[propID]) {
			continue
		}
		if len(stream.samples) < minSamples || !movementDirectionSourceCanScalarPair(stream.source) {
			continue
		}
		grouped[groupKey{
			sourceBase: movementDirectionScalarSourceBase(stream.source),
			propID:     propID,
			actorID:    stream.actorID,
		}] = append(grouped[groupKey{
			sourceBase: movementDirectionScalarSourceBase(stream.source),
			propID:     propID,
			actorID:    stream.actorID,
		}], stream)
	}
	groupAllowed := map[groupKey]bool{}
	for key, fragments := range grouped {
		sourceKey := sourceGroupKey{sourceBase: key.sourceBase, propID: key.propID}
		_ = sourceKey
		_ = fragments
	}
	groupRanks := map[sourceGroupKey][]struct {
		key        groupKey
		maxSamples int
	}{}
	for key, fragments := range grouped {
		maxSamples := 0
		for _, fragment := range fragments {
			if len(fragment.samples) > maxSamples {
				maxSamples = len(fragment.samples)
			}
		}
		sourceKey := sourceGroupKey{sourceBase: key.sourceBase, propID: key.propID}
		groupRanks[sourceKey] = append(groupRanks[sourceKey], struct {
			key        groupKey
			maxSamples int
		}{
			key:        key,
			maxSamples: maxSamples,
		})
	}
	for _, ranks := range groupRanks {
		sort.Slice(ranks, func(i, j int) bool {
			if ranks[i].maxSamples == ranks[j].maxSamples {
				return ranks[i].key.actorID < ranks[j].key.actorID
			}
			return ranks[i].maxSamples > ranks[j].maxSamples
		})
		if len(ranks) > 3 {
			ranks = ranks[:3]
		}
		for _, rank := range ranks {
			groupAllowed[rank.key] = true
		}
	}
	outMap := map[pairKey][]movementDirectionAngleSample{}
	for key, fragments := range grouped {
		if !groupAllowed[key] {
			continue
		}
		if len(fragments) < 2 {
			continue
		}
		sort.Slice(fragments, func(i, j int) bool {
			if len(fragments[i].samples) == len(fragments[j].samples) {
				if fragments[i].vectorOffset == fragments[j].vectorOffset {
					return fragments[i].source < fragments[j].source
				}
				return fragments[i].vectorOffset < fragments[j].vectorOffset
			}
			return len(fragments[i].samples) > len(fragments[j].samples)
		})
		if len(fragments) > 8 {
			fragments = fragments[:8]
		}
		sort.Slice(fragments, func(i, j int) bool {
			if fragments[i].vectorOffset == fragments[j].vectorOffset {
				return fragments[i].source < fragments[j].source
			}
			return fragments[i].vectorOffset < fragments[j].vectorOffset
		})
		for i := 0; i < len(fragments); i++ {
			for j := i + 1; j < len(fragments); j++ {
				left := fragments[i]
				right := fragments[j]
				if left.vectorOffset == right.vectorOffset {
					continue
				}
				shared := movementDirectionScalarPairSamples(left.samples, right.samples, minSamples)
				if len(shared.offsets) < minSamples {
					continue
				}
				for _, signA := range []int{1, -1} {
					for _, signB := range []int{1, -1} {
						base := make([]movementDirectionAngleSample, 0, len(shared.offsets))
						doubled := make([]movementDirectionAngleSample, 0, len(shared.offsets))
						for index := range shared.offsets {
							angle := math.Atan2(float64(signB)*shared.right[index], float64(signA)*shared.left[index]) * 180 / math.Pi
							base = append(base, movementDirectionAngleSample{
								offset:       shared.offsets[index],
								sampleIndex:  shared.sampleIndexes[index],
								milliseconds: shared.milliseconds[index],
								hasTime:      shared.hasTime[index],
								angle:        angle,
							})
							doubled = append(doubled, movementDirectionAngleSample{
								offset:       shared.offsets[index],
								sampleIndex:  shared.sampleIndexes[index],
								milliseconds: shared.milliseconds[index],
								hasTime:      shared.hasTime[index],
								angle:        angle * 2,
							})
						}
						if len(base) >= minSamples {
							outMap[pairKey{
								source:  key.sourceBase + "-scalar-pair",
								propID:  key.propID,
								actorID: key.actorID,
								offsetA: left.vectorOffset,
								offsetB: right.vectorOffset,
								signA:   signA,
								signB:   signB,
							}] = base
							outMap[pairKey{
								source:  key.sourceBase + "-scalar-pair-double",
								propID:  key.propID,
								actorID: key.actorID,
								offsetA: left.vectorOffset,
								offsetB: right.vectorOffset,
								signA:   signA,
								signB:   signB,
							}] = doubled
						}
					}
				}
			}
		}
	}
	out := make([]movementDirectionStream, 0, len(outMap))
	for key, samples := range outMap {
		if len(samples) < minSamples {
			continue
		}
		out = append(out, movementDirectionStream{
			source:       key.source,
			propID:       key.propID,
			actorID:      key.actorID,
			vectorOffset: key.offsetA,
			axisA:        signedAxisName("o"+strconv.Itoa(key.offsetA), float64(key.signA)),
			axisB:        signedAxisName("o"+strconv.Itoa(key.offsetB), float64(key.signB)),
			samples:      samples,
		})
	}
	return movementFuseActorSplitDirectionStreams(out, minSamples)
}

type movementDirectionScalarPairShared struct {
	offsets       []int
	sampleIndexes []int
	milliseconds  []int
	hasTime       []bool
	left          []float64
	right         []float64
}

func movementDirectionScalarPairSamples(left []movementDirectionAngleSample, right []movementDirectionAngleSample, minSamples int) movementDirectionScalarPairShared {
	if len(left) == 0 || len(right) == 0 {
		return movementDirectionScalarPairShared{}
	}
	if len(left) > len(right) {
		left, right = right, left
	}
	out := movementDirectionScalarPairShared{
		offsets:       make([]int, 0, minInt(len(left), len(right))),
		sampleIndexes: make([]int, 0, minInt(len(left), len(right))),
		milliseconds:  make([]int, 0, minInt(len(left), len(right))),
		hasTime:       make([]bool, 0, minInt(len(left), len(right))),
		left:          make([]float64, 0, minInt(len(left), len(right))),
		right:         make([]float64, 0, minInt(len(left), len(right))),
	}
	for _, sample := range left {
		other, ok := movementInterpolatedAngleSampleAtOffset(right, sample.offset, 32768)
		if !ok {
			continue
		}
		out.offsets = append(out.offsets, sample.offset)
		out.sampleIndexes = append(out.sampleIndexes, sample.sampleIndex)
		out.milliseconds = append(out.milliseconds, sample.milliseconds)
		out.hasTime = append(out.hasTime, sample.hasTime)
		out.left = append(out.left, sample.angle)
		out.right = append(out.right, other.angle)
	}
	if len(out.offsets) < minSamples {
		return movementDirectionScalarPairShared{}
	}
	return out
}

func movementInterpolatedAngleSampleAtOffset(samples []movementDirectionAngleSample, offset int, window int) (movementDirectionAngleSample, bool) {
	if len(samples) == 0 {
		return movementDirectionAngleSample{}, false
	}
	index := sort.Search(len(samples), func(i int) bool {
		return samples[i].offset >= offset
	})
	if index == 0 {
		if samples[0].offset-offset > window {
			return movementDirectionAngleSample{}, false
		}
		return samples[0], true
	}
	if index >= len(samples) {
		last := samples[len(samples)-1]
		if offset-last.offset > window {
			return movementDirectionAngleSample{}, false
		}
		return last, true
	}
	left := samples[index-1]
	right := samples[index]
	if offset-left.offset > window && right.offset-offset > window {
		return movementDirectionAngleSample{}, false
	}
	if left.offset == right.offset {
		return left, true
	}
	if offset-left.offset > window {
		return right, true
	}
	if right.offset-offset > window {
		return left, true
	}
	ratio := float64(offset-left.offset) / float64(right.offset-left.offset)
	interp := left
	interp.offset = offset
	interp.angle = left.angle + ((right.angle - left.angle) * ratio)
	if !left.hasTime && right.hasTime {
		interp.milliseconds = right.milliseconds
		interp.hasTime = true
	} else if left.hasTime && !right.hasTime {
		interp.milliseconds = left.milliseconds
		interp.hasTime = true
	} else if left.hasTime && right.hasTime {
		interp.milliseconds = left.milliseconds + int(math.Round(float64(right.milliseconds-left.milliseconds)*ratio))
		interp.hasTime = true
	}
	return interp, true
}

func movementDirectionSourceCanScalarPair(source string) bool {
	if strings.Contains(source, "pair") || strings.Contains(source, "quat") || strings.Contains(source, "vector") {
		return false
	}
	if strings.Contains(source, "deg") && !strings.Contains(source, "integrated") {
		return false
	}
	return strings.Contains(source, "integrated") || strings.Contains(source, "rad")
}

func movementDirectionScalarSourceBase(source string) string {
	base := strings.ReplaceAll(source, "-actor-fused", "")
	base = strings.ReplaceAll(base, "-neg", "")
	return base
}

func movementDirectionDeltaLooksReasonable[T ~float64 | ~int64](delta T, limit float64) bool {
	value := absFloat64(float64(delta))
	return value > 0 && value <= limit
}

func movementDirectionPropProbeForProp(streams []movementDirectionStream, candidates []MovementDirectionCandidate, propID string) *MovementDirectionPropProbe {
	propID = normalizeMovementPropID(propID)
	if propID == "" {
		return nil
	}
	type laneKey struct {
		source       string
		plane        string
		vectorOffset int
	}
	filtered := make([]movementDirectionStream, 0)
	for _, stream := range streams {
		if normalizeMovementPropID(stream.propID) == propID {
			filtered = append(filtered, stream)
		}
	}
	if len(filtered) == 0 {
		return nil
	}
	candidateByKey := map[string]MovementDirectionCandidate{}
	for _, candidate := range candidates {
		if normalizeMovementPropID(candidate.PropID) != propID {
			continue
		}
		candidateByKey[movementDirectionCandidateKey(candidate)] = candidate
	}
	laneGroups := map[laneKey][]movementDirectionStream{}
	for _, stream := range filtered {
		laneGroups[laneKey{
			source:       stream.source,
			plane:        movementDirectionStreamPlane(stream),
			vectorOffset: stream.vectorOffset,
		}] = append(laneGroups[laneKey{
			source:       stream.source,
			plane:        movementDirectionStreamPlane(stream),
			vectorOffset: stream.vectorOffset,
		}], stream)
	}
	out := &MovementDirectionPropProbe{
		PropID: propID,
	}
	actorOrder := make([]MovementDirectionActorSegment, 0, len(filtered))
	for _, stream := range filtered {
		actorOrder = append(actorOrder, movementDirectionActorSegmentForStream(stream))
	}
	sort.Slice(actorOrder, func(i, j int) bool {
		if actorOrder[i].FirstOffset == actorOrder[j].FirstOffset {
			if actorOrder[i].LastOffset == actorOrder[j].LastOffset {
				return actorOrder[i].ActorID < actorOrder[j].ActorID
			}
			return actorOrder[i].LastOffset < actorOrder[j].LastOffset
		}
		return actorOrder[i].FirstOffset < actorOrder[j].FirstOffset
	})
	for index := range actorOrder {
		if index == 0 {
			continue
		}
		actorOrder[index].GapFromPreviousOffset = actorOrder[index].FirstOffset - actorOrder[index-1].LastOffset
	}
	if len(actorOrder) > 24 {
		actorOrder = actorOrder[:24]
	}
	out.ActorOrder = actorOrder
	groupSummaries := make([]MovementDirectionLaneGroup, 0, len(laneGroups))
	for key, groupStreams := range laneGroups {
		group := MovementDirectionLaneGroup{
			Source:       key.source,
			Plane:        key.plane,
			VectorOffset: key.vectorOffset,
			ActorCount:   len(groupStreams),
			Actors:       make([]MovementDirectionActorSegment, 0, len(groupStreams)),
		}
		totalSamples := 0
		for _, stream := range groupStreams {
			totalSamples += len(stream.samples)
			group.Actors = append(group.Actors, movementDirectionActorSegmentForStream(stream))
			if candidate, ok := candidateByKey[movementDirectionStreamCandidateKey(stream)]; ok {
				if group.BestCandidate == nil || movementDirectionCandidateBetter(candidate, *group.BestCandidate) {
					copyCandidate := candidate
					group.BestCandidate = &copyCandidate
				}
			}
		}
		group.TotalSamples = totalSamples
		sort.Slice(group.Actors, func(i, j int) bool {
			if group.Actors[i].SampleCount == group.Actors[j].SampleCount {
				if group.Actors[i].FirstOffset == group.Actors[j].FirstOffset {
					return group.Actors[i].ActorID < group.Actors[j].ActorID
				}
				return group.Actors[i].FirstOffset < group.Actors[j].FirstOffset
			}
			return group.Actors[i].SampleCount > group.Actors[j].SampleCount
		})
		if len(group.Actors) > 8 {
			group.Actors = group.Actors[:8]
		}
		groupSummaries = append(groupSummaries, group)
	}
	sort.Slice(groupSummaries, func(i, j int) bool {
		left := groupSummaries[i]
		right := groupSummaries[j]
		if left.TotalSamples == right.TotalSamples {
			return left.Source < right.Source
		}
		return left.TotalSamples > right.TotalSamples
	})
	if len(groupSummaries) > 24 {
		groupSummaries = groupSummaries[:24]
	}
	out.LaneGroups = groupSummaries
	return out
}

func movementDirectionActorSegmentForStream(stream movementDirectionStream) MovementDirectionActorSegment {
	segment := MovementDirectionActorSegment{
		ActorID:     stream.actorID,
		SampleCount: len(stream.samples),
	}
	if len(stream.samples) == 0 {
		return segment
	}
	first := stream.samples[0]
	last := stream.samples[len(stream.samples)-1]
	segment.FirstOffset = first.offset
	segment.LastOffset = last.offset
	segment.FirstAngle = first.angle
	segment.LastAngle = last.angle
	if first.hasTime {
		value := first.milliseconds
		segment.FirstMilliseconds = &value
	}
	if last.hasTime {
		value := last.milliseconds
		segment.LastMilliseconds = &value
	}
	return segment
}

func movementDirectionStreamPlane(stream movementDirectionStream) string {
	return stream.axisA + "/" + stream.axisB
}

func movementDirectionStreamCandidateKey(stream movementDirectionStream) string {
	return strings.Join([]string{
		stream.source,
		normalizeMovementPropID(stream.propID),
		stream.actorID,
		strconv.Itoa(stream.vectorOffset),
		movementDirectionStreamPlane(stream),
	}, "|")
}

func movementDirectionCandidateKey(candidate MovementDirectionCandidate) string {
	return strings.Join([]string{
		candidate.Source,
		normalizeMovementPropID(candidate.PropID),
		candidate.ActorID,
		strconv.Itoa(candidate.VectorOffset),
		candidate.Plane,
	}, "|")
}

func movementDirectionFloat64PairStreams(header Header, buf []byte, start int, zeroActorPrefixes map[string]int, allowedProps map[string]bool, clockEntries []movementClockEntry) []movementDirectionStream {
	minSamples := movementDirectionMinSampleCount(header)
	type streamKey struct {
		source  string
		propID  string
		actorID string
		offsetA int
		offsetB int
		signA   int
		signB   int
	}
	streams := map[streamKey][]movementDirectionAngleSample{}
	for i := start; i < len(buf); i++ {
		candidate, ok := movementCandidateRecordAtCodeVersion(header.CodeVersion, buf, i, zeroActorPrefixes)
		if !ok || !movementPropPatternAllowed(header.CodeVersion, candidate.prop) {
			continue
		}
		if header.CodeVersion >= Y11S1Alpha3 && movementY11CandidateIsPosition(candidate) {
			continue
		}
		propID := hex.EncodeToString(candidate.prop)
		if len(allowedProps) > 0 && !allowedProps[propID] {
			continue
		}
		actorID := hex.EncodeToString(candidate.actor)
		for relA := 22; relA <= 33; relA++ {
			if i+relA+8 > len(buf) {
				continue
			}
			valueA := math.Float64frombits(binary.LittleEndian.Uint64(buf[i+relA : i+relA+8]))
			if math.IsNaN(valueA) || math.IsInf(valueA, 0) || valueA == 0 || absFloat64(valueA) > 1e6 {
				continue
			}
			for relB := relA + 8; relB <= minInt(relA+16, 41); relB++ {
				if i+relB+8 > len(buf) {
					continue
				}
				valueB := math.Float64frombits(binary.LittleEndian.Uint64(buf[i+relB : i+relB+8]))
				if math.IsNaN(valueB) || math.IsInf(valueB, 0) || valueB == 0 || absFloat64(valueB) > 1e6 {
					continue
				}
				for _, signA := range []int{1, -1} {
					for _, signB := range []int{1, -1} {
						baseAngle := math.Atan2(float64(signB)*valueB, float64(signA)*valueA) * 180 / math.Pi
						for _, variant := range []struct {
							name    string
							degrees float64
						}{
							{name: "f64-pair", degrees: baseAngle},
							{name: "f64-pair-double", degrees: baseAngle * 2},
						} {
							key := streamKey{
								source:  variant.name,
								propID:  propID,
								actorID: actorID,
								offsetA: relA,
								offsetB: relB,
								signA:   signA,
								signB:   signB,
							}
							streams[key] = append(streams[key], movementDirectionAngleSampleAtOffset(i, variant.degrees, clockEntries))
						}
					}
				}
			}
		}
		if header.CodeVersion < Y11S1Alpha3 {
			i += 37
		}
	}
	out := make([]movementDirectionStream, 0, len(streams))
	for key, samples := range streams {
		if len(samples) < minSamples {
			continue
		}
		out = append(out, movementDirectionStream{
			source:       key.source,
			propID:       key.propID,
			actorID:      key.actorID,
			vectorOffset: key.offsetA,
			axisA:        signedAxisName("o"+strconv.Itoa(key.offsetA), float64(key.signA)),
			axisB:        signedAxisName("o"+strconv.Itoa(key.offsetB), float64(key.signB)),
			samples:      samples,
		})
	}
	return out
}

func movementDirectionPropAllowlist(header Header, buf []byte, start int, zeroActorPrefixes map[string]int, actorID string) map[string]bool {
	type propStats struct {
		propID        string
		sampleCount   int
		actorHitCount int
	}
	statsByProp := map[string]*propStats{}
	for i := start; i < len(buf); i++ {
		candidate, ok := movementCandidateRecordAtCodeVersion(header.CodeVersion, buf, i, zeroActorPrefixes)
		if !ok || !movementPropPatternAllowed(header.CodeVersion, candidate.prop) {
			continue
		}
		if header.CodeVersion >= Y11S1Alpha3 && movementY11CandidateIsPosition(candidate) {
			continue
		}
		propID := hex.EncodeToString(candidate.prop)
		stats := statsByProp[propID]
		if stats == nil {
			stats = &propStats{propID: propID}
			statsByProp[propID] = stats
		}
		stats.sampleCount++
		if hex.EncodeToString(candidate.actor) == actorID {
			stats.actorHitCount++
		}
		if header.CodeVersion < Y11S1Alpha3 {
			i += 37
		}
	}
	ordered := make([]propStats, 0, len(statsByProp))
	for _, stats := range statsByProp {
		ordered = append(ordered, *stats)
	}
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].actorHitCount == ordered[j].actorHitCount {
			if ordered[i].sampleCount == ordered[j].sampleCount {
				return ordered[i].propID < ordered[j].propID
			}
			return ordered[i].sampleCount > ordered[j].sampleCount
		}
		return ordered[i].actorHitCount > ordered[j].actorHitCount
	})
	allowed := map[string]bool{}
	minActorHits := 16
	maxActorProps := 0
	maxSampleProps := 6
	if len(header.Players) == 1 {
		minActorHits = 4
		maxActorProps = 24
		maxSampleProps = 24
	}
	actorProps := 0
	for _, stats := range ordered {
		if stats.actorHitCount < minActorHits {
			continue
		}
		allowed[stats.propID] = true
		actorProps++
		if maxActorProps > 0 && actorProps >= maxActorProps {
			break
		}
	}
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].sampleCount == ordered[j].sampleCount {
			return ordered[i].propID < ordered[j].propID
		}
		return ordered[i].sampleCount > ordered[j].sampleCount
	})
	for _, stats := range ordered {
		if len(header.Players) == 1 && stats.sampleCount < 16 {
			continue
		}
		allowed[stats.propID] = true
		if len(allowed) >= maxSampleProps {
			break
		}
	}
	return allowed
}

func movementDirectionCandidateForStream(movementSamples []movementDirectionAngleSample, stream movementDirectionStream, unwrapped []movementDirectionAngleSample, seed MovementDirectionCandidate, minSamples int) (MovementDirectionCandidate, bool) {
	best := seed
	for _, step := range []int{8192, 2048, 512, 128, 32, 8} {
		start := best.ByteShift - (step * 4)
		end := best.ByteShift + (step * 4)
		for shift := start; shift <= end; shift += step {
			candidate, ok := movementDirectionCandidateAtShift(movementSamples, stream, unwrapped, shift, minSamples)
			if !ok {
				continue
			}
			if movementDirectionCandidateBetter(candidate, best) {
				best = candidate
			}
		}
	}
	if candidate, ok := movementDirectionCandidateByProgress(movementSamples, stream, unwrapped, minSamples); ok && movementDirectionCandidateBetter(candidate, best) {
		best = candidate
	}
	if movementDirectionSamplesTimed(movementSamples) && movementDirectionSamplesTimed(unwrapped) {
		if candidate, ok := movementDirectionCandidateByTime(movementSamples, stream, unwrapped, minSamples); ok && movementDirectionCandidateBetter(candidate, best) {
			best = candidate
		}
	}
	best.MakesSense = best.MeanCosineAgreement >= 0.6 && best.MedianErrorDegrees <= 45
	return best, true
}

func movementTrackMovementAngles(track MovementTrack) []movementDirectionAngleSample {
	points := make([]movementDirectionPositionPoint, 0, track.PositionSamples)
	for sampleIndex, sample := range track.Samples {
		if sample.Changed == "position" && sample.Position != nil {
			points = append(points, movementDirectionPositionPoint{
				sampleIndex:   sampleIndex,
				offset:        sample.Offset,
				position:      *sample.Position,
				timeInSeconds: sample.TimeInSeconds,
			})
		}
	}
	if len(points) < 2 {
		return nil
	}
	if movementDirectionTimedPositionPoints(points) {
		return movementTimedTrackMovementAngles(points)
	}
	gap := 4
	if len(points) <= gap {
		gap = 1
	}
	out := make([]movementDirectionAngleSample, 0, len(points)-gap)
	for i := gap; i < len(points); i++ {
		previous := points[i-gap]
		current := points[i]
		dx := float64(current.position.X - previous.position.X)
		dy := float64(current.position.Y - previous.position.Y)
		if math.Hypot(dx, dy) < 0.15 {
			continue
		}
		sample := movementDirectionAngleSample{
			offset:      current.offset,
			sampleIndex: current.sampleIndex,
			angle:       math.Atan2(dy, dx) * 180 / math.Pi,
		}
		if current.timeInSeconds != nil {
			sample.milliseconds = int(math.Round(*current.timeInSeconds * 1000))
			sample.hasTime = true
		}
		out = append(out, sample)
	}
	return out
}

func movementDirectionMinSampleCount(header Header) int {
	if len(header.Players) == 1 {
		return 8
	}
	return 24
}

func movementDirectionAngleSampleAtOffset(offset int, angle float64, clockEntries []movementClockEntry) movementDirectionAngleSample {
	sample := movementDirectionAngleSample{
		offset: offset,
		angle:  angle,
	}
	if milliseconds, ok := movementRecoveredMillisecondsAtOffset(clockEntries, offset); ok {
		sample.milliseconds = milliseconds
		sample.hasTime = true
	}
	return sample
}

func movementAppendDeltaIntegratedDirectionStreams(streams []movementDirectionStream, minSamples int) []movementDirectionStream {
	extra := make([]movementDirectionStream, 0, len(streams))
	for _, stream := range streams {
		if len(stream.samples) < minSamples || strings.Contains(stream.source, "integrated") {
			continue
		}
		deltaIntegrated := movementDirectionDeltaIntegratedSamples(stream.samples)
		if len(deltaIntegrated) < minSamples {
			continue
		}
		extra = append(extra, movementDirectionStream{
			source:       stream.source + "-delta-integrated",
			propID:       stream.propID,
			actorID:      stream.actorID,
			vectorOffset: stream.vectorOffset,
			axisA:        stream.axisA,
			axisB:        stream.axisB,
			samples:      deltaIntegrated,
		})
	}
	return append(streams, extra...)
}

func movementDirectionFinalizeSoloScalarStreams(streams []movementDirectionStream, minSamples int) []movementDirectionStream {
	streams = movementFuseActorSplitDirectionStreams(streams, minSamples)
	streams = movementAppendDeltaIntegratedDirectionStreams(streams, minSamples)
	streams = movementFuseActorSplitDirectionStreams(streams, minSamples)
	return streams
}

func movementDirectionPropFusedDeltaStreams(streams []movementDirectionStream, minSamples int) []movementDirectionStream {
	type groupKey struct {
		source       string
		propID       string
		vectorOffset int
		axisA        string
		axisB        string
	}
	grouped := map[groupKey][]movementDirectionAngleSample{}
	for _, stream := range streams {
		if len(stream.samples) < minSamples || strings.Contains(stream.source, "integrated") || strings.Contains(stream.source, "actor-fused") || strings.Contains(stream.source, "prop-fused") {
			continue
		}
		if !movementDirectionSourceCanPropFuse(stream.source) {
			continue
		}
		key := groupKey{
			source:       stream.source,
			propID:       stream.propID,
			vectorOffset: stream.vectorOffset,
			axisA:        stream.axisA,
			axisB:        stream.axisB,
		}
		grouped[key] = append(grouped[key], movementDirectionCloneAngleSamples(stream.samples)...)
	}
	extra := make([]movementDirectionStream, 0, len(grouped)*2)
	for key, samples := range grouped {
		if len(samples) < minSamples {
			continue
		}
		sort.Slice(samples, func(i, j int) bool {
			if samples[i].offset == samples[j].offset {
				return samples[i].sampleIndex < samples[j].sampleIndex
			}
			return samples[i].offset < samples[j].offset
		})
		merged := make([]movementDirectionAngleSample, 0, len(samples))
		for _, sample := range samples {
			if len(merged) > 0 && merged[len(merged)-1].offset == sample.offset {
				merged[len(merged)-1].angle = (merged[len(merged)-1].angle + sample.angle) / 2
				continue
			}
			merged = append(merged, sample)
		}
		if len(merged) < minSamples {
			continue
		}
		raw := movementDirectionStream{
			source:       key.source + "-prop-fused",
			propID:       key.propID,
			actorID:      "prop-fused",
			vectorOffset: key.vectorOffset,
			axisA:        key.axisA,
			axisB:        key.axisB,
			samples:      merged,
		}
		extra = append(extra, raw)
		deltaIntegrated := movementDirectionDeltaIntegratedSamples(merged)
		if len(deltaIntegrated) >= minSamples {
			extra = append(extra, movementDirectionStream{
				source:       raw.source + "-delta-integrated",
				propID:       key.propID,
				actorID:      "prop-fused",
				vectorOffset: key.vectorOffset,
				axisA:        key.axisA,
				axisB:        key.axisB,
				samples:      deltaIntegrated,
			})
		}
	}
	return extra
}

func movementDirectionSourceCanPropFuse(source string) bool {
	return strings.Contains(source, "s16") || strings.Contains(source, "u16") || strings.Contains(source, "i32-word-delta")
}

func movementDirectionDeltaIntegratedSamples(samples []movementDirectionAngleSample) []movementDirectionAngleSample {
	if len(samples) < 2 {
		return nil
	}
	out := make([]movementDirectionAngleSample, 0, len(samples))
	first := samples[0]
	first.angle = 0
	out = append(out, first)
	cumulative := 0.0
	for index := 1; index < len(samples); index++ {
		delta := movementWrapDegrees(samples[index].angle - samples[index-1].angle)
		cumulative += delta
		sample := samples[index]
		sample.angle = cumulative
		out = append(out, sample)
	}
	return out
}

func movementFuseActorSplitDirectionStreams(streams []movementDirectionStream, minSamples int) []movementDirectionStream {
	type groupKey struct {
		source       string
		propID       string
		vectorOffset int
		axisA        string
		axisB        string
	}
	groups := map[groupKey][]movementDirectionStream{}
	for _, stream := range streams {
		if len(stream.samples) < minSamples || strings.Contains(stream.source, "actor-fused") || !movementDirectionSourceCanActorFuse(stream.source) {
			continue
		}
		key := groupKey{
			source:       stream.source,
			propID:       stream.propID,
			vectorOffset: stream.vectorOffset,
			axisA:        stream.axisA,
			axisB:        stream.axisB,
		}
		groups[key] = append(groups[key], stream)
	}
	extra := make([]movementDirectionStream, 0, len(groups))
	for key, fragments := range groups {
		if len(fragments) < 2 {
			continue
		}
		sort.Slice(fragments, func(i, j int) bool {
			left := fragments[i].samples[0]
			right := fragments[j].samples[0]
			if left.offset == right.offset {
				return fragments[i].actorID < fragments[j].actorID
			}
			return left.offset < right.offset
		})
		wrapped := !strings.Contains(key.source, "integrated")
		fused := movementDirectionCloneAngleSamples(fragments[0].samples)
		usedActors := map[string]bool{fragments[0].actorID: true}
		for index := 1; index < len(fragments); index++ {
			shift, ok := movementDirectionFragmentShift(fused, fragments[index].samples, wrapped)
			if !ok {
				continue
			}
			shifted := movementDirectionShiftAngleSamples(fragments[index].samples, shift, wrapped)
			fused = movementDirectionMergeAngleSamples(fused, shifted, wrapped)
			usedActors[fragments[index].actorID] = true
		}
		if len(usedActors) < 2 || len(fused) < minSamples {
			continue
		}
		extra = append(extra, movementDirectionStream{
			source:       key.source + "-actor-fused",
			propID:       key.propID,
			actorID:      "fused",
			vectorOffset: key.vectorOffset,
			axisA:        key.axisA,
			axisB:        key.axisB,
			samples:      fused,
		})
	}
	return append(streams, extra...)
}

func movementDirectionCloneAngleSamples(samples []movementDirectionAngleSample) []movementDirectionAngleSample {
	out := make([]movementDirectionAngleSample, len(samples))
	copy(out, samples)
	return out
}

func movementDirectionSourceCanActorFuse(source string) bool {
	if strings.Contains(source, "integrated") {
		return true
	}
	return strings.Contains(source, "s16") || strings.Contains(source, "u16") || strings.Contains(source, "pair") || strings.Contains(source, "quat") || strings.Contains(source, "vector")
}

func movementDirectionShiftAngleSamples(samples []movementDirectionAngleSample, shift float64, wrapped bool) []movementDirectionAngleSample {
	out := make([]movementDirectionAngleSample, len(samples))
	for index, sample := range samples {
		sample.angle += shift
		if wrapped {
			sample.angle = movementWrapDegrees(sample.angle)
		}
		out[index] = sample
	}
	return out
}

func movementDirectionFragmentShift(current []movementDirectionAngleSample, next []movementDirectionAngleSample, wrapped bool) (float64, bool) {
	if len(current) == 0 || len(next) == 0 {
		return 0, false
	}
	diffs := make([]float64, 0, len(next))
	for _, sample := range next {
		value, ok := movementInterpolatedAngleAtOffset(current, sample.offset, 32768)
		if !ok {
			continue
		}
		diff := value - sample.angle
		if wrapped {
			diff = movementWrapDegrees(diff)
		}
		diffs = append(diffs, diff)
	}
	if len(diffs) >= 4 {
		if wrapped {
			return movementCircularMeanAngleValues(diffs), true
		}
		return movementMean(diffs), true
	}
	fallback := current[len(current)-1].angle - next[0].angle
	if wrapped {
		fallback = movementWrapDegrees(fallback)
	}
	return fallback, true
}

func movementDirectionMergeAngleSamples(current []movementDirectionAngleSample, next []movementDirectionAngleSample, wrapped bool) []movementDirectionAngleSample {
	type angleAggregate struct {
		sample movementDirectionAngleSample
		count  int
	}
	byOffset := map[int]angleAggregate{}
	for _, sample := range current {
		byOffset[sample.offset] = angleAggregate{sample: sample, count: 1}
	}
	for _, sample := range next {
		entry, ok := byOffset[sample.offset]
		if !ok {
			byOffset[sample.offset] = angleAggregate{sample: sample, count: 1}
			continue
		}
		if wrapped {
			entry.sample.angle = movementCircularMeanAngleValues([]float64{entry.sample.angle, sample.angle})
		} else {
			entry.sample.angle = ((entry.sample.angle * float64(entry.count)) + sample.angle) / float64(entry.count+1)
		}
		entry.count++
		byOffset[sample.offset] = entry
	}
	offsets := make([]int, 0, len(byOffset))
	for offset := range byOffset {
		offsets = append(offsets, offset)
	}
	sort.Ints(offsets)
	out := make([]movementDirectionAngleSample, 0, len(offsets))
	for _, offset := range offsets {
		out = append(out, byOffset[offset].sample)
	}
	return out
}

func movementDirectionTimedPositionPoints(points []movementDirectionPositionPoint) bool {
	for _, point := range points {
		if point.timeInSeconds == nil {
			return false
		}
	}
	return len(points) > 0
}

func movementTimedTrackMovementAngles(points []movementDirectionPositionPoint) []movementDirectionAngleSample {
	const targetGapSeconds = 0.14
	out := make([]movementDirectionAngleSample, 0, len(points))
	for currentIndex := 1; currentIndex < len(points); currentIndex++ {
		current := points[currentIndex]
		bestPrevious := -1
		bestGap := math.Inf(1)
		for previousIndex := currentIndex - 1; previousIndex >= 0; previousIndex-- {
			previous := points[previousIndex]
			gap := *previous.timeInSeconds - *current.timeInSeconds
			if gap < targetGapSeconds {
				continue
			}
			if gap < bestGap {
				bestGap = gap
				bestPrevious = previousIndex
			}
		}
		if bestPrevious == -1 {
			continue
		}
		previous := points[bestPrevious]
		dx := float64(current.position.X - previous.position.X)
		dy := float64(current.position.Y - previous.position.Y)
		if math.Hypot(dx, dy) < 0.15 {
			continue
		}
		sample := movementDirectionAngleSample{
			offset:      current.offset,
			sampleIndex: current.sampleIndex,
			angle:       math.Atan2(dy, dx) * 180 / math.Pi,
		}
		if current.timeInSeconds != nil {
			sample.milliseconds = int(math.Round(*current.timeInSeconds * 1000))
			sample.hasTime = true
		}
		out = append(out, sample)
	}
	return out
}

func movementNearestAngleSampleByOffset(samples []movementDirectionAngleSample, offset int, window int) (movementDirectionAngleSample, bool) {
	bestIndex := -1
	bestDistance := window + 1
	for i, sample := range samples {
		distance := sample.offset - offset
		if distance < 0 {
			distance = -distance
		}
		if distance > window || distance >= bestDistance {
			continue
		}
		bestDistance = distance
		bestIndex = i
	}
	if bestIndex == -1 {
		return movementDirectionAngleSample{}, false
	}
	return samples[bestIndex], true
}

func movementDirectionCandidateAtShift(movementSamples []movementDirectionAngleSample, stream movementDirectionStream, unwrapped []movementDirectionAngleSample, shift int, minSamples int) (MovementDirectionCandidate, bool) {
	best := MovementDirectionCandidate{}
	bestMetric := math.Inf(-1)
	for _, scale := range movementDirectionScaleChoices(stream.source) {
		diffs := make([]float64, 0, len(movementSamples))
		matched := make([]movementDirectionAngleSample, 0, len(movementSamples))
		for _, movement := range movementSamples {
			angle, ok := movementInterpolatedAngleAtOffset(unwrapped, movement.offset+shift, 32768)
			if !ok {
				continue
			}
			angle *= scale
			matched = append(matched, movement)
			diffs = append(diffs, movement.angle-angle)
		}
		if len(diffs) < minSamples {
			continue
		}
		offset := movementCircularMeanAngleValues(diffs)
		errors := make([]float64, 0, len(matched))
		cosineSum := 0.0
		for _, movement := range matched {
			angle, ok := movementInterpolatedAngleAtOffset(unwrapped, movement.offset+shift, 32768)
			if !ok {
				continue
			}
			angle *= scale
			diff := movementWrapDegrees(movement.angle - (angle + offset))
			errors = append(errors, math.Abs(diff))
			cosineSum += math.Cos(diff * math.Pi / 180)
		}
		if len(errors) < minSamples {
			continue
		}
		meanCosine := cosineSum / float64(len(errors))
		candidate := MovementDirectionCandidate{
			Source:               stream.source,
			PropID:               stream.propID,
			ActorID:              stream.actorID,
			VectorOffset:         stream.vectorOffset,
			Plane:                stream.axisA + "/" + stream.axisB,
			Alignment:            "offset",
			ByteShift:            shift,
			AngleScale:           scale,
			HeadingOffsetDegrees: offset,
			SampleCount:          len(errors),
			MeanErrorDegrees:     movementMean(errors),
			MedianErrorDegrees:   movementPercentile(errors, 0.50),
			P90ErrorDegrees:      movementPercentile(errors, 0.90),
			MeanCosineAgreement:  meanCosine,
		}
		metric := candidate.MeanCosineAgreement - (candidate.MeanErrorDegrees / 360)
		if metric > bestMetric || (metric == bestMetric && movementDirectionCandidateBetter(candidate, best)) {
			best = candidate
			bestMetric = metric
		}
	}
	if best.SampleCount == 0 {
		return MovementDirectionCandidate{}, false
	}
	return best, true
}

func movementDirectionCandidateByProgress(movementSamples []movementDirectionAngleSample, stream movementDirectionStream, unwrapped []movementDirectionAngleSample, minSamples int) (MovementDirectionCandidate, bool) {
	best := MovementDirectionCandidate{}
	bestMetric := math.Inf(-1)
	for _, step := range []float64{0.10, 0.025, 0.00625, 0.0015} {
		start := -step * 4
		end := step * 4
		if best.SampleCount > 0 {
			start = progressShift(best.ByteShift, len(movementSamples), len(unwrapped)) - (step * 4)
			end = progressShift(best.ByteShift, len(movementSamples), len(unwrapped)) + (step * 4)
		}
		for shift := start; shift <= end+1e-9; shift += step {
			candidate, metric, ok := movementDirectionCandidateAtProgressShift(movementSamples, stream, unwrapped, shift, minSamples)
			if !ok {
				continue
			}
			if metric > bestMetric || (metric == bestMetric && movementDirectionCandidateBetter(candidate, best)) {
				best = candidate
				bestMetric = metric
			}
		}
	}
	if best.SampleCount == 0 {
		return MovementDirectionCandidate{}, false
	}
	return best, true
}

func movementDirectionCandidateByTime(movementSamples []movementDirectionAngleSample, stream movementDirectionStream, unwrapped []movementDirectionAngleSample, minSamples int) (MovementDirectionCandidate, bool) {
	if !movementDirectionTimeSeriesLooksUsable(unwrapped) {
		return MovementDirectionCandidate{}, false
	}
	best := MovementDirectionCandidate{}
	bestMetric := math.Inf(-1)
	for _, step := range []int{1200, 300, 80, 20, 5} {
		start := -step * 4
		end := step * 4
		if best.SampleCount > 0 {
			start = best.TimeShiftMilliseconds - (step * 4)
			end = best.TimeShiftMilliseconds + (step * 4)
		}
		for shift := start; shift <= end; shift += step {
			candidate, ok := movementDirectionCandidateAtTimeShift(movementSamples, stream, unwrapped, shift, minSamples)
			if !ok {
				continue
			}
			metric := candidate.MeanCosineAgreement - (candidate.MeanErrorDegrees / 360) + (float64(candidate.SampleCount) / float64(maxInt(len(movementSamples), 1)))
			if metric > bestMetric || (metric == bestMetric && movementDirectionCandidateBetter(candidate, best)) {
				best = candidate
				bestMetric = metric
			}
		}
	}
	if best.SampleCount == 0 {
		return MovementDirectionCandidate{}, false
	}
	return best, true
}

func movementDirectionTimeSeriesLooksUsable(samples []movementDirectionAngleSample) bool {
	if len(samples) < 8 {
		return false
	}
	distinct := map[int]bool{}
	minMilliseconds := 0
	maxMilliseconds := 0
	hasTime := false
	for _, sample := range samples {
		if !sample.hasTime {
			continue
		}
		distinct[sample.milliseconds] = true
		if !hasTime {
			minMilliseconds = sample.milliseconds
			maxMilliseconds = sample.milliseconds
			hasTime = true
			continue
		}
		if sample.milliseconds < minMilliseconds {
			minMilliseconds = sample.milliseconds
		}
		if sample.milliseconds > maxMilliseconds {
			maxMilliseconds = sample.milliseconds
		}
	}
	if !hasTime {
		return false
	}
	distinctCount := len(distinct)
	span := maxMilliseconds - minMilliseconds
	if distinctCount < maxInt(8, len(samples)/16) {
		return false
	}
	if len(samples) >= 64 && span < 250 {
		return false
	}
	return true
}

func movementDirectionCandidateAtProgressShift(movementSamples []movementDirectionAngleSample, stream movementDirectionStream, unwrapped []movementDirectionAngleSample, shift float64, minSamples int) (MovementDirectionCandidate, float64, bool) {
	best := MovementDirectionCandidate{}
	bestMetric := math.Inf(-1)
	movementDenom := maxInt(len(movementSamples)-1, 1)
	for _, scale := range movementDirectionScaleChoices(stream.source) {
		diffs := make([]float64, 0, len(movementSamples))
		matched := 0
		for index, movement := range movementSamples {
			progress := float64(index)/float64(movementDenom) + shift
			angle, ok := movementInterpolatedAngleAtProgress(unwrapped, progress)
			if !ok {
				continue
			}
			angle *= scale
			diffs = append(diffs, movement.angle-angle)
			matched++
		}
		if matched < minSamples {
			continue
		}
		offset := movementCircularMeanAngleValues(diffs)
		errors := make([]float64, 0, matched)
		cosineSum := 0.0
		for index, movement := range movementSamples {
			progress := float64(index)/float64(movementDenom) + shift
			angle, ok := movementInterpolatedAngleAtProgress(unwrapped, progress)
			if !ok {
				continue
			}
			angle *= scale
			diff := movementWrapDegrees(movement.angle - (angle + offset))
			errors = append(errors, math.Abs(diff))
			cosineSum += math.Cos(diff * math.Pi / 180)
		}
		if len(errors) < minSamples {
			continue
		}
		meanCosine := cosineSum / float64(len(errors))
		coverage := float64(len(errors)) / float64(len(movementSamples))
		metric := meanCosine - (movementMean(errors) / 360) + (coverage * 0.2)
		candidate := MovementDirectionCandidate{
			Source:               stream.source,
			PropID:               stream.propID,
			ActorID:              stream.actorID,
			VectorOffset:         stream.vectorOffset,
			Plane:                stream.axisA + "/" + stream.axisB,
			Alignment:            "progress",
			ByteShift:            progressShiftToByteShift(shift, len(movementSamples), len(unwrapped)),
			AngleScale:           scale,
			HeadingOffsetDegrees: offset,
			SampleCount:          len(errors),
			MeanErrorDegrees:     movementMean(errors),
			MedianErrorDegrees:   movementPercentile(errors, 0.50),
			P90ErrorDegrees:      movementPercentile(errors, 0.90),
			MeanCosineAgreement:  meanCosine,
		}
		if metric > bestMetric || (metric == bestMetric && movementDirectionCandidateBetter(candidate, best)) {
			best = candidate
			bestMetric = metric
		}
	}
	if best.SampleCount == 0 {
		return MovementDirectionCandidate{}, 0, false
	}
	return best, bestMetric, true
}

func movementDirectionCandidateAtTimeShift(movementSamples []movementDirectionAngleSample, stream movementDirectionStream, unwrapped []movementDirectionAngleSample, shiftMilliseconds int, minSamples int) (MovementDirectionCandidate, bool) {
	best := MovementDirectionCandidate{}
	bestMetric := math.Inf(-1)
	for _, scale := range movementDirectionScaleChoices(stream.source) {
		diffs := make([]float64, 0, len(movementSamples))
		matched := make([]movementDirectionAngleSample, 0, len(movementSamples))
		for _, movement := range movementSamples {
			if !movement.hasTime {
				continue
			}
			angle, ok := movementInterpolatedAngleAtMilliseconds(unwrapped, movement.milliseconds+shiftMilliseconds, 1200)
			if !ok {
				continue
			}
			angle *= scale
			matched = append(matched, movement)
			diffs = append(diffs, movement.angle-angle)
		}
		if len(diffs) < minSamples {
			continue
		}
		offset := movementCircularMeanAngleValues(diffs)
		errors := make([]float64, 0, len(matched))
		cosineSum := 0.0
		for _, movement := range matched {
			angle, ok := movementInterpolatedAngleAtMilliseconds(unwrapped, movement.milliseconds+shiftMilliseconds, 1200)
			if !ok {
				continue
			}
			angle *= scale
			diff := movementWrapDegrees(movement.angle - (angle + offset))
			errors = append(errors, math.Abs(diff))
			cosineSum += math.Cos(diff * math.Pi / 180)
		}
		if len(errors) < minSamples {
			continue
		}
		meanCosine := cosineSum / float64(len(errors))
		candidate := MovementDirectionCandidate{
			Source:                stream.source,
			PropID:                stream.propID,
			ActorID:               stream.actorID,
			VectorOffset:          stream.vectorOffset,
			Plane:                 stream.axisA + "/" + stream.axisB,
			Alignment:             "time",
			TimeShiftMilliseconds: shiftMilliseconds,
			AngleScale:            scale,
			HeadingOffsetDegrees:  offset,
			SampleCount:           len(errors),
			MeanErrorDegrees:      movementMean(errors),
			MedianErrorDegrees:    movementPercentile(errors, 0.50),
			P90ErrorDegrees:       movementPercentile(errors, 0.90),
			MeanCosineAgreement:   meanCosine,
		}
		metric := candidate.MeanCosineAgreement - (candidate.MeanErrorDegrees / 360)
		if metric > bestMetric || (metric == bestMetric && movementDirectionCandidateBetter(candidate, best)) {
			best = candidate
			bestMetric = metric
		}
	}
	if best.SampleCount == 0 {
		return MovementDirectionCandidate{}, false
	}
	return best, true
}

func movementInterpolatedAngleAtProgress(samples []movementDirectionAngleSample, progress float64) (float64, bool) {
	if len(samples) == 0 || progress < 0 || progress > 1 {
		return 0, false
	}
	if len(samples) == 1 {
		return samples[0].angle, true
	}
	scaled := progress * float64(len(samples)-1)
	index := int(math.Floor(scaled))
	if index >= len(samples)-1 {
		return samples[len(samples)-1].angle, true
	}
	ratio := scaled - float64(index)
	left := samples[index].angle
	right := samples[index+1].angle
	return left + ((right - left) * ratio), true
}

func movementInterpolatedAngleAtMilliseconds(samples []movementDirectionAngleSample, milliseconds int, window int) (float64, bool) {
	if len(samples) == 0 {
		return 0, false
	}
	index := sort.Search(len(samples), func(i int) bool {
		return samples[i].milliseconds >= milliseconds
	})
	if index == 0 {
		if !samples[0].hasTime || samples[0].milliseconds-milliseconds > window {
			return 0, false
		}
		return samples[0].angle, true
	}
	if index >= len(samples) {
		last := samples[len(samples)-1]
		if !last.hasTime || milliseconds-last.milliseconds > window {
			return 0, false
		}
		return last.angle, true
	}
	left := samples[index-1]
	right := samples[index]
	if !left.hasTime || !right.hasTime {
		return 0, false
	}
	if milliseconds-left.milliseconds > window && right.milliseconds-milliseconds > window {
		return 0, false
	}
	if left.milliseconds == right.milliseconds {
		return left.angle, true
	}
	if milliseconds-left.milliseconds > window {
		return right.angle, true
	}
	if right.milliseconds-milliseconds > window {
		return left.angle, true
	}
	ratio := float64(milliseconds-left.milliseconds) / float64(right.milliseconds-left.milliseconds)
	return left.angle + ((right.angle - left.angle) * ratio), true
}

func progressShift(byteShift int, movementCount int, streamCount int) float64 {
	if movementCount <= 1 || streamCount <= 1 {
		return 0
	}
	return float64(byteShift) / float64(maxInt(movementCount, streamCount)*512)
}

func progressShiftToByteShift(shift float64, movementCount int, streamCount int) int {
	return int(math.Round(shift * float64(maxInt(movementCount, streamCount)*512)))
}

func movementDirectionCandidateBetter(left MovementDirectionCandidate, right MovementDirectionCandidate) bool {
	leftMetric := left.MeanCosineAgreement - (left.MeanErrorDegrees / 360)
	rightMetric := right.MeanCosineAgreement - (right.MeanErrorDegrees / 360)
	if leftMetric == rightMetric {
		if left.MedianErrorDegrees == right.MedianErrorDegrees {
			return left.SampleCount > right.SampleCount
		}
		return left.MedianErrorDegrees < right.MedianErrorDegrees
	}
	return leftMetric > rightMetric
}

func movementDirectionShortlistScored(scored []movementDirectionScoredStream, solo bool, maxScored int) []movementDirectionScoredStream {
	metricTop := append([]movementDirectionScoredStream(nil), scored...)
	coverageTop := append([]movementDirectionScoredStream(nil), scored...)
	sort.Slice(coverageTop, func(i, j int) bool {
		if coverageTop[i].rawCount == coverageTop[j].rawCount {
			return movementDirectionCandidateBetter(coverageTop[i].candidate, coverageTop[j].candidate)
		}
		return coverageTop[i].rawCount > coverageTop[j].rawCount
	})
	keep := make([]movementDirectionScoredStream, 0, maxScored)
	seenStreams := map[string]bool{}
	seenActors := map[string]int{}
	streamKey := func(item movementDirectionScoredStream) string {
		return item.candidate.Source + "|" + item.candidate.PropID + "|" + item.candidate.ActorID + "|" + item.candidate.Plane + "|" + strconv.Itoa(item.candidate.VectorOffset)
	}
	actorKey := func(item movementDirectionScoredStream) string {
		return item.candidate.PropID + "|" + item.candidate.ActorID
	}
	add := func(item movementDirectionScoredStream) bool {
		key := streamKey(item)
		if seenStreams[key] {
			return false
		}
		seenStreams[key] = true
		seenActors[actorKey(item)]++
		keep = append(keep, item)
		return true
	}
	metricQuota := 32
	if solo {
		metricQuota = 24
	}
	for _, item := range metricTop {
		if add(item) && len(keep) >= metricQuota {
			break
		}
	}
	if solo {
		actorQuota := minInt(maxScored, metricQuota+48)
		for _, item := range metricTop {
			if len(keep) >= actorQuota {
				break
			}
			if seenActors[actorKey(item)] >= 4 {
				continue
			}
			add(item)
		}
	}
	for _, item := range coverageTop {
		if len(keep) >= maxScored {
			break
		}
		if solo && seenActors[actorKey(item)] >= 6 {
			continue
		}
		add(item)
	}
	return keep
}

func movementBestDenseDirectionCandidate(candidates []MovementDirectionCandidate, movementCount int) *MovementDirectionCandidate {
	if len(candidates) == 0 || movementCount == 0 {
		return nil
	}
	bestIndex := -1
	bestMetric := math.Inf(-1)
	for i, candidate := range candidates {
		coverage := float64(candidate.SampleCount) / float64(movementCount)
		metric := candidate.MeanCosineAgreement - (candidate.MeanErrorDegrees / 360) + (coverage * 0.35)
		if coverage < 0.10 {
			metric -= 1
		}
		if metric <= bestMetric {
			continue
		}
		bestMetric = metric
		bestIndex = i
	}
	if bestIndex == -1 {
		return nil
	}
	best := candidates[bestIndex]
	best.MakesSense = best.MeanCosineAgreement >= 0.6 && best.MedianErrorDegrees <= 45 && best.SampleCount >= maxInt(96, movementCount/8)
	return &best
}

func movementBestSameActorDirectionCandidate(candidates []MovementDirectionCandidate, actorID string, minSamples int) *MovementDirectionCandidate {
	bestIndex := -1
	bestMetric := math.Inf(-1)
	if minSamples < 8 {
		minSamples = 8
	}
	for i, candidate := range candidates {
		if candidate.ActorID != actorID {
			continue
		}
		if candidate.SampleCount < minSamples {
			continue
		}
		metric := candidate.MeanCosineAgreement - (candidate.MeanErrorDegrees / 360) + (float64(candidate.SampleCount) / 500)
		if metric <= bestMetric {
			continue
		}
		bestIndex = i
		bestMetric = metric
	}
	if bestIndex == -1 {
		return nil
	}
	best := candidates[bestIndex]
	best.MakesSense = best.MeanCosineAgreement >= 0.6 && best.MedianErrorDegrees <= 45 && best.SampleCount >= minSamples
	return &best
}

func movementDirectionPredictionsForCandidate(movementSamples []movementDirectionAngleSample, stream movementDirectionStream, unwrapped []movementDirectionAngleSample, candidate MovementDirectionCandidate) ([]movementDirectionPrediction, map[string]bool) {
	packetKeys := map[string]bool{}
	for _, sample := range stream.samples {
		packetKeys[movementPacketKey(stream.propID, stream.actorID, sample.offset)] = true
	}
	predictions := make([]movementDirectionPrediction, 0, candidate.SampleCount)
	switch candidate.Alignment {
	case "time":
		for _, movement := range movementSamples {
			if !movement.hasTime {
				continue
			}
			angle, ok := movementInterpolatedAngleAtMilliseconds(unwrapped, movement.milliseconds+candidate.TimeShiftMilliseconds, 1200)
			if !ok {
				continue
			}
			predictions = append(predictions, movementDirectionPrediction{
				sampleIndex:     movement.sampleIndex,
				offset:          movement.offset,
				movementDegrees: movement.angle,
				viewingDegrees:  movementWrapDegrees((angle * candidate.AngleScale) + candidate.HeadingOffsetDegrees),
			})
		}
	case "progress":
		progress := progressShift(candidate.ByteShift, len(movementSamples), len(unwrapped))
		movementDenom := maxInt(len(movementSamples)-1, 1)
		for index, movement := range movementSamples {
			angle, ok := movementInterpolatedAngleAtProgress(unwrapped, (float64(index)/float64(movementDenom))+progress)
			if !ok {
				continue
			}
			predictions = append(predictions, movementDirectionPrediction{
				sampleIndex:     movement.sampleIndex,
				offset:          movement.offset,
				movementDegrees: movement.angle,
				viewingDegrees:  movementWrapDegrees((angle * candidate.AngleScale) + candidate.HeadingOffsetDegrees),
			})
		}
	default:
		for _, movement := range movementSamples {
			angle, ok := movementInterpolatedAngleAtOffset(unwrapped, movement.offset+candidate.ByteShift, 32768)
			if !ok {
				continue
			}
			predictions = append(predictions, movementDirectionPrediction{
				sampleIndex:     movement.sampleIndex,
				offset:          movement.offset,
				movementDegrees: movement.angle,
				viewingDegrees:  movementWrapDegrees((angle * candidate.AngleScale) + candidate.HeadingOffsetDegrees),
			})
		}
	}
	return predictions, packetKeys
}

func movementDirectionSamplesTimed(samples []movementDirectionAngleSample) bool {
	for _, sample := range samples {
		if !sample.hasTime {
			return false
		}
	}
	return len(samples) > 0
}

type movementDirectionFusedMetrics struct {
	sampleCount         int
	coveragePercent     float64
	meanErrorDegrees    float64
	medianErrorDegrees  float64
	p90ErrorDegrees     float64
	meanCosineAgreement float64
	score               float64
	makesSense          bool
}

func movementBestFusedDirectionCandidate(movementSamples []movementDirectionAngleSample, evals []movementDirectionCandidateEval, actorID string, minSamples int) (*MovementDirectionFusedTrack, map[int]float64, map[string]bool, map[string]bool) {
	if len(movementSamples) == 0 || len(evals) == 0 {
		return nil, nil, nil, nil
	}
	filtered := make([]movementDirectionCandidateEval, 0, len(evals))
	seen := map[string]bool{}
	for _, eval := range evals {
		if eval.candidate.SampleCount < minSamples {
			continue
		}
		if eval.candidate.MeanCosineAgreement < 0.85 {
			continue
		}
		if eval.candidate.MedianErrorDegrees > 32 {
			continue
		}
		filtered = append(filtered, eval)
		seen[movementDirectionCandidateEvalKey(eval.candidate)] = true
	}
	if len(movementSamples) >= 512 {
		denseMinSamples := maxInt(256, len(movementSamples)/12)
		for _, eval := range evals {
			if seen[movementDirectionCandidateEvalKey(eval.candidate)] {
				continue
			}
			if !strings.Contains(eval.candidate.Source, "integrated") {
				continue
			}
			if eval.candidate.SampleCount < denseMinSamples {
				continue
			}
			if eval.candidate.MeanCosineAgreement < 0.30 {
				continue
			}
			if eval.candidate.MedianErrorDegrees > 80 {
				continue
			}
			filtered = append(filtered, eval)
			seen[movementDirectionCandidateEvalKey(eval.candidate)] = true
		}
	}
	if len(filtered) == 0 {
		for _, eval := range evals {
			if !eval.candidate.MakesSense || eval.candidate.SampleCount < minSamples {
				continue
			}
			filtered = append(filtered, eval)
		}
	}
	if len(filtered) == 0 {
		return nil, nil, nil, nil
	}
	sort.Slice(filtered, func(i, j int) bool {
		left := movementDirectionFusionSeedScore(filtered[i].candidate, actorID, len(movementSamples))
		right := movementDirectionFusionSeedScore(filtered[j].candidate, actorID, len(movementSamples))
		if left == right {
			return movementDirectionCandidateBetter(filtered[i].candidate, filtered[j].candidate)
		}
		return left > right
	})
	bestMetrics := movementDirectionFusedMetrics{}
	bestPredictions := map[int]float64{}
	bestEvalIndices := []int(nil)
	for seedIndex, seed := range filtered {
		current := movementDirectionPredictionMap(seed.prediction)
		currentEvals := []int{seedIndex}
		currentMetrics := movementDirectionFusedPredictionMetrics(movementSamples, current)
		improved := true
		for improved {
			improved = false
			bestAddIndex := -1
			bestAddPredictions := map[int]float64(nil)
			bestAddMetrics := currentMetrics
			for candidateIndex, candidate := range filtered {
				if movementDirectionFusedContains(currentEvals, candidateIndex) {
					continue
				}
				if !movementDirectionCanMergePredictions(current, candidate.prediction) {
					continue
				}
				merged := movementDirectionMergePredictions(current, candidate.prediction)
				metrics := movementDirectionFusedPredictionMetrics(movementSamples, merged)
				if !movementDirectionFusionImproves(currentMetrics, metrics) {
					continue
				}
				bestAddIndex = candidateIndex
				bestAddPredictions = merged
				bestAddMetrics = metrics
			}
			if bestAddIndex != -1 {
				current = bestAddPredictions
				currentMetrics = bestAddMetrics
				currentEvals = append(currentEvals, bestAddIndex)
				improved = true
			}
		}
		if movementDirectionFusionImproves(bestMetrics, currentMetrics) {
			bestMetrics = currentMetrics
			bestPredictions = current
			bestEvalIndices = append([]int(nil), currentEvals...)
		}
	}
	if len(bestPredictions) == 0 || len(bestEvalIndices) == 0 {
		return nil, nil, nil, nil
	}
	packetKeys := map[string]bool{}
	actors := map[string]bool{}
	props := map[string]bool{}
	sources := map[string]bool{}
	planes := map[string]bool{}
	alignments := map[string]bool{}
	offsets := map[int]bool{}
	for _, index := range bestEvalIndices {
		eval := filtered[index]
		props[eval.candidate.PropID] = true
		actors[eval.candidate.ActorID] = true
		sources[eval.candidate.Source] = true
		planes[eval.candidate.Plane] = true
		alignments[eval.candidate.Alignment] = true
		offsets[eval.candidate.VectorOffset] = true
		for key := range eval.packetKeys {
			packetKeys[key] = true
		}
	}
	return &MovementDirectionFusedTrack{
			Source:              movementDirectionJoinKeys(sources),
			PropIDs:             movementDirectionSortedKeys(props),
			ActorIDs:            movementDirectionSortedKeys(actors),
			VectorOffsets:       movementDirectionSortedInts(offsets),
			Planes:              movementDirectionSortedKeys(planes),
			AlignmentModes:      movementDirectionSortedKeys(alignments),
			CandidateCount:      len(bestEvalIndices),
			CandidateKeys:       movementDirectionEvalKeys(filtered, bestEvalIndices),
			UsedPacketCount:     len(packetKeys),
			SampleCount:         bestMetrics.sampleCount,
			CoveragePercent:     bestMetrics.coveragePercent,
			MeanErrorDegrees:    bestMetrics.meanErrorDegrees,
			MedianErrorDegrees:  bestMetrics.medianErrorDegrees,
			P90ErrorDegrees:     bestMetrics.p90ErrorDegrees,
			MeanCosineAgreement: bestMetrics.meanCosineAgreement,
			MakesSense:          bestMetrics.makesSense,
		},
		bestPredictions,
		packetKeys,
		actors
}

func movementBestDenseFallbackDirectionCandidate(movementSamples []movementDirectionAngleSample, evals []movementDirectionCandidateEval, bestDense *MovementDirectionCandidate, bestSameActor *MovementDirectionCandidate) (*MovementDirectionFusedTrack, map[int]float64, map[string]bool, map[string]bool) {
	if len(movementSamples) == 0 || bestDense == nil {
		return nil, nil, nil, nil
	}
	denseEval, ok := movementDirectionFindEval(evals, *bestDense)
	if !ok {
		return nil, nil, nil, nil
	}
	currentPredictions := denseEval.prediction
	currentPackets := movementClonePacketKeys(denseEval.packetKeys)
	currentActors := map[string]bool{denseEval.candidate.ActorID: true}
	if bestSameActor != nil {
		if sameEval, ok := movementDirectionFindEval(evals, *bestSameActor); ok {
			if shift, overlap := movementDirectionPredictionAlignmentShift(currentPredictions, sameEval.prediction); overlap >= 6 {
				currentPredictions = movementDirectionShiftPredictions(currentPredictions, shift)
			}
			currentPredictions = movementDirectionMergePredictionsList(currentPredictions, sameEval.prediction)
			for key := range sameEval.packetKeys {
				currentPackets[key] = true
			}
			currentActors[sameEval.candidate.ActorID] = true
		}
	}
	fused := movementDirectionPredictionMap(currentPredictions)
	metrics := movementDirectionFusedPredictionMetrics(movementSamples, fused)
	if metrics.sampleCount == 0 {
		return nil, nil, nil, nil
	}
	if metrics.coveragePercent < 6 || metrics.meanCosineAgreement < 0.45 || metrics.medianErrorDegrees > 80 {
		return nil, nil, nil, nil
	}
	sources := map[string]bool{denseEval.candidate.Source + "+dense-fallback": true}
	props := map[string]bool{denseEval.candidate.PropID: true}
	offsets := map[int]bool{denseEval.candidate.VectorOffset: true}
	planes := map[string]bool{denseEval.candidate.Plane: true}
	alignments := map[string]bool{denseEval.candidate.Alignment: true}
	count := 1
	if bestSameActor != nil {
		sources[bestSameActor.Source] = true
		props[bestSameActor.PropID] = true
		offsets[bestSameActor.VectorOffset] = true
		planes[bestSameActor.Plane] = true
		alignments[bestSameActor.Alignment] = true
		count = 2
	}
	return &MovementDirectionFusedTrack{
			Source:              movementDirectionJoinKeys(sources),
			PropIDs:             movementDirectionSortedKeys(props),
			ActorIDs:            movementDirectionSortedKeys(currentActors),
			VectorOffsets:       movementDirectionSortedInts(offsets),
			Planes:              movementDirectionSortedKeys(planes),
			AlignmentModes:      movementDirectionSortedKeys(alignments),
			CandidateCount:      count,
			CandidateKeys:       movementDirectionCandidateKeys(bestDense, bestSameActor),
			UsedPacketCount:     len(currentPackets),
			SampleCount:         metrics.sampleCount,
			CoveragePercent:     metrics.coveragePercent,
			MeanErrorDegrees:    metrics.meanErrorDegrees,
			MedianErrorDegrees:  metrics.medianErrorDegrees,
			P90ErrorDegrees:     metrics.p90ErrorDegrees,
			MeanCosineAgreement: metrics.meanCosineAgreement,
			MakesSense:          metrics.makesSense,
		},
		fused,
		currentPackets,
		currentActors
}

func movementBestDenseLocalFragmentDirectionCandidate(movementSamples []movementDirectionAngleSample, evals []movementDirectionCandidateEval, bestDense *MovementDirectionCandidate, bestSameActor *MovementDirectionCandidate) (*MovementDirectionFusedTrack, map[int]float64, map[string]bool, map[string]bool) {
	if len(movementSamples) < 32 || bestDense == nil {
		return nil, nil, nil, nil
	}
	type localEval struct {
		eval         movementDirectionCandidateEval
		predByIndex  map[int]float64
		localScore   map[int]float64
		localCount   map[int]int
		packetKeys   map[string]bool
		globalWeight float64
	}
	denseEvals := make([]movementDirectionCandidateEval, 0, len(evals))
	for _, eval := range evals {
		if eval.candidate.PropID != bestDense.PropID {
			continue
		}
		if eval.candidate.SampleCount < 16 {
			continue
		}
		if eval.candidate.MeanCosineAgreement < 0.10 {
			continue
		}
		denseEvals = append(denseEvals, eval)
	}
	if len(denseEvals) == 0 {
		return nil, nil, nil, nil
	}
	sort.Slice(denseEvals, func(i, j int) bool {
		left := movementDirectionDenseLocalSeedScore(denseEvals[i].candidate)
		right := movementDirectionDenseLocalSeedScore(denseEvals[j].candidate)
		if left == right {
			return movementDirectionCandidateBetter(denseEvals[i].candidate, denseEvals[j].candidate)
		}
		return left > right
	})
	if len(denseEvals) > 192 {
		denseEvals = denseEvals[:192]
	}
	movementOrderByIndex := make(map[int]int, len(movementSamples))
	for order, movement := range movementSamples {
		movementOrderByIndex[movement.sampleIndex] = order
	}
	windowRadius := 12
	prepared := make([]localEval, 0, len(denseEvals))
	for _, eval := range denseEvals {
		predByIndex := movementDirectionPredictionMap(eval.prediction)
		if len(predByIndex) < 16 {
			continue
		}
		localScore := map[int]float64{}
		localCount := map[int]int{}
		for _, prediction := range eval.prediction {
			order, ok := movementOrderByIndex[prediction.sampleIndex]
			if !ok {
				continue
			}
			left := maxInt(0, order-windowRadius)
			right := minInt(len(movementSamples)-1, order+windowRadius)
			cosineSum := 0.0
			errorSum := 0.0
			count := 0
			for sampleOrder := left; sampleOrder <= right; sampleOrder++ {
				movement := movementSamples[sampleOrder]
				angle, ok := predByIndex[movement.sampleIndex]
				if !ok {
					continue
				}
				diff := movementWrapDegrees(movement.angle - angle)
				errorSum += math.Abs(diff)
				cosineSum += math.Cos(diff * math.Pi / 180)
				count++
			}
			if count < 6 {
				continue
			}
			meanError := errorSum / float64(count)
			localScore[order] = (cosineSum / float64(count)) - (meanError / 360)
			localCount[order] = count
		}
		if len(localScore) == 0 {
			continue
		}
		prepared = append(prepared, localEval{
			eval:         eval,
			predByIndex:  predByIndex,
			localScore:   localScore,
			localCount:   localCount,
			packetKeys:   movementClonePacketKeys(eval.packetKeys),
			globalWeight: movementDirectionDenseLocalSeedScore(eval.candidate),
		})
	}
	if len(prepared) == 0 {
		return nil, nil, nil, nil
	}
	derived := map[int]float64{}
	packetKeys := map[string]bool{}
	actors := map[string]bool{}
	props := map[string]bool{}
	sources := map[string]bool{}
	planes := map[string]bool{}
	alignments := map[string]bool{}
	offsets := map[int]bool{}
	usedCandidates := map[string]bool{}
	lastEvalKey := ""
	lastAngle := 0.0
	hasLast := false
	for order, movement := range movementSamples {
		bestIndex := -1
		bestScore := 0.0
		bestAngle := 0.0
		for index, candidate := range prepared {
			angle, ok := candidate.predByIndex[movement.sampleIndex]
			if !ok {
				continue
			}
			score, ok := candidate.localScore[order]
			if !ok {
				continue
			}
			score += candidate.globalWeight * 0.05
			if candidate.localCount[order] >= 10 {
				score += 0.04
			}
			key := movementDirectionCandidateEvalKey(candidate.eval.candidate)
			if key == lastEvalKey {
				score += 0.06
			}
			if hasLast {
				jump := math.Abs(movementWrapDegrees(angle - lastAngle))
				score -= jump / 720
			}
			if bestIndex == -1 || score > bestScore {
				bestIndex = index
				bestScore = score
				bestAngle = angle
			}
		}
		if bestIndex == -1 || bestScore < 0.20 {
			continue
		}
		chosen := prepared[bestIndex]
		derived[movement.sampleIndex] = bestAngle
		for key := range chosen.packetKeys {
			packetKeys[key] = true
		}
		actors[chosen.eval.candidate.ActorID] = true
		props[chosen.eval.candidate.PropID] = true
		sources[chosen.eval.candidate.Source+"+dense-local"] = true
		planes[chosen.eval.candidate.Plane] = true
		alignments[chosen.eval.candidate.Alignment] = true
		offsets[chosen.eval.candidate.VectorOffset] = true
		usedCandidates[movementDirectionCandidateEvalKey(chosen.eval.candidate)] = true
		lastEvalKey = movementDirectionCandidateEvalKey(chosen.eval.candidate)
		lastAngle = bestAngle
		hasLast = true
	}
	if len(derived) == 0 {
		return nil, nil, nil, nil
	}
	movementDirectionInterpolateDerivedHeadingGaps(movementSamples, derived, 8)
	metrics := movementDirectionFusedPredictionMetrics(movementSamples, derived)
	if metrics.sampleCount == 0 {
		return nil, nil, nil, nil
	}
	if metrics.coveragePercent < 20 || metrics.meanCosineAgreement < 0.60 || metrics.medianErrorDegrees > 40 {
		return nil, nil, nil, nil
	}
	if bestSameActor != nil {
		if sameEval, ok := movementDirectionFindEval(evals, *bestSameActor); ok {
			sources[bestSameActor.Source] = true
			props[bestSameActor.PropID] = true
			actors[bestSameActor.ActorID] = true
			planes[bestSameActor.Plane] = true
			alignments[bestSameActor.Alignment] = true
			offsets[bestSameActor.VectorOffset] = true
			for key := range sameEval.packetKeys {
				packetKeys[key] = true
			}
		}
	}
	return &MovementDirectionFusedTrack{
			Source:              movementDirectionJoinKeys(sources),
			PropIDs:             movementDirectionSortedKeys(props),
			ActorIDs:            movementDirectionSortedKeys(actors),
			VectorOffsets:       movementDirectionSortedInts(offsets),
			Planes:              movementDirectionSortedKeys(planes),
			AlignmentModes:      movementDirectionSortedKeys(alignments),
			CandidateCount:      len(usedCandidates),
			CandidateKeys:       movementDirectionSortedKeys(usedCandidates),
			UsedPacketCount:     len(packetKeys),
			SampleCount:         metrics.sampleCount,
			CoveragePercent:     metrics.coveragePercent,
			MeanErrorDegrees:    metrics.meanErrorDegrees,
			MedianErrorDegrees:  metrics.medianErrorDegrees,
			P90ErrorDegrees:     metrics.p90ErrorDegrees,
			MeanCosineAgreement: metrics.meanCosineAgreement,
			MakesSense:          metrics.makesSense,
		},
		derived,
		packetKeys,
		actors
}

func movementDirectionDenseLocalSeedScore(candidate MovementDirectionCandidate) float64 {
	score := candidate.MeanCosineAgreement
	score += float64(candidate.SampleCount) / 1200
	score -= candidate.MeanErrorDegrees / 720
	if strings.Contains(candidate.Source, "integrated") {
		score += 0.08
	}
	if strings.Contains(candidate.Source, "pair") {
		score += 0.04
	}
	return score
}

func movementDirectionInterpolateDerivedHeadingGaps(movementSamples []movementDirectionAngleSample, derived map[int]float64, maxGap int) {
	if len(derived) < 2 {
		return
	}
	lastKnownOrder := -1
	lastKnownAngle := 0.0
	for order, movement := range movementSamples {
		currentAngle, ok := derived[movement.sampleIndex]
		if !ok {
			continue
		}
		if lastKnownOrder != -1 {
			gap := order - lastKnownOrder - 1
			if gap > 0 && gap <= maxGap {
				delta := movementWrapDegrees(currentAngle - lastKnownAngle)
				for fill := 1; fill <= gap; fill++ {
					ratio := float64(fill) / float64(gap+1)
					interp := movementWrapDegrees(lastKnownAngle + (delta * ratio))
					derived[movementSamples[lastKnownOrder+fill].sampleIndex] = interp
				}
			}
		}
		lastKnownOrder = order
		lastKnownAngle = currentAngle
	}
}

func movementBestDensePropAlignedDirectionCandidate(movementSamples []movementDirectionAngleSample, evals []movementDirectionCandidateEval, bestDense *MovementDirectionCandidate, bestSameActor *MovementDirectionCandidate) (*MovementDirectionFusedTrack, map[int]float64, map[string]bool, map[string]bool) {
	if len(movementSamples) == 0 || bestDense == nil || bestSameActor == nil {
		return nil, nil, nil, nil
	}
	sameEval, ok := movementDirectionFindEval(evals, *bestSameActor)
	if !ok {
		return nil, nil, nil, nil
	}
	propEvals := make([]movementDirectionCandidateEval, 0, len(evals))
	for _, eval := range evals {
		if eval.candidate.PropID != bestDense.PropID {
			continue
		}
		if eval.candidate.SampleCount < 24 {
			continue
		}
		propEvals = append(propEvals, eval)
	}
	if len(propEvals) == 0 {
		return nil, nil, nil, nil
	}
	type alignedEval struct {
		eval       movementDirectionCandidateEval
		prediction []movementDirectionPrediction
		packets    map[string]bool
		metrics    movementDirectionFusedMetrics
		overlap    int
	}
	aligned := make([]alignedEval, 0, len(propEvals))
	for _, eval := range propEvals {
		predictions := eval.prediction
		shift, overlap := movementDirectionPredictionAlignmentShift(predictions, sameEval.prediction)
		if overlap >= 4 {
			predictions = movementDirectionShiftPredictions(predictions, shift)
		}
		metrics := movementDirectionFusedPredictionMetrics(movementSamples, movementDirectionPredictionMap(predictions))
		if metrics.sampleCount < 24 {
			continue
		}
		if metrics.meanCosineAgreement < 0.45 || metrics.medianErrorDegrees > 80 {
			continue
		}
		aligned = append(aligned, alignedEval{
			eval:       eval,
			prediction: predictions,
			packets:    movementClonePacketKeys(eval.packetKeys),
			metrics:    metrics,
			overlap:    overlap,
		})
	}
	if len(aligned) == 0 {
		return nil, nil, nil, nil
	}
	sort.Slice(aligned, func(i, j int) bool {
		leftScore := movementDirectionDenseAlignedSeedScore(aligned[i].metrics, aligned[i].overlap)
		rightScore := movementDirectionDenseAlignedSeedScore(aligned[j].metrics, aligned[j].overlap)
		if leftScore == rightScore {
			return movementDirectionCandidateBetter(aligned[i].eval.candidate, aligned[j].eval.candidate)
		}
		return leftScore > rightScore
	})
	currentPredictions := aligned[0].prediction
	currentMetrics := aligned[0].metrics
	currentPackets := movementClonePacketKeys(aligned[0].packets)
	currentActors := map[string]bool{aligned[0].eval.candidate.ActorID: true}
	currentProps := map[string]bool{aligned[0].eval.candidate.PropID: true}
	currentSources := map[string]bool{aligned[0].eval.candidate.Source + "+dense-aligned": true}
	currentPlanes := map[string]bool{aligned[0].eval.candidate.Plane: true}
	currentAlignments := map[string]bool{aligned[0].eval.candidate.Alignment: true}
	currentOffsets := map[int]bool{aligned[0].eval.candidate.VectorOffset: true}
	currentCandidateKeys := map[string]bool{movementDirectionCandidateEvalKey(aligned[0].eval.candidate): true}
	currentCount := 1
	for index := 1; index < len(aligned); index++ {
		candidate := aligned[index]
		shift, overlap := movementDirectionPredictionAlignmentShift(candidate.prediction, currentPredictions)
		predictions := candidate.prediction
		if overlap >= 4 {
			predictions = movementDirectionShiftPredictions(predictions, shift)
		}
		if !movementDirectionCanMergePredictionSlices(currentPredictions, predictions) {
			continue
		}
		merged := movementDirectionMergePredictionsList(currentPredictions, predictions)
		metrics := movementDirectionFusedPredictionMetrics(movementSamples, movementDirectionPredictionMap(merged))
		if !movementDirectionFusionImproves(currentMetrics, metrics) {
			continue
		}
		currentPredictions = merged
		currentMetrics = metrics
		for key := range candidate.packets {
			currentPackets[key] = true
		}
		currentActors[candidate.eval.candidate.ActorID] = true
		currentSources[candidate.eval.candidate.Source] = true
		currentPlanes[candidate.eval.candidate.Plane] = true
		currentAlignments[candidate.eval.candidate.Alignment] = true
		currentOffsets[candidate.eval.candidate.VectorOffset] = true
		currentCandidateKeys[movementDirectionCandidateEvalKey(candidate.eval.candidate)] = true
		currentCount++
	}
	if shift, overlap := movementDirectionPredictionAlignmentShift(sameEval.prediction, currentPredictions); overlap >= 4 {
		anchorPredictions := movementDirectionShiftPredictions(sameEval.prediction, shift)
		if movementDirectionCanMergePredictionSlices(currentPredictions, anchorPredictions) {
			merged := movementDirectionMergePredictionsList(currentPredictions, anchorPredictions)
			metrics := movementDirectionFusedPredictionMetrics(movementSamples, movementDirectionPredictionMap(merged))
			if movementDirectionFusionImproves(currentMetrics, metrics) || metrics.meanCosineAgreement >= currentMetrics.meanCosineAgreement-0.02 {
				currentPredictions = merged
				currentMetrics = metrics
				for key := range sameEval.packetKeys {
					currentPackets[key] = true
				}
				currentActors[sameEval.candidate.ActorID] = true
				currentSources[sameEval.candidate.Source] = true
				currentProps[sameEval.candidate.PropID] = true
				currentPlanes[sameEval.candidate.Plane] = true
				currentAlignments[sameEval.candidate.Alignment] = true
				currentOffsets[sameEval.candidate.VectorOffset] = true
				currentCandidateKeys[movementDirectionCandidateEvalKey(sameEval.candidate)] = true
				currentCount++
			}
		}
	}
	fused := movementDirectionPredictionMap(currentPredictions)
	if currentMetrics.sampleCount == 0 {
		return nil, nil, nil, nil
	}
	return &MovementDirectionFusedTrack{
			Source:              movementDirectionJoinKeys(currentSources),
			PropIDs:             movementDirectionSortedKeys(currentProps),
			ActorIDs:            movementDirectionSortedKeys(currentActors),
			VectorOffsets:       movementDirectionSortedInts(currentOffsets),
			Planes:              movementDirectionSortedKeys(currentPlanes),
			AlignmentModes:      movementDirectionSortedKeys(currentAlignments),
			CandidateCount:      currentCount,
			CandidateKeys:       movementDirectionSortedKeys(currentCandidateKeys),
			UsedPacketCount:     len(currentPackets),
			SampleCount:         currentMetrics.sampleCount,
			CoveragePercent:     currentMetrics.coveragePercent,
			MeanErrorDegrees:    currentMetrics.meanErrorDegrees,
			MedianErrorDegrees:  currentMetrics.medianErrorDegrees,
			P90ErrorDegrees:     currentMetrics.p90ErrorDegrees,
			MeanCosineAgreement: currentMetrics.meanCosineAgreement,
			MakesSense:          currentMetrics.makesSense,
		},
		fused,
		currentPackets,
		currentActors
}

func movementBestAnchorExtendedDirectionCandidate(movementSamples []movementDirectionAngleSample, streams []movementDirectionStream, evals []movementDirectionCandidateEval, bestSameActor *MovementDirectionCandidate, densePropID string, minSamples int) movementDirectionAnchorResult {
	if bestSameActor == nil || !bestSameActor.MakesSense {
		return movementDirectionAnchorResult{}
	}
	var anchorEval *movementDirectionCandidateEval
	for index := range evals {
		if movementDirectionCandidateMatches(evals[index].candidate, *bestSameActor) {
			anchorEval = &evals[index]
			break
		}
	}
	if anchorEval == nil {
		return movementDirectionAnchorResult{}
	}
	anchorSamples := movementDirectionAnchorSamples(movementSamples, anchorEval.prediction)
	if len(anchorSamples) < minSamples {
		return movementDirectionAnchorResult{}
	}
	current := movementDirectionPredictionMap(anchorEval.prediction)
	currentMetrics := movementDirectionFusedPredictionMetrics(movementSamples, current)
	currentPacketKeys := map[string]bool{}
	for key := range anchorEval.packetKeys {
		currentPacketKeys[key] = true
	}
	currentActors := map[string]bool{anchorEval.candidate.ActorID: true}
	currentProps := map[string]bool{anchorEval.candidate.PropID: true}
	currentSources := map[string]bool{anchorEval.candidate.Source: true}
	currentPlanes := map[string]bool{anchorEval.candidate.Plane: true}
	currentAlignments := map[string]bool{anchorEval.candidate.Alignment: true}
	currentOffsets := map[int]bool{anchorEval.candidate.VectorOffset: true}
	currentCandidateKeys := map[string]bool{movementDirectionCandidateEvalKey(anchorEval.candidate): true}
	currentCount := 1
	extended := false
	type anchorEvalCandidate struct {
		eval       movementDirectionCandidateEval
		metrics    movementDirectionFusedMetrics
		packetKeys map[string]bool
	}
	anchorStreams := movementDirectionAnchorStreamShortlist(streams, bestSameActor.ActorID, densePropID, minSamples)
	candidates := make([]anchorEvalCandidate, 0, len(anchorStreams))
	for _, stream := range anchorStreams {
		unwrapped := movementUnwrapAngleSamples(stream.samples)
		seed, ok := movementDirectionCandidateAtShift(anchorSamples, stream, unwrapped, 0, minSamples)
		if !ok {
			continue
		}
		candidate, ok := movementDirectionCandidateForStream(anchorSamples, stream, unwrapped, seed, minSamples)
		if !ok || candidate.SampleCount < minSamples {
			continue
		}
		predictions, packetKeys := movementDirectionPredictionsForCandidate(movementSamples, stream, unwrapped, candidate)
		minPredictions := movementDirectionAnchorMinPredictions(len(anchorSamples), minSamples)
		if len(predictions) < minPredictions {
			continue
		}
		metrics := movementDirectionFusedPredictionMetrics(movementSamples, movementDirectionPredictionMap(predictions))
		if metrics.meanCosineAgreement < 0.25 || metrics.sampleCount < minPredictions {
			continue
		}
		candidates = append(candidates, anchorEvalCandidate{
			eval: movementDirectionCandidateEval{
				candidate:  candidate,
				stream:     stream,
				prediction: predictions,
				packetKeys: packetKeys,
			},
			metrics:    metrics,
			packetKeys: packetKeys,
		})
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].metrics.coveragePercent == candidates[j].metrics.coveragePercent {
			return movementDirectionCandidateBetter(candidates[i].eval.candidate, candidates[j].eval.candidate)
		}
		return candidates[i].metrics.coveragePercent > candidates[j].metrics.coveragePercent
	})
	improved := true
	for improved {
		improved = false
		bestIndex := -1
		bestPredictions := map[int]float64(nil)
		bestMetrics := currentMetrics
		for index, candidate := range candidates {
			if currentActors[candidate.eval.candidate.ActorID] && currentProps[candidate.eval.candidate.PropID] && currentOffsets[candidate.eval.candidate.VectorOffset] {
				continue
			}
			if !movementDirectionCanMergePredictions(current, candidate.eval.prediction) {
				continue
			}
			merged := movementDirectionMergePredictions(current, candidate.eval.prediction)
			metrics := movementDirectionFusedPredictionMetrics(movementSamples, merged)
			if !movementDirectionAnchorImproves(currentMetrics, metrics) {
				continue
			}
			if bestIndex == -1 || movementDirectionFusionImproves(bestMetrics, metrics) {
				bestIndex = index
				bestPredictions = merged
				bestMetrics = metrics
			}
		}
		if bestIndex == -1 {
			break
		}
		chosen := candidates[bestIndex]
		current = bestPredictions
		currentMetrics = bestMetrics
		currentActors[chosen.eval.candidate.ActorID] = true
		currentProps[chosen.eval.candidate.PropID] = true
		currentSources[chosen.eval.candidate.Source] = true
		currentPlanes[chosen.eval.candidate.Plane] = true
		currentAlignments[chosen.eval.candidate.Alignment] = true
		currentOffsets[chosen.eval.candidate.VectorOffset] = true
		currentCandidateKeys[movementDirectionCandidateEvalKey(chosen.eval.candidate)] = true
		for key := range chosen.packetKeys {
			currentPacketKeys[key] = true
		}
		currentCount++
		improved = true
		extended = true
	}
	if !extended {
		return movementDirectionAnchorResult{}
	}
	return movementDirectionAnchorResult{
		track: &MovementDirectionFusedTrack{
			Source:              movementDirectionJoinKeys(currentSources),
			PropIDs:             movementDirectionSortedKeys(currentProps),
			ActorIDs:            movementDirectionSortedKeys(currentActors),
			VectorOffsets:       movementDirectionSortedInts(currentOffsets),
			Planes:              movementDirectionSortedKeys(currentPlanes),
			AlignmentModes:      movementDirectionSortedKeys(currentAlignments),
			CandidateCount:      currentCount,
			CandidateKeys:       movementDirectionSortedKeys(currentCandidateKeys),
			UsedPacketCount:     len(currentPacketKeys),
			SampleCount:         currentMetrics.sampleCount,
			CoveragePercent:     currentMetrics.coveragePercent,
			MeanErrorDegrees:    currentMetrics.meanErrorDegrees,
			MedianErrorDegrees:  currentMetrics.medianErrorDegrees,
			P90ErrorDegrees:     currentMetrics.p90ErrorDegrees,
			MeanCosineAgreement: currentMetrics.meanCosineAgreement,
			MakesSense:          currentMetrics.makesSense,
		},
		derived:    current,
		packetKeys: currentPacketKeys,
		actors:     currentActors,
	}
}

func movementBestMotionHeadingFallback(movementSamples []movementDirectionAngleSample, evals []movementDirectionCandidateEval, bestSameActor *MovementDirectionCandidate) (*MovementDirectionFusedTrack, map[int]float64) {
	if len(movementSamples) < 24 {
		return nil, nil
	}
	offset := 0.0
	if bestSameActor != nil {
		if sameEval, ok := movementDirectionFindEval(evals, *bestSameActor); ok {
			diffs := make([]float64, 0, len(sameEval.prediction))
			movementByIndex := map[int]float64{}
			for _, sample := range movementSamples {
				movementByIndex[sample.sampleIndex] = sample.angle
			}
			for _, prediction := range sameEval.prediction {
				movementAngle, ok := movementByIndex[prediction.sampleIndex]
				if !ok {
					continue
				}
				diffs = append(diffs, movementWrapDegrees(prediction.viewingDegrees-movementAngle))
			}
			if len(diffs) >= 4 {
				offset = movementCircularMeanAngleValues(diffs)
			}
		}
	}
	derived := make(map[int]float64, len(movementSamples))
	for index := range movementSamples {
		left := maxInt(0, index-3)
		right := minInt(len(movementSamples)-1, index+3)
		window := make([]float64, 0, right-left+1)
		for sampleIndex := left; sampleIndex <= right; sampleIndex++ {
			window = append(window, movementSamples[sampleIndex].angle)
		}
		derived[movementSamples[index].sampleIndex] = movementWrapDegrees(movementCircularMeanAngleValues(window) + offset)
	}
	metrics := movementDirectionFusedPredictionMetrics(movementSamples, derived)
	return &MovementDirectionFusedTrack{
		Source:              "movement-smoothed-fallback",
		PropIDs:             nil,
		ActorIDs:            nil,
		VectorOffsets:       nil,
		Planes:              nil,
		AlignmentModes:      []string{"movement"},
		CandidateCount:      1,
		UsedPacketCount:     0,
		SampleCount:         metrics.sampleCount,
		CoveragePercent:     metrics.coveragePercent,
		MeanErrorDegrees:    metrics.meanErrorDegrees,
		MedianErrorDegrees:  metrics.medianErrorDegrees,
		P90ErrorDegrees:     metrics.p90ErrorDegrees,
		MeanCosineAgreement: metrics.meanCosineAgreement,
		MakesSense:          metrics.makesSense,
	}, derived
}

func movementDirectionAnchorMinPredictions(anchorSampleCount int, minSamples int) int {
	return maxInt(minSamples, maxInt((anchorSampleCount*3)/4, 24))
}

func movementDirectionAnchorStreamShortlist(streams []movementDirectionStream, actorID string, densePropID string, minSamples int) []movementDirectionStream {
	type streamCandidate struct {
		stream   movementDirectionStream
		priority int
		rawCount int
	}
	candidates := make([]streamCandidate, 0, len(streams))
	seen := map[string]bool{}
	for _, stream := range streams {
		if len(stream.samples) < minSamples {
			continue
		}
		sameActor := stream.actorID == actorID
		sameDenseProp := densePropID != "" && stream.propID == densePropID
		if !sameActor && !sameDenseProp {
			continue
		}
		key := stream.source + "|" + stream.propID + "|" + stream.actorID + "|" + strconv.Itoa(stream.vectorOffset) + "|" + stream.axisA + "|" + stream.axisB
		if seen[key] {
			continue
		}
		seen[key] = true
		priority := len(stream.samples)
		if sameActor {
			priority += 1000000
		}
		if sameDenseProp {
			priority += 500000
		}
		if strings.Contains(stream.source, "delta-integrated") {
			priority += 250000
		}
		if strings.Contains(stream.source, "integrated") {
			priority += 125000
		}
		candidates = append(candidates, streamCandidate{
			stream:   stream,
			priority: priority,
			rawCount: len(stream.samples),
		})
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].priority == candidates[j].priority {
			if candidates[i].rawCount == candidates[j].rawCount {
				left := candidates[i].stream
				right := candidates[j].stream
				if left.propID == right.propID {
					if left.actorID == right.actorID {
						if left.vectorOffset == right.vectorOffset {
							return left.source < right.source
						}
						return left.vectorOffset < right.vectorOffset
					}
					return left.actorID < right.actorID
				}
				return left.propID < right.propID
			}
			return candidates[i].rawCount > candidates[j].rawCount
		}
		return candidates[i].priority > candidates[j].priority
	})
	limit := 256
	if len(candidates) > limit {
		candidates = candidates[:limit]
	}
	out := make([]movementDirectionStream, 0, len(candidates))
	for _, candidate := range candidates {
		out = append(out, candidate.stream)
	}
	return out
}

func movementDirectionFusionSeedScore(candidate MovementDirectionCandidate, actorID string, movementCount int) float64 {
	score := candidate.MeanCosineAgreement - (candidate.MeanErrorDegrees / 360) + (float64(candidate.SampleCount) / 1200)
	score += float64(candidate.SampleCount) / float64(maxInt(movementCount, 1))
	if candidate.ActorID == actorID {
		score += 0.08
	}
	if candidate.MakesSense {
		score += 0.05
	}
	if strings.Contains(candidate.Source, "integrated") {
		score += 0.08
	}
	return score
}

func movementDirectionFusedContains(indexes []int, candidate int) bool {
	for _, index := range indexes {
		if index == candidate {
			return true
		}
	}
	return false
}

func movementDirectionPredictionMap(predictions []movementDirectionPrediction) map[int]float64 {
	out := make(map[int]float64, len(predictions))
	for _, prediction := range predictions {
		out[prediction.sampleIndex] = prediction.viewingDegrees
	}
	return out
}

func movementDirectionFindEval(evals []movementDirectionCandidateEval, candidate MovementDirectionCandidate) (movementDirectionCandidateEval, bool) {
	for _, eval := range evals {
		if movementDirectionCandidateMatches(eval.candidate, candidate) {
			return eval, true
		}
	}
	return movementDirectionCandidateEval{}, false
}

func movementDirectionPredictionAlignmentShift(left []movementDirectionPrediction, right []movementDirectionPrediction) (float64, int) {
	rightByIndex := map[int]float64{}
	for _, prediction := range right {
		rightByIndex[prediction.sampleIndex] = prediction.viewingDegrees
	}
	diffs := make([]float64, 0, minInt(len(left), len(right)))
	for _, prediction := range left {
		other, ok := rightByIndex[prediction.sampleIndex]
		if !ok {
			continue
		}
		diffs = append(diffs, movementWrapDegrees(other-prediction.viewingDegrees))
	}
	if len(diffs) == 0 {
		return 0, 0
	}
	return movementCircularMeanAngleValues(diffs), len(diffs)
}

func movementDirectionShiftPredictions(predictions []movementDirectionPrediction, shift float64) []movementDirectionPrediction {
	out := make([]movementDirectionPrediction, len(predictions))
	for index, prediction := range predictions {
		prediction.viewingDegrees = movementWrapDegrees(prediction.viewingDegrees + shift)
		out[index] = prediction
	}
	return out
}

func movementDirectionMergePredictionsList(current []movementDirectionPrediction, next []movementDirectionPrediction) []movementDirectionPrediction {
	type predictionAggregate struct {
		prediction movementDirectionPrediction
		count      int
	}
	bySample := map[int]predictionAggregate{}
	for _, prediction := range current {
		bySample[prediction.sampleIndex] = predictionAggregate{prediction: prediction, count: 1}
	}
	for _, prediction := range next {
		entry, ok := bySample[prediction.sampleIndex]
		if !ok {
			bySample[prediction.sampleIndex] = predictionAggregate{prediction: prediction, count: 1}
			continue
		}
		entry.prediction.viewingDegrees = movementCircularMeanAngleValues([]float64{entry.prediction.viewingDegrees, prediction.viewingDegrees})
		entry.count++
		bySample[prediction.sampleIndex] = entry
	}
	indexes := make([]int, 0, len(bySample))
	for index := range bySample {
		indexes = append(indexes, index)
	}
	sort.Ints(indexes)
	out := make([]movementDirectionPrediction, 0, len(indexes))
	for _, index := range indexes {
		out = append(out, bySample[index].prediction)
	}
	return out
}

func movementDirectionCanMergePredictionSlices(current []movementDirectionPrediction, next []movementDirectionPrediction) bool {
	return movementDirectionCanMergePredictions(movementDirectionPredictionMap(current), next)
}

func movementDirectionDenseAlignedSeedScore(metrics movementDirectionFusedMetrics, overlap int) float64 {
	return metrics.score + (float64(overlap) / 200)
}

func movementClonePacketKeys(values map[string]bool) map[string]bool {
	out := make(map[string]bool, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}

func movementDirectionCanMergePredictions(current map[int]float64, candidate []movementDirectionPrediction) bool {
	overlap := 0
	errorSum := 0.0
	newCoverage := 0
	for _, prediction := range candidate {
		existing, ok := current[prediction.sampleIndex]
		if ok {
			overlap++
			errorSum += math.Abs(movementWrapDegrees(existing - prediction.viewingDegrees))
			continue
		}
		newCoverage++
	}
	if newCoverage < 8 {
		return false
	}
	if overlap == 0 {
		return true
	}
	return (errorSum / float64(overlap)) <= 24
}

func movementDirectionMergePredictions(current map[int]float64, candidate []movementDirectionPrediction) map[int]float64 {
	merged := make(map[int]float64, len(current)+len(candidate))
	for sampleIndex, viewingDegrees := range current {
		merged[sampleIndex] = viewingDegrees
	}
	for _, prediction := range candidate {
		if existing, ok := merged[prediction.sampleIndex]; ok {
			merged[prediction.sampleIndex] = movementCircularMeanAngleValues([]float64{existing, prediction.viewingDegrees})
			continue
		}
		merged[prediction.sampleIndex] = prediction.viewingDegrees
	}
	return merged
}

func movementDirectionFusedPredictionMetrics(movementSamples []movementDirectionAngleSample, fused map[int]float64) movementDirectionFusedMetrics {
	if len(fused) == 0 || len(movementSamples) == 0 {
		return movementDirectionFusedMetrics{}
	}
	errors := make([]float64, 0, len(fused))
	cosineSum := 0.0
	for _, movement := range movementSamples {
		viewingDegrees, ok := fused[movement.sampleIndex]
		if !ok {
			continue
		}
		diff := movementWrapDegrees(movement.angle - viewingDegrees)
		errors = append(errors, math.Abs(diff))
		cosineSum += math.Cos(diff * math.Pi / 180)
	}
	if len(errors) == 0 {
		return movementDirectionFusedMetrics{}
	}
	coverage := movementPercent(len(errors), len(movementSamples))
	meanError := movementMean(errors)
	meanCosine := cosineSum / float64(len(errors))
	return movementDirectionFusedMetrics{
		sampleCount:         len(errors),
		coveragePercent:     coverage,
		meanErrorDegrees:    meanError,
		medianErrorDegrees:  movementPercentile(errors, 0.50),
		p90ErrorDegrees:     movementPercentile(errors, 0.90),
		meanCosineAgreement: meanCosine,
		score:               meanCosine - (meanError / 360) + (coverage / 250),
		makesSense:          meanCosine >= 0.78 && movementPercentile(errors, 0.50) <= 35,
	}
}

func movementDirectionFusionImproves(current movementDirectionFusedMetrics, next movementDirectionFusedMetrics) bool {
	if next.sampleCount == 0 {
		return false
	}
	if current.sampleCount == 0 {
		return true
	}
	if next.makesSense && !current.makesSense {
		return true
	}
	if next.score > current.score+0.006 {
		return true
	}
	coverageGain := next.coveragePercent - current.coveragePercent
	if coverageGain >= 2.0 && next.meanCosineAgreement >= current.meanCosineAgreement-0.02 && next.medianErrorDegrees <= current.medianErrorDegrees+4 {
		return true
	}
	if coverageGain >= 8.0 && next.meanCosineAgreement >= maxFloat64(current.meanCosineAgreement-0.12, 0.35) && next.medianErrorDegrees <= current.medianErrorDegrees+20 {
		return true
	}
	return false
}

func movementDirectionPreferDenseFallback(current movementDirectionFusedMetrics, next movementDirectionFusedMetrics) bool {
	if next.sampleCount == 0 {
		return false
	}
	if current.sampleCount == 0 {
		return true
	}
	if next.coveragePercent >= current.coveragePercent+10 && next.meanCosineAgreement >= 0.45 && next.medianErrorDegrees <= 80 {
		return true
	}
	if current.coveragePercent < 5 && next.coveragePercent >= current.coveragePercent*4 && next.meanCosineAgreement >= 0.45 && next.medianErrorDegrees <= 80 {
		return true
	}
	return false
}

func movementDirectionPreferMotionFallback(current movementDirectionFusedMetrics, next movementDirectionFusedMetrics) bool {
	if next.sampleCount == 0 {
		return false
	}
	if current.sampleCount == 0 {
		return true
	}
	if current.meanCosineAgreement >= 0.75 && current.coveragePercent >= 40 {
		return false
	}
	return next.meanCosineAgreement >= current.meanCosineAgreement+0.2
}

func movementDirectionAnchorImproves(current movementDirectionFusedMetrics, next movementDirectionFusedMetrics) bool {
	if next.sampleCount == 0 {
		return false
	}
	if current.sampleCount == 0 {
		return true
	}
	if next.makesSense && !current.makesSense {
		return true
	}
	if next.score > current.score+0.01 {
		return true
	}
	coverageGain := next.coveragePercent - current.coveragePercent
	if coverageGain >= 6 && next.meanCosineAgreement >= current.meanCosineAgreement-0.04 && next.medianErrorDegrees <= current.medianErrorDegrees+8 {
		return true
	}
	return false
}

func movementDirectionCandidateEvalKey(candidate MovementDirectionCandidate) string {
	return strings.Join([]string{
		candidate.Source,
		candidate.PropID,
		candidate.ActorID,
		strconv.Itoa(candidate.VectorOffset),
		candidate.Plane,
		candidate.Alignment,
		strconv.Itoa(candidate.ByteShift),
		strconv.Itoa(candidate.TimeShiftMilliseconds),
		strconv.FormatFloat(candidate.AngleScale, 'g', -1, 64),
	}, "|")
}

func movementDirectionTrackFromCandidate(candidate MovementDirectionCandidate, packetKeys map[string]bool, metrics movementDirectionFusedMetrics) *MovementDirectionFusedTrack {
	if metrics.sampleCount == 0 {
		return nil
	}
	return &MovementDirectionFusedTrack{
		Source:              candidate.Source,
		PropIDs:             []string{candidate.PropID},
		ActorIDs:            []string{candidate.ActorID},
		VectorOffsets:       []int{candidate.VectorOffset},
		Planes:              []string{candidate.Plane},
		AlignmentModes:      []string{candidate.Alignment},
		CandidateCount:      1,
		UsedPacketCount:     len(packetKeys),
		SampleCount:         metrics.sampleCount,
		CoveragePercent:     metrics.coveragePercent,
		MeanErrorDegrees:    metrics.meanErrorDegrees,
		MedianErrorDegrees:  metrics.medianErrorDegrees,
		P90ErrorDegrees:     metrics.p90ErrorDegrees,
		MeanCosineAgreement: metrics.meanCosineAgreement,
		MakesSense:          metrics.makesSense,
	}
}

func movementDirectionCandidateMatches(left MovementDirectionCandidate, right MovementDirectionCandidate) bool {
	return left.Source == right.Source &&
		left.PropID == right.PropID &&
		left.ActorID == right.ActorID &&
		left.VectorOffset == right.VectorOffset &&
		left.Plane == right.Plane &&
		left.Alignment == right.Alignment &&
		left.ByteShift == right.ByteShift &&
		left.AngleScale == right.AngleScale &&
		left.TimeShiftMilliseconds == right.TimeShiftMilliseconds
}

func movementDirectionScaleChoices(source string) []float64 {
	if strings.Contains(source, "word-delta") {
		return []float64{1.0 / 65536, 1.0 / 16384, 1.0 / 4096, 1.0 / 1024, 1.0 / 256, 1.0 / 64, 1.0 / 16, 1.0 / 4, 1, 4, 16}
	}
	if !strings.Contains(source, "integrated") {
		return []float64{1}
	}
	return []float64{0.0625, 0.125, 0.25, 0.5, 0.75, 1, 1.5, 2, 3, 4, 6, 8, 12, 16, 24, 32}
}

func movementDirectionAnchorSamples(movementSamples []movementDirectionAngleSample, predictions []movementDirectionPrediction) []movementDirectionAngleSample {
	sampleByIndex := map[int]movementDirectionAngleSample{}
	for _, sample := range movementSamples {
		sampleByIndex[sample.sampleIndex] = sample
	}
	out := make([]movementDirectionAngleSample, 0, len(predictions))
	for _, prediction := range predictions {
		target := movementDirectionAngleSample{
			offset:      prediction.offset,
			sampleIndex: prediction.sampleIndex,
			angle:       prediction.viewingDegrees,
		}
		if movement, ok := sampleByIndex[prediction.sampleIndex]; ok {
			target.milliseconds = movement.milliseconds
			target.hasTime = movement.hasTime
		}
		out = append(out, target)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].sampleIndex < out[j].sampleIndex
	})
	return out
}

func fusedTrackMetrics(track *MovementDirectionFusedTrack) movementDirectionFusedMetrics {
	if track == nil {
		return movementDirectionFusedMetrics{}
	}
	return movementDirectionFusedMetrics{
		sampleCount:         track.SampleCount,
		coveragePercent:     track.CoveragePercent,
		meanErrorDegrees:    track.MeanErrorDegrees,
		medianErrorDegrees:  track.MedianErrorDegrees,
		p90ErrorDegrees:     track.P90ErrorDegrees,
		meanCosineAgreement: track.MeanCosineAgreement,
		score:               track.MeanCosineAgreement - (track.MeanErrorDegrees / 360) + (track.CoveragePercent / 250),
		makesSense:          track.MakesSense,
	}
}

func movementDirectionSortedKeys(values map[string]bool) []string {
	out := make([]string, 0, len(values))
	for value := range values {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func movementDirectionJoinKeys(values map[string]bool) string {
	return strings.Join(movementDirectionSortedKeys(values), ", ")
}

func movementDirectionSortedInts(values map[int]bool) []int {
	out := make([]int, 0, len(values))
	for value := range values {
		out = append(out, value)
	}
	sort.Ints(out)
	return out
}

func movementUnwrapAngleSamples(samples []movementDirectionAngleSample) []movementDirectionAngleSample {
	if len(samples) == 0 {
		return nil
	}
	out := make([]movementDirectionAngleSample, len(samples))
	out[0] = samples[0]
	for i := 1; i < len(samples); i++ {
		angle := samples[i].angle
		previous := out[i-1].angle
		for angle-previous > 180 {
			angle -= 360
		}
		for angle-previous < -180 {
			angle += 360
		}
		out[i] = movementDirectionAngleSample{
			offset:       samples[i].offset,
			sampleIndex:  samples[i].sampleIndex,
			milliseconds: samples[i].milliseconds,
			hasTime:      samples[i].hasTime,
			angle:        angle,
		}
	}
	return out
}

func movementInterpolatedAngleAtOffset(samples []movementDirectionAngleSample, offset int, window int) (float64, bool) {
	if len(samples) == 0 {
		return 0, false
	}
	index := sort.Search(len(samples), func(i int) bool {
		return samples[i].offset >= offset
	})
	if index == 0 {
		if samples[0].offset-offset > window {
			return 0, false
		}
		return samples[0].angle, true
	}
	if index >= len(samples) {
		if offset-samples[len(samples)-1].offset > window {
			return 0, false
		}
		return samples[len(samples)-1].angle, true
	}
	left := samples[index-1]
	right := samples[index]
	if offset-left.offset > window && right.offset-offset > window {
		return 0, false
	}
	if left.offset == right.offset {
		return left.angle, true
	}
	if offset-left.offset > window {
		return right.angle, true
	}
	if right.offset-offset > window {
		return left.angle, true
	}
	ratio := float64(offset-left.offset) / float64(right.offset-left.offset)
	return left.angle + ((right.angle - left.angle) * ratio), true
}

func movementCircularMeanAngleValues(values []float64) float64 {
	sinSum := 0.0
	cosSum := 0.0
	for _, value := range values {
		radians := value * math.Pi / 180
		sinSum += math.Sin(radians)
		cosSum += math.Cos(radians)
	}
	if sinSum == 0 && cosSum == 0 {
		return 0
	}
	return math.Atan2(sinSum, cosSum) * 180 / math.Pi
}

func signedAxisName(axis string, sign float64) string {
	if sign < 0 {
		return "-" + axis
	}
	return "+" + axis
}

func maxInt(left int, right int) int {
	if left > right {
		return left
	}
	return right
}

func minInt(left int, right int) int {
	if left < right {
		return left
	}
	return right
}
