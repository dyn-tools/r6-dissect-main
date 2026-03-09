package dissect

import (
	"encoding/binary"
	"fmt"
	"math"
	"sort"
)

var (
	movementClockEntryPattern   = []byte{0x17, 0x84, 0xE4, 0x09}
	movementClockSecondsPattern = []byte{0x1F, 0x07, 0xEF, 0xC9, 0x04}
	movementClockMillisPattern  = []byte{0x22, 0x18, 0x37, 0x46, 0x6C, 0x04}
)

type MovementClock struct {
	Source             string  `json:"source,omitempty"`
	EntryCount         int     `json:"entryCount"`
	AnchorCount        int     `json:"anchorCount"`
	AppliedSamples     int     `json:"appliedSamples"`
	CoveragePercent    float64 `json:"coveragePercent,omitempty"`
	FirstMilliseconds  int     `json:"firstMillisecondsRemaining,omitempty"`
	LastMilliseconds   int     `json:"lastMillisecondsRemaining,omitempty"`
	EstimatedTickRate  float64 `json:"estimatedTickRate,omitempty"`
	Mapping            string  `json:"mapping,omitempty"`
	TrimmedLeading     int     `json:"trimmedLeadingEntries,omitempty"`
	TrimmedTrailing    int     `json:"trimmedTrailingEntries,omitempty"`
	MonotonicSamples   bool    `json:"monotonicSamples,omitempty"`
	MedianStepMs       int     `json:"medianStepMilliseconds,omitempty"`
	P90StepMs          int     `json:"p90StepMilliseconds,omitempty"`
	DuplicateStepCount int     `json:"duplicateStepCount,omitempty"`
}

type movementClockEntry struct {
	index        int
	offset       int
	milliseconds int
}

type movementClockAnchor struct {
	entryIndex   int
	milliseconds int
	seconds      int
}

type movementSampleRef struct {
	track   int
	actorID string
	index   int
	offset  int
}

type movementClockStats struct {
	monotonic      bool
	medianStepMs   int
	p90StepMs      int
	duplicateSteps int
}

func movementAttachRecoveredClock(codeVersion int, buf []byte, ordered []MovementTrack, primary []MovementTrack) ([]MovementTrack, []MovementTrack, *MovementClock) {
	if codeVersion < Y11S1Alpha3 {
		return ordered, primary, nil
	}
	sampleCount := movementTrackSampleCount(ordered)
	if sampleCount == 0 || movementTimedSampleCount(ordered) > 0 {
		return ordered, primary, nil
	}
	entries, anchors, trimmedLeading, trimmedTrailing, ok := movementRecoveredClockEntries(buf)
	if !ok || len(entries) < 16 || len(anchors) < 2 {
		return ordered, primary, nil
	}
	refs := movementAllSampleRefs(ordered)
	if len(refs) == 0 {
		return ordered, primary, nil
	}
	sampleMilliseconds := make([]int, len(refs))
	mapping := "offset-interpolation"
	for sampleRank := range refs {
		milliseconds, ok := movementRecoveredMillisecondsAtOffset(entries, refs[sampleRank].offset)
		if !ok {
			entryIndex := movementClockEntryIndex(sampleRank, len(refs), len(entries))
			milliseconds = entries[entryIndex].milliseconds
		}
		sampleMilliseconds[sampleRank] = milliseconds
		sample := &ordered[refs[sampleRank].track].Samples[refs[sampleRank].index]
		movementAssignRecoveredTime(sample, milliseconds)
	}
	stats := movementClockSampleStats(sampleMilliseconds)
	if (len(sampleMilliseconds) >= 2 && sampleMilliseconds[0] <= sampleMilliseconds[len(sampleMilliseconds)-1]) || (stats.duplicateSteps > len(sampleMilliseconds)/2) {
		mapping = "sample-order"
		for sampleRank := range refs {
			entryIndex := movementClockEntryIndex(sampleRank, len(refs), len(entries))
			milliseconds := entries[entryIndex].milliseconds
			sampleMilliseconds[sampleRank] = milliseconds
			sample := &ordered[refs[sampleRank].track].Samples[refs[sampleRank].index]
			movementAssignRecoveredTime(sample, milliseconds)
		}
		stats = movementClockSampleStats(sampleMilliseconds)
	}
	primary = movementPrimaryTracksFromOrdered(ordered, primary)
	coverage := movementPercent(len(refs), len(entries))
	return ordered, primary, &MovementClock{
		Source:             "tail-index-ms",
		EntryCount:         len(entries),
		AnchorCount:        len(anchors),
		AppliedSamples:     len(refs),
		CoveragePercent:    coverage,
		FirstMilliseconds:  entries[0].milliseconds,
		LastMilliseconds:   entries[len(entries)-1].milliseconds,
		EstimatedTickRate:  movementRecoveredClockRate(entries),
		Mapping:            mapping,
		TrimmedLeading:     trimmedLeading,
		TrimmedTrailing:    trimmedTrailing,
		MonotonicSamples:   stats.monotonic,
		MedianStepMs:       stats.medianStepMs,
		P90StepMs:          stats.p90StepMs,
		DuplicateStepCount: stats.duplicateSteps,
	}
}

func movementRecoveredClockEntries(buf []byte) ([]movementClockEntry, []movementClockAnchor, int, int, bool) {
	starts := movementClockEntryStarts(buf)
	if len(starts) < 3 {
		return nil, nil, 0, 0, false
	}
	anchors := make([]movementClockAnchor, 0, len(starts))
	for index, start := range starts {
		end := len(buf)
		if index+1 < len(starts) {
			end = starts[index+1]
		}
		segment := buf[start:end]
		seconds, hasSeconds := movementFieldUint32(segment, movementClockSecondsPattern)
		milliseconds, hasMilliseconds := movementFieldUint32(segment, movementClockMillisPattern)
		if !hasMilliseconds || milliseconds == 0 {
			continue
		}
		anchor := movementClockAnchor{
			entryIndex:   index,
			milliseconds: int(milliseconds),
		}
		if hasSeconds {
			anchor.seconds = int(seconds)
		} else {
			anchor.seconds = int(milliseconds / 1000)
		}
		anchors = append(anchors, anchor)
	}
	anchors = movementFilterClockAnchors(anchors)
	if len(anchors) < 2 {
		return nil, nil, 0, 0, false
	}
	entries := make([]movementClockEntry, len(starts))
	for index := range starts {
		entries[index] = movementClockEntry{
			index:        index,
			offset:       starts[index],
			milliseconds: movementInterpolateClockMilliseconds(index, anchors),
		}
	}
	trimmedLeading := 0
	trimmedTrailing := 0
	entries, trimmedLeading, trimmedTrailing = movementTrimClockEntries(entries)
	if len(entries) < 2 {
		return nil, nil, 0, 0, false
	}
	return entries, anchors, trimmedLeading, trimmedTrailing, true
}

func movementClockEntryStarts(buf []byte) []int {
	starts := make([]int, 0, 1024)
	for offset := 0; offset <= len(buf)-len(movementClockEntryPattern); offset++ {
		if !movementMatchAt(buf, offset, movementClockEntryPattern) {
			continue
		}
		starts = append(starts, offset)
	}
	return starts
}

func movementFieldUint32(buf []byte, pattern []byte) (uint32, bool) {
	if len(buf) < len(pattern)+4 {
		return 0, false
	}
	for offset := 0; offset <= len(buf)-len(pattern)-4; offset++ {
		if !movementMatchAt(buf, offset, pattern) {
			continue
		}
		return binary.LittleEndian.Uint32(buf[offset+len(pattern) : offset+len(pattern)+4]), true
	}
	return 0, false
}

func movementFilterClockAnchors(anchors []movementClockAnchor) []movementClockAnchor {
	if len(anchors) < 2 {
		return anchors
	}
	best := make([]movementClockAnchor, 0, len(anchors))
	for start := range anchors {
		chain := make([]movementClockAnchor, 0, len(anchors)-start)
		for _, anchor := range anchors[start:] {
			if anchor.milliseconds <= 0 {
				continue
			}
			if len(chain) == 0 {
				chain = append(chain, anchor)
				continue
			}
			previous := chain[len(chain)-1]
			if anchor.entryIndex <= previous.entryIndex {
				continue
			}
			if anchor.milliseconds >= previous.milliseconds {
				continue
			}
			delta := previous.milliseconds - anchor.milliseconds
			if delta < 800 || delta > 1200 {
				continue
			}
			chain = append(chain, anchor)
		}
		if len(chain) > len(best) {
			best = chain
		}
	}
	return best
}

func movementInterpolateClockMilliseconds(index int, anchors []movementClockAnchor) int {
	if index <= anchors[0].entryIndex {
		return movementExtrapolateMilliseconds(index, anchors[0], anchors[1])
	}
	last := anchors[len(anchors)-1]
	if index >= last.entryIndex {
		return movementExtrapolateMilliseconds(index, anchors[len(anchors)-2], last)
	}
	for anchorIndex := 0; anchorIndex < len(anchors)-1; anchorIndex++ {
		left := anchors[anchorIndex]
		right := anchors[anchorIndex+1]
		if index < left.entryIndex || index > right.entryIndex {
			continue
		}
		return movementExtrapolateMilliseconds(index, left, right)
	}
	return anchors[len(anchors)-1].milliseconds
}

func movementExtrapolateMilliseconds(index int, left movementClockAnchor, right movementClockAnchor) int {
	if right.entryIndex == left.entryIndex {
		return left.milliseconds
	}
	progress := float64(index-left.entryIndex) / float64(right.entryIndex-left.entryIndex)
	value := float64(left.milliseconds) + progress*float64(right.milliseconds-left.milliseconds)
	if value < 0 {
		value = 0
	}
	return int(math.Round(value))
}

func movementRecoveredClockRate(entries []movementClockEntry) float64 {
	if len(entries) < 2 {
		return 0
	}
	spanMs := entries[0].milliseconds - entries[len(entries)-1].milliseconds
	if spanMs <= 0 {
		return 0
	}
	seconds := float64(spanMs) / 1000
	if seconds <= 0 {
		return 0
	}
	return float64(len(entries)-1) / seconds
}

func movementAllSampleRefs(tracks []MovementTrack) []movementSampleRef {
	refs := make([]movementSampleRef, 0, movementTrackSampleCount(tracks))
	for trackIndex := range tracks {
		for sampleIndex := range tracks[trackIndex].Samples {
			refs = append(refs, movementSampleRef{
				track:   trackIndex,
				actorID: tracks[trackIndex].ActorID,
				index:   sampleIndex,
				offset:  tracks[trackIndex].Samples[sampleIndex].Offset,
			})
		}
	}
	sort.SliceStable(refs, func(i, j int) bool {
		if refs[i].offset == refs[j].offset {
			if refs[i].actorID == refs[j].actorID {
				return refs[i].index < refs[j].index
			}
			return refs[i].actorID < refs[j].actorID
		}
		return refs[i].offset < refs[j].offset
	})
	return refs
}

func movementClockEntryIndex(sampleRank int, sampleCount int, entryCount int) int {
	if sampleCount <= 1 || entryCount <= 1 {
		return 0
	}
	ratio := float64(entryCount-1) / float64(sampleCount-1)
	index := int(math.Round(float64(sampleRank) * ratio))
	if index < 0 {
		return 0
	}
	if index >= entryCount {
		return entryCount - 1
	}
	return index
}

func movementRecoveredMillisecondsAtOffset(entries []movementClockEntry, offset int) (int, bool) {
	if len(entries) == 0 {
		return 0, false
	}
	index := sort.Search(len(entries), func(i int) bool {
		return entries[i].offset >= offset
	})
	if index == 0 {
		return entries[0].milliseconds, true
	}
	if index >= len(entries) {
		return entries[len(entries)-1].milliseconds, true
	}
	left := entries[index-1]
	right := entries[index]
	if left.offset == right.offset {
		return left.milliseconds, true
	}
	ratio := float64(offset-left.offset) / float64(right.offset-left.offset)
	value := float64(left.milliseconds) + ratio*float64(right.milliseconds-left.milliseconds)
	return int(math.Round(value)), true
}

func movementAssignRecoveredTime(sample *MovementSample, milliseconds int) {
	seconds := float64(milliseconds) / 1000
	sample.TimeInSeconds = &seconds
	sample.Time = movementFormatMilliseconds(milliseconds)
}

func movementFormatMilliseconds(milliseconds int) string {
	if milliseconds < 0 {
		milliseconds = 0
	}
	totalSeconds := milliseconds / 1000
	return fmt.Sprintf("%d:%02d.%03d", totalSeconds/60, totalSeconds%60, milliseconds%1000)
}

func movementTimedSampleCount(tracks []MovementTrack) int {
	count := 0
	for _, track := range tracks {
		for _, sample := range track.Samples {
			if sample.TimeInSeconds != nil {
				count++
			}
		}
	}
	return count
}

func movementPrimaryTracksFromOrdered(ordered []MovementTrack, primary []MovementTrack) []MovementTrack {
	if len(primary) == 0 {
		return primary
	}
	byActor := make(map[string]MovementTrack, len(ordered))
	for _, track := range ordered {
		byActor[track.ActorID] = track
	}
	out := make([]MovementTrack, 0, len(primary))
	for _, track := range primary {
		if updated, ok := byActor[track.ActorID]; ok {
			out = append(out, updated)
			continue
		}
		out = append(out, track)
	}
	return out
}

func movementTrimClockEntries(entries []movementClockEntry) ([]movementClockEntry, int, int) {
	if len(entries) == 0 {
		return nil, 0, 0
	}
	first := -1
	last := -1
	for index, entry := range entries {
		if entry.milliseconds <= 0 {
			continue
		}
		if first == -1 {
			first = index
		}
		last = index
	}
	if first == -1 || last == -1 || first > last {
		return nil, len(entries), 0
	}
	trimmed := append([]movementClockEntry(nil), entries[first:last+1]...)
	return trimmed, first, len(entries) - last - 1
}

func movementClockSampleStats(sampleMilliseconds []int) movementClockStats {
	if len(sampleMilliseconds) < 2 {
		return movementClockStats{monotonic: len(sampleMilliseconds) > 0}
	}
	steps := make([]float64, 0, len(sampleMilliseconds)-1)
	duplicates := 0
	monotonic := true
	for index := 1; index < len(sampleMilliseconds); index++ {
		step := sampleMilliseconds[index-1] - sampleMilliseconds[index]
		if step < 0 {
			monotonic = false
			continue
		}
		if step == 0 {
			duplicates++
		}
		steps = append(steps, float64(step))
	}
	if len(steps) == 0 {
		return movementClockStats{
			monotonic:      monotonic,
			duplicateSteps: duplicates,
		}
	}
	return movementClockStats{
		monotonic:      monotonic,
		medianStepMs:   int(math.Round(movementPercentile(steps, 0.50))),
		p90StepMs:      int(math.Round(movementPercentile(steps, 0.90))),
		duplicateSteps: duplicates,
	}
}
