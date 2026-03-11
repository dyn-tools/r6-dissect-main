package dissect

import (
	"math"
	"os"
	"testing"
)

func TestMovementDirectionRegression360PatternReplay(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping replay regression in short mode")
	}
	f, err := os.Open("../replays/360_pattern.rec")
	if err != nil {
		t.Skipf("360 pattern replay not available: %v", err)
	}
	defer f.Close()
	r, err := NewReader(f)
	if err != nil {
		t.Fatalf("new reader: %v", err)
	}
	out, err := r.MovementDataWithPlayersOptions(MovementOptions{})
	if err != nil {
		t.Fatalf("movement data: %v", err)
	}
	if len(out.PrimaryTracks) == 0 {
		t.Fatal("expected primary track")
	}
	samples := out.PrimaryTracks[0].Samples
	maxTime := 0.0
	for _, sample := range samples {
		if sample.TimeInSeconds != nil && *sample.TimeInSeconds > maxTime {
			maxTime = *sample.TimeInSeconds
		}
	}
	yawSpan, yawChanges, yawCount := sectionHeadingStats(samples, maxTime, 17.6, 20.8)
	pitchSpan, _, pitchCount := sectionHeadingStats(samples, maxTime, 21.1, 24.2)
	if yawCount < 40 {
		t.Fatalf("expected substantial heading coverage in standing yaw section, got %d", yawCount)
	}
	if yawSpan < 80 || yawChanges < 4 {
		t.Fatalf("expected real standing-yaw variation (span>=80, changes>=4), got span=%f changes=%d", yawSpan, yawChanges)
	}
	if pitchCount < 30 {
		t.Fatalf("expected heading coverage in standing pitch section, got %d", pitchCount)
	}
	if pitchSpan > 45 {
		t.Fatalf("expected mostly stable yaw during pitch-only section, got span=%f", pitchSpan)
	}
}

func sectionHeadingStats(samples []MovementSample, maxTime float64, startSec float64, endSec float64) (span float64, changes int, count int) {
	angles := make([]float64, 0)
	for _, sample := range samples {
		if sample.TimeInSeconds == nil || sample.ViewingDirectionDegrees == nil {
			continue
		}
		rel := maxTime - *sample.TimeInSeconds
		if rel < startSec || rel > endSec {
			continue
		}
		angles = append(angles, *sample.ViewingDirectionDegrees)
	}
	if len(angles) == 0 {
		return 0, 0, 0
	}
	unwrapped := make([]float64, len(angles))
	unwrapped[0] = angles[0]
	for i := 1; i < len(angles); i++ {
		value := angles[i]
		for value-unwrapped[i-1] > 180 {
			value -= 360
		}
		for value-unwrapped[i-1] < -180 {
			value += 360
		}
		unwrapped[i] = value
		if math.Abs(unwrapped[i]-unwrapped[i-1]) > 1 {
			changes++
		}
	}
	minValue := unwrapped[0]
	maxValue := unwrapped[0]
	for _, value := range unwrapped[1:] {
		if value < minValue {
			minValue = value
		}
		if value > maxValue {
			maxValue = value
		}
	}
	return maxValue - minValue, changes, len(unwrapped)
}
