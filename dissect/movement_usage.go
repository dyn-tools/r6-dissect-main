package dissect

import (
	"encoding/hex"
	"strconv"
)

type MovementUsage struct {
	TotalCandidatePackets        int     `json:"totalCandidatePackets"`
	TotalCandidateActors         int     `json:"totalCandidateActors,omitempty"`
	SelectedPropPackets          int     `json:"selectedPropPackets"`
	SelectedPropActors           int     `json:"selectedPropActors,omitempty"`
	PositionPropPackets          int     `json:"positionPropPackets,omitempty"`
	RotationPropPackets          int     `json:"rotationPropPackets,omitempty"`
	ExportedSamples              int     `json:"exportedSamples"`
	ExportedTrackCount           int     `json:"exportedTrackCount,omitempty"`
	PrimarySamples               int     `json:"primarySamples,omitempty"`
	PrimaryTrackCount            int     `json:"primaryTrackCount,omitempty"`
	SupplementalDirectionPackets int     `json:"supplementalDirectionPackets,omitempty"`
	SupplementalDirectionActors  int     `json:"supplementalDirectionActors,omitempty"`
	EffectiveCandidatePackets    int     `json:"effectiveCandidatePackets,omitempty"`
	EffectiveCandidateActors     int     `json:"effectiveCandidateActors,omitempty"`
	ReplayUsagePercent           float64 `json:"replayUsagePercent"`
	SelectedPropUsagePercent     float64 `json:"selectedPropUsagePercent"`
	PrimaryTrackUsagePercent     float64 `json:"primaryTrackUsagePercent,omitempty"`
	PrimarySharePercent          float64 `json:"primarySharePercent,omitempty"`
	EffectiveReplayUsagePercent  float64 `json:"effectiveReplayUsagePercent,omitempty"`
}

func movementUsageStats(codeVersion int, buf []byte, start int, zeroActorPrefixes map[string]int, positionProp []byte, rotationProp []byte, ordered []MovementTrack, primary []MovementTrack, direction movementDirectionSearchResult) *MovementUsage {
	totalActors := map[string]bool{}
	selectedActors := map[string]bool{}
	effectiveActors := map[string]bool{}
	selectedPackets := map[string]bool{}
	totalCandidatePackets := 0
	selectedPropPackets := 0
	positionPropPackets := 0
	rotationPropPackets := 0
	positionPropID := hex.EncodeToString(positionProp)
	rotationPropID := hex.EncodeToString(rotationProp)

	for i := start; i < len(buf); i++ {
		candidate, ok := movementCandidateRecordAtCodeVersion(codeVersion, buf, i, zeroActorPrefixes)
		if !ok || !movementPropPatternAllowed(codeVersion, candidate.prop) {
			continue
		}
		totalCandidatePackets++
		actorID := hex.EncodeToString(candidate.actor)
		totalActors[actorID] = true
		propID := hex.EncodeToString(candidate.prop)
		packetKey := movementPacketKey(propID, actorID, i)
		matchedPosition := propID == positionPropID
		matchedRotation := propID == rotationPropID
		if matchedPosition {
			positionPropPackets++
		}
		if matchedRotation {
			rotationPropPackets++
		}
		if matchedPosition || matchedRotation {
			selectedPropPackets++
			selectedActors[actorID] = true
			effectiveActors[actorID] = true
			selectedPackets[packetKey] = true
		}
		if codeVersion < Y11S1Alpha3 {
			i += 37
		}
	}

	exportedSamples := movementTrackSampleCount(ordered)
	primarySamples := movementTrackSampleCount(primary)
	if totalCandidatePackets == 0 && exportedSamples == 0 {
		return nil
	}
	for actorID := range direction.supplementalActors {
		effectiveActors[actorID] = true
	}
	effectivePackets := len(selectedPackets)
	for packetKey := range direction.supplementalPackets {
		selectedPackets[packetKey] = true
	}
	effectivePackets = len(selectedPackets)

	return &MovementUsage{
		TotalCandidatePackets:        totalCandidatePackets,
		TotalCandidateActors:         len(totalActors),
		SelectedPropPackets:          selectedPropPackets,
		SelectedPropActors:           len(selectedActors),
		PositionPropPackets:          positionPropPackets,
		RotationPropPackets:          rotationPropPackets,
		ExportedSamples:              exportedSamples,
		ExportedTrackCount:           len(ordered),
		PrimarySamples:               primarySamples,
		PrimaryTrackCount:            len(primary),
		SupplementalDirectionPackets: len(direction.supplementalPackets),
		SupplementalDirectionActors:  len(direction.supplementalActors),
		EffectiveCandidatePackets:    effectivePackets,
		EffectiveCandidateActors:     len(effectiveActors),
		ReplayUsagePercent:           movementPercent(exportedSamples, totalCandidatePackets),
		SelectedPropUsagePercent:     movementPercent(exportedSamples, selectedPropPackets),
		PrimaryTrackUsagePercent:     movementPercent(primarySamples, selectedPropPackets),
		PrimarySharePercent:          movementPercent(primarySamples, exportedSamples),
		EffectiveReplayUsagePercent:  movementPercent(effectivePackets, totalCandidatePackets),
	}
}

func movementTrackSampleCount(tracks []MovementTrack) int {
	total := 0
	for _, track := range tracks {
		total += len(track.Samples)
	}
	return total
}

func movementPercent(numerator int, denominator int) float64 {
	if denominator <= 0 {
		return 0
	}
	return (float64(numerator) * 100) / float64(denominator)
}

func movementPacketKey(propID string, actorID string, offset int) string {
	return propID + "|" + actorID + "|" + strconv.Itoa(offset)
}
