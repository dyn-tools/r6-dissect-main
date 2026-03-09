package dissect

import (
	"encoding/hex"
	"sort"
	"strconv"
	"strings"
)

type MovementProbeOutput struct {
	Header          Header                        `json:"header"`
	PropCandidates  []MovementProbePropCandidate  `json:"propCandidates"`
	PairCandidates  []MovementProbePairCandidate  `json:"pairCandidates,omitempty"`
	PacketClasses   []MovementProbePacketClass    `json:"packetClasses,omitempty"`
	Fragments       []MovementProbeFragment       `json:"fragments,omitempty"`
	FusedCandidates []MovementProbeFusedCandidate `json:"fusedCandidates,omitempty"`
}

type MovementProbePropCandidate struct {
	MovementPropCandidate
	PositionScore       float64 `json:"positionScore"`
	RotationScore       float64 `json:"rotationScore"`
	CombinedScore       float64 `json:"combinedScore"`
	LongTracks          int     `json:"longTracks"`
	BalancedTracks      int     `json:"balancedTracks"`
	TopPosition         int     `json:"topPosition"`
	TopRotation         int     `json:"topRotation"`
	TopMerged           int     `json:"topMerged"`
	DominantMerged      int     `json:"dominantMerged"`
	DominantGap         int     `json:"dominantGap"`
	DominantDistance    float64 `json:"dominantDistance"`
	DominantZRange      float64 `json:"dominantZRange"`
	PositionJumps       int     `json:"positionJumps"`
	RotationJumps       int     `json:"rotationJumps"`
	RotationSpanDegrees float64 `json:"rotationSpanDegrees"`
}

type MovementProbePairCandidate struct {
	PositionPropID      string  `json:"positionPropID"`
	RotationPropID      string  `json:"rotationPropID"`
	PositionScore       float64 `json:"positionScore"`
	RotationScore       float64 `json:"rotationScore"`
	CombinedScore       float64 `json:"combinedScore"`
	TopPosition         int     `json:"topPosition"`
	TopRotation         int     `json:"topRotation"`
	TopMerged           int     `json:"topMerged"`
	DominantMerged      int     `json:"dominantMerged"`
	DominantGap         int     `json:"dominantGap"`
	DominantDistance    float64 `json:"dominantDistance"`
	DominantZRange      float64 `json:"dominantZRange"`
	PositionJumps       int     `json:"positionJumps"`
	RotationJumps       int     `json:"rotationJumps"`
	RotationSpanDegrees float64 `json:"rotationSpanDegrees"`
}

type MovementProbeFragment struct {
	ID                  string   `json:"id"`
	PropID              string   `json:"propID"`
	ActorID             string   `json:"actorID"`
	PositionSamples     int      `json:"positionSamples"`
	RotationSamples     int      `json:"rotationSamples"`
	MergedSamples       int      `json:"mergedSamples"`
	FirstOffset         int      `json:"firstOffset"`
	LastOffset          int      `json:"lastOffset"`
	FirstPosition       *Vector3 `json:"firstPosition,omitempty"`
	LastPosition        *Vector3 `json:"lastPosition,omitempty"`
	FirstRotation       *Vector3 `json:"firstRotation,omitempty"`
	LastRotation        *Vector3 `json:"lastRotation,omitempty"`
	Distance            float64  `json:"distance,omitempty"`
	ZRange              float64  `json:"zRange,omitempty"`
	PositionJumps       int      `json:"positionJumps,omitempty"`
	RotationJumps       int      `json:"rotationJumps,omitempty"`
	RotationSpanDegrees float64  `json:"rotationSpanDegrees,omitempty"`
	OriginLike          bool     `json:"originLike,omitempty"`
	Score               float64  `json:"score"`
}

type MovementProbeFusedCandidate struct {
	ID                  string   `json:"id"`
	FragmentIDs         []string `json:"fragmentIDs"`
	PositionSamples     int      `json:"positionSamples"`
	RotationSamples     int      `json:"rotationSamples"`
	MergedSamples       int      `json:"mergedSamples"`
	FirstOffset         int      `json:"firstOffset"`
	LastOffset          int      `json:"lastOffset"`
	FirstPosition       *Vector3 `json:"firstPosition,omitempty"`
	LastPosition        *Vector3 `json:"lastPosition,omitempty"`
	FirstRotation       *Vector3 `json:"firstRotation,omitempty"`
	LastRotation        *Vector3 `json:"lastRotation,omitempty"`
	Distance            float64  `json:"distance,omitempty"`
	ZRange              float64  `json:"zRange,omitempty"`
	PositionJumps       int      `json:"positionJumps,omitempty"`
	RotationJumps       int      `json:"rotationJumps,omitempty"`
	RotationSpanDegrees float64  `json:"rotationSpanDegrees,omitempty"`
	Score               float64  `json:"score"`
}

type movementProbeFragmentCandidate struct {
	summary MovementProbeFragment
	track   MovementTrack
}

type MovementProbePacketClass struct {
	Key                 string                     `json:"key"`
	PropID              string                     `json:"propID"`
	SampleCount         int                        `json:"sampleCount"`
	ActorCount          int                        `json:"actorCount"`
	TopActorSampleCount int                        `json:"topActorSampleCount"`
	TailZero            bool                       `json:"tailZero"`
	PrimaryClass        string                     `json:"primaryClass"`
	SecondaryClass      string                     `json:"secondaryClass"`
	HasQuaternionAt22   bool                       `json:"hasQuaternionAt22"`
	CurrentPositionLike bool                       `json:"currentPositionLike"`
	CurrentRotationLike bool                       `json:"currentRotationLike"`
	QuaternionOffsets   []MovementProbeOffsetCount `json:"quaternionOffsets,omitempty"`
	VectorOffsets       []MovementProbeOffsetCount `json:"vectorOffsets,omitempty"`
	ExampleOffset       int                        `json:"exampleOffset"`
	ExampleActorID      string                     `json:"exampleActorID"`
	ExamplePrimary      *Vector3                   `json:"examplePrimary,omitempty"`
	ExampleSecondary    *Vector3                   `json:"exampleSecondary,omitempty"`
}

type MovementProbeOffsetCount struct {
	Offset int `json:"offset"`
	Count  int `json:"count"`
}

type movementProbePacketClassAggregate struct {
	summary           MovementProbePacketClass
	actorCounts       map[string]int
	vectorOffsets     map[int]int
	quaternionOffsets map[int]int
}

func (r *Reader) MovementProbe() MovementProbeOutput {
	return r.MovementProbeOptions(MovementOptions{})
}

func (r *Reader) MovementProbeOptions(options MovementOptions) MovementProbeOutput {
	zeroActorPrefixes := countMovementZeroActorPrefixes(r.Header.CodeVersion, r.b, r.offset)
	statsByProp := movementPropStatsByID(r.Header, r.b, r.offset, zeroActorPrefixes)
	summaries := make([]movementPropSummary, 0, len(statsByProp))
	targetActors := len(r.Header.Players)
	if targetActors == 0 {
		targetActors = 10
	}
	for _, stats := range statsByProp {
		summary := summarizeMovementProp(stats, targetActors)
		summary.candidate.Score = movementPositionScore(summary.candidate)
		summaries = append(summaries, summary)
	}
	sort.Slice(summaries, func(i, j int) bool {
		if summaries[i].candidate.SampleCount == summaries[j].candidate.SampleCount {
			return summaries[i].candidate.PropID < summaries[j].candidate.PropID
		}
		return summaries[i].candidate.SampleCount > summaries[j].candidate.SampleCount
	})
	pool := movementY11CandidateIDs(summaries, "", movementY11PropPool(summaries, 32), 32)
	if len(pool) == 0 {
		for _, summary := range summaries {
			pool = append(pool, summary.candidate.PropID)
			if len(pool) >= 32 {
				break
			}
		}
	}
	propCandidates := make([]MovementProbePropCandidate, 0, len(pool))
	qualityCache := map[string]movementTrackQuality{}
	for _, propID := range pool {
		quality := qualityCache[propID]
		if quality == (movementTrackQuality{}) {
			quality = evaluateMovementTrackQuality(r.Header, r.b, r.offset, zeroActorPrefixes, decodeMovementPropOrDefault(propID, movementPositionProp), decodeMovementPropOrDefault(propID, movementRotationProp))
			qualityCache[propID] = quality
		}
		candidate := movementProbePropCandidate(propID, summaryByPropID(summaries, propID), targetActors, quality)
		propCandidates = append(propCandidates, candidate)
	}
	sort.Slice(propCandidates, func(i, j int) bool {
		if propCandidates[i].CombinedScore == propCandidates[j].CombinedScore {
			return propCandidates[i].PropID < propCandidates[j].PropID
		}
		return propCandidates[i].CombinedScore > propCandidates[j].CombinedScore
	})
	positionIDs := movementTopProbePropIDs(propCandidates, func(candidate MovementProbePropCandidate) float64 { return candidate.PositionScore }, 8)
	rotationIDs := movementTopProbePropIDs(propCandidates, func(candidate MovementProbePropCandidate) float64 { return candidate.RotationScore }, 8)
	pairCandidates := make([]MovementProbePairCandidate, 0, len(positionIDs)*len(rotationIDs))
	for _, positionID := range positionIDs {
		for _, rotationID := range rotationIDs {
			quality := evaluateMovementTrackQuality(r.Header, r.b, r.offset, zeroActorPrefixes, decodeMovementPropOrDefault(positionID, movementPositionProp), decodeMovementPropOrDefault(rotationID, movementRotationProp))
			pairCandidates = append(pairCandidates, movementProbePairCandidate(positionID, rotationID, targetActors, quality))
		}
	}
	sort.Slice(pairCandidates, func(i, j int) bool {
		if pairCandidates[i].CombinedScore == pairCandidates[j].CombinedScore {
			if pairCandidates[i].PositionPropID == pairCandidates[j].PositionPropID {
				return pairCandidates[i].RotationPropID < pairCandidates[j].RotationPropID
			}
			return pairCandidates[i].PositionPropID < pairCandidates[j].PositionPropID
		}
		return pairCandidates[i].CombinedScore > pairCandidates[j].CombinedScore
	})
	if len(propCandidates) > 24 {
		propCandidates = propCandidates[:24]
	}
	if len(pairCandidates) > 24 {
		pairCandidates = pairCandidates[:24]
	}
	packetClasses := movementProbePacketClasses(r.Header, r.b, r.offset, zeroActorPrefixes)
	fragments := movementProbeFragments(r.Header, r.b, r.offset, zeroActorPrefixes, pool)
	fusedCandidates := movementProbeFusedCandidates(fragments, 12)
	return MovementProbeOutput{
		Header:          r.Header,
		PropCandidates:  propCandidates,
		PairCandidates:  pairCandidates,
		PacketClasses:   packetClasses,
		Fragments:       movementProbeFragmentSummaries(fragments, 40),
		FusedCandidates: fusedCandidates,
	}
}

func movementPropStatsByID(header Header, buf []byte, start int, zeroActorPrefixes map[string]int) map[string]*movementPropStats {
	statsByProp := map[string]*movementPropStats{}
	for i := start; i < len(buf); i++ {
		candidate, ok := movementCandidateRecordAtCodeVersion(header.CodeVersion, buf, i, zeroActorPrefixes)
		if !ok || !movementPropPatternAllowed(header.CodeVersion, candidate.prop) {
			continue
		}
		propID := hex.EncodeToString(candidate.prop)
		stats := statsByProp[propID]
		if stats == nil {
			stats = &movementPropStats{
				prop:               append([]byte(nil), candidate.prop...),
				actorCounts:        map[string]int{},
				positionActorCount: map[string]int{},
				rotationActorCount: map[string]int{},
			}
			statsByProp[propID] = stats
		}
		actorID := hex.EncodeToString(candidate.actor)
		stats.sampleCount++
		stats.actorCounts[actorID]++
		if header.CodeVersion >= Y11S1Alpha3 {
			if movementY11CandidateIsPosition(candidate) {
				value, _ := movementY11CandidatePosition(candidate)
				stats.positionLikeCount++
				stats.positionActorCount[actorID]++
				stats.absValues = appendMovementAbsValues(stats.absValues, value)
			}
			if movementY11CandidateIsRotation(candidate) {
				stats.rotationLikeCount++
				stats.rotationActorCount[actorID]++
			}
		} else {
			stats.absValues = appendMovementAbsValues(stats.absValues, candidate.primary)
			i += 37
		}
	}
	return statsByProp
}

func summaryByPropID(summaries []movementPropSummary, propID string) MovementPropCandidate {
	propID = normalizeMovementPropID(propID)
	for _, summary := range summaries {
		if normalizeMovementPropID(summary.candidate.PropID) == propID {
			return summary.candidate
		}
	}
	return MovementPropCandidate{PropID: propID}
}

func movementProbePropCandidate(propID string, base MovementPropCandidate, targetActors int, quality movementTrackQuality) MovementProbePropCandidate {
	base.PropID = normalizeMovementPropID(propID)
	return MovementProbePropCandidate{
		MovementPropCandidate: base,
		PositionScore:         movementTrackQualityPositionScore(targetActors, quality),
		RotationScore:         movementTrackQualityRotationScore(targetActors, quality),
		CombinedScore:         movementTrackQualityScore(targetActors, quality),
		LongTracks:            quality.longTracks,
		BalancedTracks:        quality.balancedTracks,
		TopPosition:           quality.topPosition,
		TopRotation:           quality.topRotation,
		TopMerged:             quality.topMerged,
		DominantMerged:        quality.dominantMerged,
		DominantGap:           quality.dominantGap,
		DominantDistance:      quality.dominantDistance,
		DominantZRange:        quality.dominantZRange,
		PositionJumps:         quality.positionJumps,
		RotationJumps:         quality.rotationJumps,
		RotationSpanDegrees:   quality.rotationSpanDegrees,
	}
}

func movementProbePairCandidate(positionID string, rotationID string, targetActors int, quality movementTrackQuality) MovementProbePairCandidate {
	return MovementProbePairCandidate{
		PositionPropID:      normalizeMovementPropID(positionID),
		RotationPropID:      normalizeMovementPropID(rotationID),
		PositionScore:       movementTrackQualityPositionScore(targetActors, quality),
		RotationScore:       movementTrackQualityRotationScore(targetActors, quality),
		CombinedScore:       movementTrackQualityScore(targetActors, quality),
		TopPosition:         quality.topPosition,
		TopRotation:         quality.topRotation,
		TopMerged:           quality.topMerged,
		DominantMerged:      quality.dominantMerged,
		DominantGap:         quality.dominantGap,
		DominantDistance:    quality.dominantDistance,
		DominantZRange:      quality.dominantZRange,
		PositionJumps:       quality.positionJumps,
		RotationJumps:       quality.rotationJumps,
		RotationSpanDegrees: quality.rotationSpanDegrees,
	}
}

func movementTopProbePropIDs(candidates []MovementProbePropCandidate, score func(MovementProbePropCandidate) float64, limit int) []string {
	if len(candidates) == 0 || limit <= 0 {
		return nil
	}
	sorted := append([]MovementProbePropCandidate(nil), candidates...)
	sort.Slice(sorted, func(i, j int) bool {
		left := score(sorted[i])
		right := score(sorted[j])
		if left == right {
			return sorted[i].PropID < sorted[j].PropID
		}
		return left > right
	})
	out := make([]string, 0, limit)
	seen := map[string]bool{}
	for _, candidate := range sorted {
		if candidate.PropID == "" || seen[candidate.PropID] {
			continue
		}
		seen[candidate.PropID] = true
		out = append(out, candidate.PropID)
		if len(out) >= limit {
			break
		}
	}
	return out
}

func movementProbePacketClasses(header Header, buf []byte, start int, zeroActorPrefixes map[string]int) []MovementProbePacketClass {
	classes := map[string]*movementProbePacketClassAggregate{}
	for i := start; i < len(buf); i++ {
		candidate, ok := movementCandidateRecordAtCodeVersion(header.CodeVersion, buf, i, zeroActorPrefixes)
		if !ok || !movementPropPatternAllowed(header.CodeVersion, candidate.prop) {
			continue
		}
		propID := hex.EncodeToString(candidate.prop)
		primaryClass := movementProbeVectorClass(candidate.primary, true)
		secondaryClass := movementProbeVectorClass(candidate.secondary, candidate.hasSecondary)
		positionLike := movementY11CandidateIsPosition(candidate)
		rotationLike := movementY11CandidateIsRotation(candidate)
		key := strings.Join([]string{
			propID,
			boolKey(candidate.tailZero),
			primaryClass,
			secondaryClass,
			boolKey(candidate.hasQuaternion),
			boolKey(positionLike),
			boolKey(rotationLike),
		}, "|")
		aggregate := classes[key]
		if aggregate == nil {
			aggregate = &movementProbePacketClassAggregate{
				summary: MovementProbePacketClass{
					Key:                 key,
					PropID:              propID,
					TailZero:            candidate.tailZero,
					PrimaryClass:        primaryClass,
					SecondaryClass:      secondaryClass,
					HasQuaternionAt22:   candidate.hasQuaternion,
					CurrentPositionLike: positionLike,
					CurrentRotationLike: rotationLike,
					ExampleOffset:       i,
					ExampleActorID:      hex.EncodeToString(candidate.actor),
				},
				actorCounts:       map[string]int{},
				vectorOffsets:     map[int]int{},
				quaternionOffsets: map[int]int{},
			}
			if movementVectorHasMeaningfulComponent(candidate.primary) {
				primary := candidate.primary
				aggregate.summary.ExamplePrimary = &primary
			}
			if candidate.hasSecondary && movementVectorHasMeaningfulComponent(candidate.secondary) {
				secondary := candidate.secondary
				aggregate.summary.ExampleSecondary = &secondary
			}
			classes[key] = aggregate
		}
		aggregate.summary.SampleCount++
		actorID := hex.EncodeToString(candidate.actor)
		aggregate.actorCounts[actorID]++
		if movementTrackLikeVectorOffsets(buf, i) != nil {
			for _, offset := range movementTrackLikeVectorOffsets(buf, i) {
				aggregate.vectorOffsets[offset]++
			}
		}
		if movementTrackLikeQuaternionOffsets(buf, i) != nil {
			for _, offset := range movementTrackLikeQuaternionOffsets(buf, i) {
				aggregate.quaternionOffsets[offset]++
			}
		}
		if header.CodeVersion < Y11S1Alpha3 {
			i += 37
		}
	}
	summaries := make([]MovementProbePacketClass, 0, len(classes))
	for _, aggregate := range classes {
		aggregate.summary.ActorCount = len(aggregate.actorCounts)
		for _, count := range aggregate.actorCounts {
			if count > aggregate.summary.TopActorSampleCount {
				aggregate.summary.TopActorSampleCount = count
			}
		}
		aggregate.summary.VectorOffsets = movementProbeOffsetCounts(aggregate.vectorOffsets, 4)
		aggregate.summary.QuaternionOffsets = movementProbeOffsetCounts(aggregate.quaternionOffsets, 4)
		summaries = append(summaries, aggregate.summary)
	}
	sort.Slice(summaries, func(i, j int) bool {
		if summaries[i].SampleCount == summaries[j].SampleCount {
			return summaries[i].Key < summaries[j].Key
		}
		return summaries[i].SampleCount > summaries[j].SampleCount
	})
	if len(summaries) > 40 {
		summaries = summaries[:40]
	}
	return summaries
}

func movementProbeVectorClass(value Vector3, has bool) string {
	if !has {
		return "missing"
	}
	if !movementVectorHasMeaningfulComponent(value) {
		return "zero"
	}
	if movementVectorComponentsWithin(value, movementPropAbs95Max) {
		return "bounded"
	}
	if movementVectorComponentsWithin(value, movementPropAbs95Max*10) {
		return "wide"
	}
	return "large"
}

func movementTrackLikeVectorOffsets(buf []byte, propOffset int) []int {
	offsets := make([]int, 0, 4)
	for rel := 22; rel <= 31; rel++ {
		value, ok := movementVectorAt(buf, propOffset+rel)
		if !ok || !movementVectorHasMeaningfulComponent(value) {
			continue
		}
		if !movementVectorComponentsWithin(value, movementPropAbs95Max*5) {
			continue
		}
		offsets = append(offsets, rel)
	}
	return offsets
}

func movementTrackLikeQuaternionOffsets(buf []byte, propOffset int) []int {
	offsets := make([]int, 0, 4)
	for rel := 22; rel <= 27; rel++ {
		if _, ok := movementQuaternionAt(buf, propOffset+rel); ok {
			offsets = append(offsets, rel)
		}
	}
	return offsets
}

func movementProbeOffsetCounts(counts map[int]int, limit int) []MovementProbeOffsetCount {
	if len(counts) == 0 || limit <= 0 {
		return nil
	}
	items := make([]MovementProbeOffsetCount, 0, len(counts))
	for offset, count := range counts {
		items = append(items, MovementProbeOffsetCount{Offset: offset, Count: count})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Count == items[j].Count {
			return items[i].Offset < items[j].Offset
		}
		return items[i].Count > items[j].Count
	})
	if len(items) > limit {
		items = items[:limit]
	}
	return items
}

func boolKey(value bool) string {
	if value {
		return "1"
	}
	return "0"
}

func movementProbeFragments(header Header, buf []byte, start int, zeroActorPrefixes map[string]int, propIDs []string) []movementProbeFragmentCandidate {
	fragments := make([]movementProbeFragmentCandidate, 0)
	seen := map[string]bool{}
	for _, propID := range propIDs {
		prop := decodeMovementPropOrDefault(propID, movementPositionProp)
		tracks, _ := movementTracksForProps(header, nil, buf, start, prop, prop, zeroActorPrefixes, false)
		for _, track := range tracks {
			summary := movementProbeFragmentSummary(propID, track)
			if summary.MergedSamples == 0 {
				continue
			}
			if summary.MergedSamples < 3 && summary.RotationSamples == 0 {
				continue
			}
			key := summary.ID
			if seen[key] {
				continue
			}
			seen[key] = true
			fragments = append(fragments, movementProbeFragmentCandidate{
				summary: summary,
				track:   track,
			})
		}
	}
	sort.Slice(fragments, func(i, j int) bool {
		if fragments[i].summary.Score == fragments[j].summary.Score {
			return fragments[i].summary.ID < fragments[j].summary.ID
		}
		return fragments[i].summary.Score > fragments[j].summary.Score
	})
	return fragments
}

func movementProbeFragmentSummary(propID string, track MovementTrack) MovementProbeFragment {
	effectivePosition := movementTrackEffectivePositionSamples(track)
	merged := effectivePosition + track.RotationSamples
	zRange := 0.0
	if track.Bounds != nil {
		zRange = float64(track.Bounds.Max.Z - track.Bounds.Min.Z)
	}
	summary := MovementProbeFragment{
		ID:                  normalizeMovementPropID(propID) + ":" + track.ActorID,
		PropID:              normalizeMovementPropID(propID),
		ActorID:             track.ActorID,
		PositionSamples:     effectivePosition,
		RotationSamples:     track.RotationSamples,
		MergedSamples:       merged,
		FirstOffset:         track.FirstOffset,
		LastOffset:          track.LastOffset,
		FirstPosition:       track.FirstPosition,
		LastPosition:        track.LastPosition,
		FirstRotation:       track.FirstRotation,
		LastRotation:        track.LastRotation,
		Distance:            track.Distance,
		ZRange:              zRange,
		PositionJumps:       movementTrackPositionJumpCount(track),
		RotationJumps:       movementTrackRotationJumpCount(track),
		RotationSpanDegrees: movementTrackRotationSpanDegrees(track),
		OriginLike:          movementTrackLooksOriginLike(track),
	}
	summary.Score = movementProbeTrackScore(track)
	return summary
}

func movementProbeTrackScore(track MovementTrack) float64 {
	effectivePosition := movementTrackEffectivePositionSamples(track)
	merged := effectivePosition + track.RotationSamples
	if merged == 0 {
		return -1
	}
	zRange := 0.0
	if track.Bounds != nil {
		zRange = float64(track.Bounds.Max.Z - track.Bounds.Min.Z)
	}
	score := float64(effectivePosition*1000000) +
		float64(track.RotationSamples*1000000) +
		float64(merged*1000) +
		track.Distance*5000 +
		zRange*1000 +
		movementTrackRotationSpanDegrees(track)*100 -
		float64(movementTrackPositionJumpCount(track)*20000000) -
		float64(movementTrackRotationJumpCount(track)*20000000)
	if movementTrackLooksOriginLike(track) {
		score -= 400000000
	}
	return score
}

func movementProbeFragmentSummaries(fragments []movementProbeFragmentCandidate, limit int) []MovementProbeFragment {
	if len(fragments) > limit {
		fragments = fragments[:limit]
	}
	out := make([]MovementProbeFragment, 0, len(fragments))
	for _, fragment := range fragments {
		out = append(out, fragment.summary)
	}
	return out
}

func movementProbeFusedCandidates(fragments []movementProbeFragmentCandidate, limit int) []MovementProbeFusedCandidate {
	if len(fragments) == 0 || limit <= 0 {
		return nil
	}
	seeds := fragments
	if len(seeds) > limit {
		seeds = seeds[:limit]
	}
	out := make([]MovementProbeFusedCandidate, 0, len(seeds))
	seen := map[string]bool{}
	for index, seed := range seeds {
		track, memberIDs := movementProbeFuseFromSeed(seed, fragments)
		summary := movementProbeFusedSummary(index+1, track, memberIDs)
		key := strings.Join(memberIDs, "|")
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, summary)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Score == out[j].Score {
			return out[i].ID < out[j].ID
		}
		return out[i].Score > out[j].Score
	})
	if len(out) > limit {
		out = out[:limit]
	}
	for i := range out {
		out[i].ID = "F" + leftPadInt(i+1, 2)
	}
	return out
}

func movementProbeFuseFromSeed(seed movementProbeFragmentCandidate, fragments []movementProbeFragmentCandidate) (MovementTrack, []string) {
	current := seed.track
	currentScore := movementProbeTrackScore(current)
	memberIDs := []string{seed.summary.ID}
	used := map[string]bool{seed.summary.ID: true}
	for {
		bestGain := 0.0
		bestTrack := MovementTrack{}
		bestID := ""
		for _, candidate := range fragments {
			if used[candidate.summary.ID] {
				continue
			}
			if !movementProbeTracksPotentiallyCompatible(current, candidate.track) {
				continue
			}
			merged := movementMergeTracks(current, candidate.track)
			mergedScore := movementProbeTrackScore(merged)
			gain := mergedScore - currentScore
			if gain > bestGain {
				bestGain = gain
				bestTrack = merged
				bestID = candidate.summary.ID
			}
		}
		if bestID == "" {
			break
		}
		current = bestTrack
		currentScore = movementProbeTrackScore(current)
		memberIDs = append(memberIDs, bestID)
		used[bestID] = true
	}
	sort.Strings(memberIDs)
	return current, memberIDs
}

func movementMergeTracks(left MovementTrack, right MovementTrack) MovementTrack {
	samples := append(append([]MovementSample(nil), left.Samples...), right.Samples...)
	sort.Slice(samples, func(i, j int) bool {
		if samples[i].Offset == samples[j].Offset {
			if samples[i].Changed == samples[j].Changed {
				return movementSampleFieldCount(samples[i]) > movementSampleFieldCount(samples[j])
			}
			return samples[i].Changed < samples[j].Changed
		}
		return samples[i].Offset < samples[j].Offset
	})
	deduped := make([]MovementSample, 0, len(samples))
	for _, sample := range samples {
		n := len(deduped)
		if n > 0 && deduped[n-1].Offset == sample.Offset && deduped[n-1].Changed == sample.Changed {
			if movementSampleFieldCount(sample) > movementSampleFieldCount(deduped[n-1]) {
				deduped[n-1] = sample
			}
			continue
		}
		deduped = append(deduped, sample)
	}
	merged := MovementTrack{
		ActorID: left.ActorID + "+" + right.ActorID,
		Samples: movementCarryForwardSamples(deduped),
	}
	for _, sample := range deduped {
		switch sample.Changed {
		case "position":
			merged.PositionSamples++
		case "rotation":
			merged.RotationSamples++
		}
	}
	return enrichMovementTrack(merged)
}

func movementSampleFieldCount(sample MovementSample) int {
	count := 0
	if sample.Position != nil {
		count++
	}
	if sample.Rotation != nil {
		count++
	}
	if sample.RotationDegrees != nil {
		count++
	}
	if sample.TimeInSeconds != nil {
		count++
	}
	return count
}

func movementProbeTracksPotentiallyCompatible(left MovementTrack, right MovementTrack) bool {
	if !movementProbePositionCompatible(left, right) {
		return false
	}
	if !movementProbeRotationCompatible(left, right) {
		return false
	}
	return true
}

func movementProbePositionCompatible(left MovementTrack, right MovementTrack) bool {
	leftPositions := movementTrackChangedPositionSamples(left)
	rightPositions := movementTrackChangedPositionSamples(right)
	if len(leftPositions) == 0 || len(rightPositions) == 0 {
		return true
	}
	if count, median := movementProbePositionOverlap(leftPositions, rightPositions, 16384); count >= 3 {
		return median <= 8
	}
	distance := movementProbeBoundaryPositionDistance(left, right)
	if distance < 0 {
		return true
	}
	return distance <= 8
}

func movementProbeRotationCompatible(left MovementTrack, right MovementTrack) bool {
	leftRotations := movementTrackChangedRotationSamples(left)
	rightRotations := movementTrackChangedRotationSamples(right)
	if len(leftRotations) == 0 || len(rightRotations) == 0 {
		return true
	}
	if count, median := movementProbeRotationOverlap(leftRotations, rightRotations, 16384); count >= 3 {
		return median <= 60
	}
	return true
}

func movementTrackChangedPositionSamples(track MovementTrack) []MovementSample {
	out := make([]MovementSample, 0, track.PositionSamples)
	for _, sample := range track.Samples {
		if sample.Changed == "position" && sample.Position != nil {
			out = append(out, sample)
		}
	}
	return out
}

func movementTrackChangedRotationSamples(track MovementTrack) []MovementSample {
	out := make([]MovementSample, 0, track.RotationSamples)
	for _, sample := range track.Samples {
		if sample.Changed == "rotation" && sample.Rotation != nil {
			out = append(out, sample)
		}
	}
	return out
}

func movementProbePositionOverlap(left []MovementSample, right []MovementSample, window int) (int, float64) {
	distances := make([]float64, 0)
	for _, sample := range left {
		match, ok := movementNearestMovementSampleByOffset(right, sample.Offset, window)
		if !ok || sample.Position == nil || match.Position == nil {
			continue
		}
		distances = append(distances, vectorDistance(*sample.Position, *match.Position))
	}
	return len(distances), movementMedian(distances)
}

func movementProbeRotationOverlap(left []MovementSample, right []MovementSample, window int) (int, float64) {
	distances := make([]float64, 0)
	for _, sample := range left {
		match, ok := movementNearestMovementSampleByOffset(right, sample.Offset, window)
		if !ok || sample.Rotation == nil || match.Rotation == nil {
			continue
		}
		distances = append(distances, movementRotationDifferenceDegrees(*sample.Rotation, *match.Rotation))
	}
	return len(distances), movementMedian(distances)
}

func movementNearestMovementSampleByOffset(samples []MovementSample, offset int, window int) (MovementSample, bool) {
	bestIndex := -1
	bestDistance := window + 1
	for i, sample := range samples {
		distance := sample.Offset - offset
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
		return MovementSample{}, false
	}
	return samples[bestIndex], true
}

func movementProbeBoundaryPositionDistance(left MovementTrack, right MovementTrack) float64 {
	if left.LastPosition == nil || right.FirstPosition == nil || right.LastPosition == nil || left.FirstPosition == nil {
		return -1
	}
	if left.LastOffset <= right.FirstOffset {
		return vectorDistance(*left.LastPosition, *right.FirstPosition)
	}
	if right.LastOffset <= left.FirstOffset {
		return vectorDistance(*right.LastPosition, *left.FirstPosition)
	}
	return -1
}

func movementRotationDifferenceDegrees(left Vector3, right Vector3) float64 {
	return maxFloat64(
		absFloat64(float64(radiansToWrappedDegrees(left.X)-radiansToWrappedDegrees(right.X))),
		absFloat64(float64(radiansToWrappedDegrees(left.Y)-radiansToWrappedDegrees(right.Y))),
		absFloat64(float64(radiansToWrappedDegrees(left.Z)-radiansToWrappedDegrees(right.Z))),
	)
}

func movementMedian(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	middle := len(sorted) / 2
	if len(sorted)%2 == 1 {
		return sorted[middle]
	}
	return (sorted[middle-1] + sorted[middle]) / 2
}

func movementProbeFusedSummary(index int, track MovementTrack, memberIDs []string) MovementProbeFusedCandidate {
	zRange := 0.0
	if track.Bounds != nil {
		zRange = float64(track.Bounds.Max.Z - track.Bounds.Min.Z)
	}
	return MovementProbeFusedCandidate{
		ID:                  "F" + leftPadInt(index, 2),
		FragmentIDs:         memberIDs,
		PositionSamples:     movementTrackEffectivePositionSamples(track),
		RotationSamples:     track.RotationSamples,
		MergedSamples:       movementTrackEffectivePositionSamples(track) + track.RotationSamples,
		FirstOffset:         track.FirstOffset,
		LastOffset:          track.LastOffset,
		FirstPosition:       track.FirstPosition,
		LastPosition:        track.LastPosition,
		FirstRotation:       track.FirstRotation,
		LastRotation:        track.LastRotation,
		Distance:            track.Distance,
		ZRange:              zRange,
		PositionJumps:       movementTrackPositionJumpCount(track),
		RotationJumps:       movementTrackRotationJumpCount(track),
		RotationSpanDegrees: movementTrackRotationSpanDegrees(track),
		Score:               movementProbeTrackScore(track),
	}
}

func leftPadInt(value int, width int) string {
	text := strconv.Itoa(value)
	for len(text) < width {
		text = "0" + text
	}
	return text
}
