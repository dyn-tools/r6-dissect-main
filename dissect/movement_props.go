package dissect

import (
	"encoding/hex"
	"sort"
	"strings"
)

const movementValueAbsMin = 0.001
const movementPropAbs95Min = 1
const movementPropAbs95Max = 1000
const movementRotationAbs95Min = 1
const movementRotationAbs95Max = 720

type MovementOptions struct {
	PositionPropID string
	RotationPropID string
}

type MovementPropCandidate struct {
	PropID              string  `json:"propID"`
	SampleCount         int     `json:"sampleCount"`
	ActorCount          int     `json:"actorCount"`
	StrongActorCount    int     `json:"strongActorCount"`
	TopActorSampleCount int     `json:"topActorSampleCount"`
	PositionLikeSamples int     `json:"positionLikeSamples,omitempty"`
	RotationLikeSamples int     `json:"rotationLikeSamples,omitempty"`
	Abs95               float64 `json:"abs95"`
	Score               float64 `json:"score"`
	OverlapWithPosition int     `json:"overlapWithPosition,omitempty"`
	Selected            bool    `json:"selected,omitempty"`
}

type movementPropSelection struct {
	positionProp       []byte
	rotationProp       []byte
	positionCandidates []MovementPropCandidate
	rotationCandidates []MovementPropCandidate
}

type movementPropSummary struct {
	candidate MovementPropCandidate
	topActors []string
}

type movementPropStats struct {
	prop               []byte
	sampleCount        int
	positionLikeCount  int
	rotationLikeCount  int
	actorCounts        map[string]int
	positionActorCount map[string]int
	rotationActorCount map[string]int
	absValues          []float64
}

func discoverMovementPropSelection(header Header, buf []byte, start int, zeroActorPrefixes map[string]int, options MovementOptions) movementPropSelection {
	targetActors := len(header.Players)
	if targetActors == 0 {
		targetActors = 10
	}
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
	if len(statsByProp) == 0 {
		return movementPropSelection{
			positionProp: append([]byte(nil), movementPositionProp...),
			rotationProp: append([]byte(nil), movementRotationProp...),
		}
	}
	positionSummaries := make([]movementPropSummary, 0, len(statsByProp))
	for _, stats := range statsByProp {
		positionSummaries = append(positionSummaries, summarizeMovementProp(stats, targetActors))
	}
	for i := range positionSummaries {
		positionSummaries[i].candidate.Score = movementPositionScore(positionSummaries[i].candidate)
	}
	sort.Slice(positionSummaries, func(i, j int) bool {
		if positionSummaries[i].candidate.Score == positionSummaries[j].candidate.Score {
			return positionSummaries[i].candidate.PropID < positionSummaries[j].candidate.PropID
		}
		return positionSummaries[i].candidate.Score > positionSummaries[j].candidate.Score
	})
	if header.CodeVersion >= Y11S1Alpha3 {
		return discoverY11MovementPropSelection(header, buf, start, zeroActorPrefixes, options, positionSummaries)
	}
	selectedPosition := selectMovementPropOverride(positionSummaries, options.PositionPropID)
	if selectedPosition == nil {
		for i := range positionSummaries {
			candidate := positionSummaries[i].candidate
			if candidate.Score <= 0 {
				continue
			}
			selectedPosition = &positionSummaries[i]
			break
		}
	}
	if selectedPosition == nil {
		selectedPosition = &movementPropSummary{candidate: MovementPropCandidate{PropID: hex.EncodeToString(movementPositionProp)}, topActors: nil}
	}
	rotationSummaries := make([]movementPropSummary, 0, len(statsByProp))
	for _, summary := range positionSummaries {
		candidate := summary.candidate
		candidate.OverlapWithPosition = movementActorOverlap(selectedPosition.topActors, summary.topActors)
		candidate.Score = movementRotationScore(selectedPosition.candidate, candidate)
		rotationSummaries = append(rotationSummaries, movementPropSummary{candidate: candidate, topActors: summary.topActors})
	}
	sort.Slice(rotationSummaries, func(i, j int) bool {
		if rotationSummaries[i].candidate.Score == rotationSummaries[j].candidate.Score {
			return rotationSummaries[i].candidate.PropID < rotationSummaries[j].candidate.PropID
		}
		return rotationSummaries[i].candidate.Score > rotationSummaries[j].candidate.Score
	})
	selectedRotation := selectMovementPropOverride(rotationSummaries, options.RotationPropID)
	if selectedRotation == nil {
		for i := range rotationSummaries {
			candidate := rotationSummaries[i].candidate
			if candidate.PropID == selectedPosition.candidate.PropID {
				continue
			}
			if candidate.Score <= 0 {
				continue
			}
			selectedRotation = &rotationSummaries[i]
			break
		}
	}
	if selectedRotation == nil {
		selectedRotation = &movementPropSummary{candidate: MovementPropCandidate{PropID: hex.EncodeToString(movementRotationProp)}, topActors: nil}
	}
	markSelectedMovementProp(positionSummaries, selectedPosition.candidate.PropID)
	markSelectedMovementProp(rotationSummaries, selectedRotation.candidate.PropID)
	return movementPropSelection{
		positionProp:       decodeMovementPropOrDefault(selectedPosition.candidate.PropID, movementPositionProp),
		rotationProp:       decodeMovementPropOrDefault(selectedRotation.candidate.PropID, movementRotationProp),
		positionCandidates: limitMovementCandidates(positionSummaries, 8),
		rotationCandidates: limitMovementCandidates(rotationSummaries, 8),
	}
}

func discoverY11MovementPropSelection(header Header, buf []byte, start int, zeroActorPrefixes map[string]int, options MovementOptions, positionSummaries []movementPropSummary) movementPropSelection {
	targetActors := len(header.Players)
	poolLimit := 16
	positionChoiceLimit := 4
	rotationPoolLimit := 24
	rotationChoiceLimit := 8
	if targetActors == 1 {
		poolLimit = 24
		positionChoiceLimit = 6
		rotationPoolLimit = 32
		rotationChoiceLimit = 12
	}
	pool := movementY11PropPool(positionSummaries, poolLimit)
	positionOverride := normalizeMovementPropID(options.PositionPropID)
	rotationOverride := normalizeMovementPropID(options.RotationPropID)
	cache := map[string]movementTrackQuality{}
	evaluate := func(positionID string, rotationID string) movementTrackQuality {
		key := positionID + ":" + rotationID
		if quality, ok := cache[key]; ok {
			return quality
		}
		quality := evaluateMovementTrackQuality(header, buf, start, zeroActorPrefixes, decodeMovementPropOrDefault(positionID, movementPositionProp), decodeMovementPropOrDefault(rotationID, movementRotationProp))
		cache[key] = quality
		return quality
	}
	selfScores := make([]movementY11ScoredProp, 0, len(pool))
	for _, propID := range pool {
		quality := evaluate(propID, propID)
		selfScores = append(selfScores, movementY11ScoredProp{
			propID:        propID,
			quality:       quality,
			posScore:      movementTrackQualityPositionScore(targetActors, quality),
			rotScore:      movementTrackQualityRotationScore(targetActors, quality),
			combinedScore: movementTrackQualityScore(targetActors, quality),
		})
	}
	positionChoices := pool
	rotationChoices := pool
	if positionOverride == "" {
		positionChoices = movementY11RankPositionChoices(selfScores, positionChoiceLimit)
	} else {
		positionChoices = []string{positionOverride}
	}
	if rotationOverride == "" {
		rotationPool := movementY11CandidateIDs(positionSummaries, "", movementY11PropPool(positionSummaries, rotationPoolLimit), rotationPoolLimit)
		rotationChoices = movementY11RankRotationChoices(targetActors, positionChoices, rotationPool, evaluate, rotationChoiceLimit)
	} else {
		rotationChoices = []string{rotationOverride}
	}
	if len(positionChoices) == 0 {
		positionChoices = []string{hex.EncodeToString(movementPositionProp)}
	}
	if len(rotationChoices) == 0 {
		rotationChoices = []string{hex.EncodeToString(movementRotationProp)}
	}
	selectedPositionID := positionChoices[0]
	selectedRotationID := rotationChoices[0]
	bestScore := -1.0
	for _, positionID := range positionChoices {
		for _, rotationID := range rotationChoices {
			quality := evaluate(positionID, rotationID)
			score := movementTrackQualityScore(len(header.Players), quality)
			if score > bestScore {
				bestScore = score
				selectedPositionID = positionID
				selectedRotationID = rotationID
			}
		}
	}
	positionCandidates := scoreMovementCandidateSet(positionSummaries, movementY11CandidateIDs(positionSummaries, selectedPositionID, positionChoices, 8), func(propID string) movementTrackQuality {
		return evaluate(propID, selectedRotationID)
	}, func(quality movementTrackQuality) float64 {
		return movementTrackQualityScore(len(header.Players), quality)
	}, selectedPositionID)
	rotationCandidates := scoreMovementCandidateSet(positionSummaries, movementY11CandidateIDs(positionSummaries, selectedRotationID, rotationChoices, 8), func(propID string) movementTrackQuality {
		return evaluate(selectedPositionID, propID)
	}, func(quality movementTrackQuality) float64 {
		return movementTrackQualityScore(len(header.Players), quality)
	}, selectedRotationID)
	return movementPropSelection{
		positionProp:       decodeMovementPropOrDefault(selectedPositionID, movementPositionProp),
		rotationProp:       decodeMovementPropOrDefault(selectedRotationID, movementRotationProp),
		positionCandidates: limitMovementCandidates(positionCandidates, 8),
		rotationCandidates: limitMovementCandidates(rotationCandidates, 8),
	}
}

type movementY11ScoredProp struct {
	propID        string
	quality       movementTrackQuality
	posScore      float64
	rotScore      float64
	combinedScore float64
	bestPosition  string
}

func movementY11RankPositionChoices(selfScores []movementY11ScoredProp, limit int) []string {
	if limit <= 0 || len(selfScores) == 0 {
		return nil
	}
	scored := append([]movementY11ScoredProp(nil), selfScores...)
	sort.Slice(scored, func(i, j int) bool {
		if scored[i].posScore == scored[j].posScore {
			if scored[i].combinedScore == scored[j].combinedScore {
				return scored[i].propID < scored[j].propID
			}
			return scored[i].combinedScore > scored[j].combinedScore
		}
		return scored[i].posScore > scored[j].posScore
	})
	choices := make([]string, 0, limit)
	for _, candidate := range scored {
		choices = append(choices, candidate.propID)
		if len(choices) >= limit {
			break
		}
	}
	return choices
}

func movementY11RankRotationChoices(targetActors int, positionChoices []string, rotationPool []string, evaluate func(string, string) movementTrackQuality, limit int) []string {
	if limit <= 0 || len(rotationPool) == 0 {
		return nil
	}
	if len(positionChoices) == 0 {
		return append([]string(nil), rotationPool[:minInt(limit, len(rotationPool))]...)
	}
	scored := make([]movementY11ScoredProp, 0, len(rotationPool))
	for _, rotationID := range rotationPool {
		best := movementY11ScoredProp{
			propID:        rotationID,
			rotScore:      -1,
			combinedScore: -1,
		}
		for _, positionID := range positionChoices {
			quality := evaluate(positionID, rotationID)
			rotScore := movementTrackQualityRotationScore(targetActors, quality)
			combinedScore := movementTrackQualityScore(targetActors, quality)
			if rotScore > best.rotScore || (rotScore == best.rotScore && combinedScore > best.combinedScore) {
				best.quality = quality
				best.rotScore = rotScore
				best.combinedScore = combinedScore
				best.bestPosition = positionID
			}
		}
		scored = append(scored, best)
	}
	sort.Slice(scored, func(i, j int) bool {
		if scored[i].rotScore == scored[j].rotScore {
			if scored[i].combinedScore == scored[j].combinedScore {
				return scored[i].propID < scored[j].propID
			}
			return scored[i].combinedScore > scored[j].combinedScore
		}
		return scored[i].rotScore > scored[j].rotScore
	})
	choices := make([]string, 0, limit)
	for _, candidate := range scored {
		choices = append(choices, candidate.propID)
		if len(choices) >= limit {
			break
		}
	}
	return choices
}

func movementY11CandidateIDs(summaries []movementPropSummary, selectedID string, preferred []string, limit int) []string {
	ids := make([]string, 0, limit)
	seen := map[string]bool{}
	add := func(propID string) {
		propID = normalizeMovementPropID(propID)
		if propID == "" || seen[propID] {
			return
		}
		seen[propID] = true
		ids = append(ids, propID)
	}
	add(selectedID)
	for _, propID := range preferred {
		add(propID)
		if len(ids) >= limit {
			return ids
		}
	}
	for _, summary := range summaries {
		add(summary.candidate.PropID)
		if len(ids) >= limit {
			return ids
		}
	}
	return ids
}

func scoreMovementCandidateSet(summaries []movementPropSummary, ids []string, evaluate func(string) movementTrackQuality, score func(movementTrackQuality) float64, selectedID string) []movementPropSummary {
	if len(ids) == 0 {
		return nil
	}
	byID := map[string]movementPropSummary{}
	for _, summary := range summaries {
		propID := normalizeMovementPropID(summary.candidate.PropID)
		if propID == "" {
			continue
		}
		if _, ok := byID[propID]; ok {
			continue
		}
		byID[propID] = summary
	}
	scored := make([]movementPropSummary, 0, len(ids))
	for _, propID := range ids {
		summary, ok := byID[propID]
		if !ok {
			summary = movementPropSummary{candidate: MovementPropCandidate{PropID: propID}}
		}
		quality := evaluate(propID)
		summary.candidate.PropID = propID
		summary.candidate.Score = score(quality)
		summary.candidate.Selected = propID == selectedID
		scored = append(scored, summary)
	}
	sort.Slice(scored, func(i, j int) bool {
		if scored[i].candidate.Score == scored[j].candidate.Score {
			return scored[i].candidate.PropID < scored[j].candidate.PropID
		}
		return scored[i].candidate.Score > scored[j].candidate.Score
	})
	return scored
}

func movementY11PropPool(summaries []movementPropSummary, limit int) []string {
	if limit <= 0 || len(summaries) == 0 {
		return nil
	}
	pool := make([]string, 0, limit)
	seen := map[string]bool{}
	add := func(summary movementPropSummary, allowZeroScore bool) bool {
		propID := normalizeMovementPropID(summary.candidate.PropID)
		if propID == "" || seen[propID] {
			return false
		}
		if !allowZeroScore && summary.candidate.Score <= 0 {
			return false
		}
		if allowZeroScore && summary.candidate.SampleCount == 0 {
			return false
		}
		seen[propID] = true
		pool = append(pool, propID)
		return len(pool) >= limit
	}
	passLists := [][]movementPropSummary{
		append([]movementPropSummary(nil), summaries...),
		append([]movementPropSummary(nil), summaries...),
		append([]movementPropSummary(nil), summaries...),
		append([]movementPropSummary(nil), summaries...),
		append([]movementPropSummary(nil), summaries...),
	}
	sort.Slice(passLists[1], func(i, j int) bool {
		if passLists[1][i].candidate.SampleCount == passLists[1][j].candidate.SampleCount {
			return passLists[1][i].candidate.PropID < passLists[1][j].candidate.PropID
		}
		return passLists[1][i].candidate.SampleCount > passLists[1][j].candidate.SampleCount
	})
	sort.Slice(passLists[2], func(i, j int) bool {
		if passLists[2][i].candidate.TopActorSampleCount == passLists[2][j].candidate.TopActorSampleCount {
			return passLists[2][i].candidate.PropID < passLists[2][j].candidate.PropID
		}
		return passLists[2][i].candidate.TopActorSampleCount > passLists[2][j].candidate.TopActorSampleCount
	})
	sort.Slice(passLists[3], func(i, j int) bool {
		if passLists[3][i].candidate.PositionLikeSamples == passLists[3][j].candidate.PositionLikeSamples {
			return passLists[3][i].candidate.PropID < passLists[3][j].candidate.PropID
		}
		return passLists[3][i].candidate.PositionLikeSamples > passLists[3][j].candidate.PositionLikeSamples
	})
	sort.Slice(passLists[4], func(i, j int) bool {
		if passLists[4][i].candidate.RotationLikeSamples == passLists[4][j].candidate.RotationLikeSamples {
			return passLists[4][i].candidate.PropID < passLists[4][j].candidate.PropID
		}
		return passLists[4][i].candidate.RotationLikeSamples > passLists[4][j].candidate.RotationLikeSamples
	})
	maxLen := 0
	for _, list := range passLists {
		if len(list) > maxLen {
			maxLen = len(list)
		}
	}
	for index := 0; index < maxLen; index++ {
		for listIndex, list := range passLists {
			if index >= len(list) {
				continue
			}
			if add(list[index], listIndex != 0) {
				return pool
			}
		}
	}
	return pool
}

type movementTrackQuality struct {
	longTracks          int
	balancedTracks      int
	originLikeTop       int
	topMerged           int
	topPosition         int
	topRotation         int
	dominantMerged      int
	dominantGap         int
	dominantZRange      float64
	dominantDistance    float64
	positionJumps       int
	rotationJumps       int
	rotationSpanDegrees float64
}

func evaluateMovementTrackQuality(header Header, buf []byte, start int, zeroActorPrefixes map[string]int, positionProp []byte, rotationProp []byte) movementTrackQuality {
	ordered, _ := movementTracksForProps(header, nil, buf, start, positionProp, rotationProp, zeroActorPrefixes, false)
	targetActors := len(header.Players)
	if targetActors == 0 {
		targetActors = 10
	}
	quality := movementTrackQuality{}
	for i, track := range ordered {
		effectivePosition := movementTrackEffectivePositionSamples(track)
		effectiveMerged := effectivePosition + track.RotationSamples
		if effectivePosition >= movementPrimaryTrackMinPositions {
			quality.longTracks++
		}
		if effectivePosition >= 5 && track.RotationSamples >= 5 {
			quality.balancedTracks++
		}
		if i < targetActors {
			quality.topMerged += effectiveMerged
			quality.topPosition += effectivePosition
			quality.topRotation += track.RotationSamples
			if movementTrackLooksOriginLike(track) {
				quality.originLikeTop++
			}
		}
	}
	if len(ordered) > 0 {
		top := ordered[0]
		topPosition := movementTrackEffectivePositionSamples(top)
		quality.dominantMerged = topPosition + top.RotationSamples
		if len(ordered) > 1 {
			second := movementTrackEffectivePositionSamples(ordered[1]) + ordered[1].RotationSamples
			quality.dominantGap = quality.dominantMerged - second
		} else {
			quality.dominantGap = quality.dominantMerged
		}
		quality.dominantDistance = top.Distance
		if top.Bounds != nil {
			quality.dominantZRange = float64(top.Bounds.Max.Z - top.Bounds.Min.Z)
		}
		quality.positionJumps = movementTrackPositionJumpCount(top)
		quality.rotationJumps = movementTrackRotationJumpCount(top)
		quality.rotationSpanDegrees = movementTrackRotationSpanDegrees(top)
	}
	return quality
}

func movementTrackQualityPositionScore(targetActors int, quality movementTrackQuality) float64 {
	if targetActors == 1 {
		return float64(quality.topPosition*10000000) +
			float64(quality.dominantGap*500000) +
			quality.dominantDistance*5000 +
			quality.dominantZRange*1000 +
			float64(quality.topMerged*100) -
			float64(quality.positionJumps*20000000) -
			float64(quality.rotationJumps*2000000) -
			float64(quality.originLikeTop*500000000)
	}
	if targetActors <= 0 {
		targetActors = 10
	}
	balanced := quality.balancedTracks
	if balanced > targetActors {
		balanced = targetActors
	}
	longTracks := quality.longTracks
	if longTracks > targetActors {
		longTracks = targetActors
	}
	return float64(quality.topPosition*1000000) + float64(longTracks*100000000) + float64(balanced*5000000) + float64(quality.topMerged*100) + float64(quality.topRotation) - float64(quality.originLikeTop*500000000)
}

func movementTrackQualityRotationScore(targetActors int, quality movementTrackQuality) float64 {
	if targetActors == 1 {
		return float64(quality.topRotation*10000000) +
			quality.rotationSpanDegrees*50000 +
			float64(quality.dominantGap*500000) +
			float64(quality.topMerged*100) -
			float64(quality.rotationJumps*20000000) -
			float64(quality.positionJumps*2000000) -
			float64(quality.originLikeTop*100000000)
	}
	if targetActors <= 0 {
		targetActors = 10
	}
	balanced := quality.balancedTracks
	if balanced > targetActors {
		balanced = targetActors
	}
	longTracks := quality.longTracks
	if longTracks > targetActors {
		longTracks = targetActors
	}
	return float64(quality.topRotation*1000000) + float64(balanced*100000000) + float64(longTracks*10000000) + float64(quality.topMerged*100) + float64(quality.topPosition) - float64(quality.originLikeTop*100000000)
}

func movementTrackQualityScore(targetActors int, quality movementTrackQuality) float64 {
	if targetActors == 1 {
		return float64(quality.topPosition*4000000) +
			float64(quality.topRotation*4000000) +
			float64(quality.dominantGap*500000) +
			quality.rotationSpanDegrees*25000 +
			quality.dominantDistance*2000 +
			quality.dominantZRange*500 +
			float64(quality.topMerged*100) -
			float64(quality.positionJumps*20000000) -
			float64(quality.rotationJumps*20000000) -
			float64(quality.originLikeTop*400000000)
	}
	if targetActors <= 0 {
		targetActors = 10
	}
	balanced := quality.balancedTracks
	if balanced > targetActors {
		balanced = targetActors
	}
	longTracks := quality.longTracks
	if longTracks > targetActors {
		longTracks = targetActors
	}
	return float64(quality.topPosition*1000000) + float64(quality.topRotation*1000000) + float64(longTracks*100000000) + float64(balanced*20000000) + float64(quality.topMerged*100) - float64(quality.originLikeTop*400000000)
}
func summarizeMovementProp(stats *movementPropStats, targetActors int) movementPropSummary {
	topActors := make([]string, 0, len(stats.actorCounts))
	for actorID := range stats.actorCounts {
		topActors = append(topActors, actorID)
	}
	sort.Slice(topActors, func(i, j int) bool {
		left := stats.actorCounts[topActors[i]]
		right := stats.actorCounts[topActors[j]]
		if left == right {
			return topActors[i] < topActors[j]
		}
		return left > right
	})
	if len(topActors) > targetActors {
		topActors = append([]string(nil), topActors[:targetActors]...)
	}
	strongActors := 0
	topActorSampleCount := 0
	for actorID, count := range stats.actorCounts {
		if count >= movementPrimaryTrackMinPositions {
			strongActors++
		}
		if containsString(topActors, actorID) {
			topActorSampleCount += count
		}
	}
	values := append([]float64(nil), stats.absValues...)
	sort.Float64s(values)
	abs95 := 0.0
	if len(values) > 0 {
		index := int(float64(len(values)-1) * 0.95)
		abs95 = values[index]
	}
	return movementPropSummary{
		candidate: MovementPropCandidate{
			PropID:              hex.EncodeToString(stats.prop),
			SampleCount:         stats.sampleCount,
			ActorCount:          len(stats.actorCounts),
			StrongActorCount:    strongActors,
			TopActorSampleCount: topActorSampleCount,
			PositionLikeSamples: stats.positionLikeCount,
			RotationLikeSamples: stats.rotationLikeCount,
			Abs95:               abs95,
		},
		topActors: topActors,
	}
}

func appendMovementAbsValues(dst []float64, value Vector3) []float64 {
	for _, component := range []float64{absFloat64(float64(value.X)), absFloat64(float64(value.Y)), absFloat64(float64(value.Z))} {
		if component < movementValueAbsMin {
			continue
		}
		dst = append(dst, component)
	}
	return dst
}

func movementPositionScore(candidate MovementPropCandidate) float64 {
	if candidate.Abs95 < movementPropAbs95Min || candidate.Abs95 > movementPropAbs95Max {
		return -1
	}
	if candidate.TopActorSampleCount == 0 {
		return -1
	}
	return float64(candidate.StrongActorCount*100000) + float64(candidate.TopActorSampleCount)
}

func movementRotationScore(position MovementPropCandidate, candidate MovementPropCandidate) float64 {
	if candidate.Abs95 < movementRotationAbs95Min || candidate.Abs95 > movementRotationAbs95Max {
		return -1
	}
	if candidate.TopActorSampleCount == 0 || candidate.OverlapWithPosition == 0 {
		return -1
	}
	score := float64(candidate.OverlapWithPosition*100000) + float64(candidate.TopActorSampleCount)
	if position.Abs95 > 0 && candidate.Abs95 > position.Abs95*2 {
		score /= 2
	}
	return score
}

func selectMovementPropOverride(summaries []movementPropSummary, propID string) *movementPropSummary {
	propID = normalizeMovementPropID(propID)
	if propID == "" {
		return nil
	}
	for i := range summaries {
		if summaries[i].candidate.PropID == propID {
			return &summaries[i]
		}
	}
	return nil
}

func normalizeMovementPropID(propID string) string {
	if propID == "" {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(propID))
}

func decodeMovementPropOrDefault(propID string, fallback []byte) []byte {
	decoded, err := hex.DecodeString(normalizeMovementPropID(propID))
	if err != nil || len(decoded) != len(fallback) {
		return append([]byte(nil), fallback...)
	}
	return decoded
}

func markSelectedMovementProp(summaries []movementPropSummary, propID string) {
	for i := range summaries {
		summaries[i].candidate.Selected = summaries[i].candidate.PropID == propID
	}
}

func limitMovementCandidates(summaries []movementPropSummary, limit int) []MovementPropCandidate {
	if len(summaries) > limit {
		summaries = summaries[:limit]
	}
	out := make([]MovementPropCandidate, 0, len(summaries))
	for _, summary := range summaries {
		out = append(out, summary.candidate)
	}
	return out
}

func movementActorOverlap(left []string, right []string) int {
	if len(left) == 0 || len(right) == 0 {
		return 0
	}
	seen := map[string]bool{}
	for _, actorID := range left {
		seen[actorID] = true
	}
	overlap := 0
	for _, actorID := range right {
		if seen[actorID] {
			overlap++
		}
	}
	return overlap
}

func movementPropPatternAllowed(codeVersion int, prop []byte) bool {
	if len(prop) != 12 {
		return false
	}
	if codeVersion >= Y11S1Alpha3 {
		return prop[0] == 0 && prop[1] == 0 && prop[7] == 0xF0 && prop[8] == 0 && prop[9] == 0 && prop[10] == 0 && prop[11] == 0
	}
	return prop[0] == 0 && prop[1] == 0 && prop[2] == 0 && prop[3] == 0
}

func absFloat64(value float64) float64 {
	if value < 0 {
		return -value
	}
	return value
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
