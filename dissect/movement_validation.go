package dissect

import (
	"math"
	"sort"
)

type MovementValidation struct {
	TrackChecks []MovementTrackValidation `json:"trackChecks,omitempty"`
}

type MovementTrackValidation struct {
	ActorID              string  `json:"actorID"`
	Label                string  `json:"label,omitempty"`
	PlayerNameGuess      string  `json:"playerNameGuess,omitempty"`
	StepSamples          int     `json:"stepSamples"`
	HeadingSourceGuess   string  `json:"headingSourceGuess,omitempty"`
	HeadingAxisGuess     string  `json:"headingAxisGuess,omitempty"`
	HeadingSignGuess     int     `json:"headingSignGuess,omitempty"`
	HeadingOffsetDegrees float64 `json:"headingOffsetDegrees,omitempty"`
	MeanErrorDegrees     float64 `json:"meanErrorDegrees,omitempty"`
	MedianErrorDegrees   float64 `json:"medianErrorDegrees,omitempty"`
	P90ErrorDegrees      float64 `json:"p90ErrorDegrees,omitempty"`
	MeanCosineAgreement  float64 `json:"meanCosineAgreement,omitempty"`
	MakesSense           bool    `json:"makesSense,omitempty"`
}

type movementDirectionSample struct {
	movementAngleDegrees float64
	rotationDegrees      Vector3
	viewingDegrees       *float64
	hasRotation          bool
}

type movementDirectionPoint struct {
	position                Vector3
	rotation                Vector3
	viewingDirectionDegrees *float64
	hasRotation             bool
	timeInSeconds           *float64
}

func movementValidation(tracks []MovementTrack) *MovementValidation {
	checks := make([]MovementTrackValidation, 0, len(tracks))
	for _, track := range tracks {
		check, ok := movementTrackValidation(track)
		if ok {
			checks = append(checks, check)
		}
	}
	if len(checks) == 0 {
		return nil
	}
	return &MovementValidation{TrackChecks: checks}
}

func movementTrackValidation(track MovementTrack) (MovementTrackValidation, bool) {
	samples := movementDirectionSamples(track)
	if len(samples) < 12 {
		return MovementTrackValidation{}, false
	}
	best := MovementTrackValidation{
		ActorID:         track.ActorID,
		Label:           track.Label,
		PlayerNameGuess: track.PlayerNameGuess,
		StepSamples:     len(samples),
	}
	bestMetric := -2.0
	consider := func(source string, axisName string, sign int, project func(movementDirectionSample) (float64, bool)) {
		usable := 0
		offset := movementCircularMeanDegrees(samples, func(sample movementDirectionSample) float64 {
			projected, ok := project(sample)
			if !ok {
				return 0
			}
			usable++
			return sample.movementAngleDegrees - projected
		})
		if usable < 12 {
			return
		}
		errors := make([]float64, 0, len(samples))
		cosineSum := 0.0
		for _, sample := range samples {
			projected, ok := project(sample)
			if !ok {
				continue
			}
			predicted := projected + offset
			diff := movementWrapDegrees(sample.movementAngleDegrees - predicted)
			errors = append(errors, math.Abs(diff))
			cosineSum += math.Cos(diff * math.Pi / 180)
		}
		if len(errors) < 12 {
			return
		}
		meanCosine := cosineSum / float64(len(errors))
		meanError := movementMean(errors)
		median := movementPercentile(errors, 0.50)
		p90 := movementPercentile(errors, 0.90)
		metric := meanCosine - (meanError / 360)
		if metric <= bestMetric {
			return
		}
		bestMetric = metric
		best.HeadingSourceGuess = source
		best.HeadingAxisGuess = axisName
		best.HeadingSignGuess = sign
		best.HeadingOffsetDegrees = offset
		best.MeanErrorDegrees = meanError
		best.MedianErrorDegrees = median
		best.P90ErrorDegrees = p90
		best.MeanCosineAgreement = meanCosine
	}
	consider("derived-heading", "derived", 1, func(sample movementDirectionSample) (float64, bool) {
		if sample.viewingDegrees == nil {
			return 0, false
		}
		return *sample.viewingDegrees, true
	})
	for _, axis := range []struct {
		name string
		read func(Vector3) float64
	}{
		{name: "x", read: func(value Vector3) float64 { return float64(radiansToWrappedDegrees(value.X)) }},
		{name: "y", read: func(value Vector3) float64 { return float64(radiansToWrappedDegrees(value.Y)) }},
		{name: "z", read: func(value Vector3) float64 { return float64(radiansToWrappedDegrees(value.Z)) }},
	} {
		for _, sign := range []int{1, -1} {
			consider("euler-axis", axis.name, sign, func(sample movementDirectionSample) (float64, bool) {
				if !sample.hasRotation {
					return 0, false
				}
				return float64(sign) * axis.read(sample.rotationDegrees), true
			})
		}
	}
	for _, forward := range []struct {
		name string
		axis Vector3
	}{
		{name: "+x", axis: Vector3{X: 1}},
		{name: "-x", axis: Vector3{X: -1}},
		{name: "+y", axis: Vector3{Y: 1}},
		{name: "-y", axis: Vector3{Y: -1}},
		{name: "+z", axis: Vector3{Z: 1}},
		{name: "-z", axis: Vector3{Z: -1}},
	} {
		consider("forward-vector", forward.name, 1, func(sample movementDirectionSample) (float64, bool) {
			if !sample.hasRotation {
				return 0, false
			}
			quat := movementQuaternionFromEuler(sample.rotationDegrees)
			world := movementRotateVector(quat, forward.axis)
			return math.Atan2(float64(world.Y), float64(world.X)) * 180 / math.Pi, true
		})
	}
	best.MakesSense = best.MeanCosineAgreement >= 0.6 && best.MedianErrorDegrees <= 45
	return best, true
}

func movementDirectionSamples(track MovementTrack) []movementDirectionSample {
	points := make([]movementDirectionPoint, 0, track.PositionSamples)
	for _, sample := range track.Samples {
		if sample.Changed == "position" && sample.Position != nil && (sample.Rotation != nil || sample.ViewingDirectionDegrees != nil) {
			rotation := Vector3{}
			hasRotation := sample.Rotation != nil
			if sample.Rotation != nil {
				rotation = *sample.Rotation
			}
			points = append(points, movementDirectionPoint{
				position:                *sample.Position,
				rotation:                rotation,
				viewingDirectionDegrees: sample.ViewingDirectionDegrees,
				hasRotation:             hasRotation,
				timeInSeconds:           sample.TimeInSeconds,
			})
		}
	}
	if len(points) < 2 {
		return nil
	}
	if movementDirectionPointsTimed(points) {
		return movementTimedDirectionSamples(points)
	}
	gap := 4
	if len(points) <= gap {
		gap = 1
	}
	samples := make([]movementDirectionSample, 0, len(points)-gap)
	for i := gap; i < len(points); i++ {
		previous := points[i-gap]
		current := points[i]
		dx := float64(current.position.X - previous.position.X)
		dy := float64(current.position.Y - previous.position.Y)
		if math.Hypot(dx, dy) < 0.15 {
			continue
		}
		samples = append(samples, movementDirectionSample{
			movementAngleDegrees: math.Atan2(dy, dx) * 180 / math.Pi,
			rotationDegrees:      current.rotation,
			viewingDegrees:       current.viewingDirectionDegrees,
			hasRotation:          current.hasRotation,
		})
	}
	return samples
}

func movementDirectionPointsTimed(points []movementDirectionPoint) bool {
	for _, point := range points {
		if point.timeInSeconds == nil {
			return false
		}
	}
	return len(points) > 0
}

func movementTimedDirectionSamples(points []movementDirectionPoint) []movementDirectionSample {
	const targetGapSeconds = 0.14
	samples := make([]movementDirectionSample, 0, len(points))
	for currentIndex := 1; currentIndex < len(points); currentIndex++ {
		previousIndex := movementTimedDirectionPreviousIndex(points, currentIndex, targetGapSeconds)
		if previousIndex == -1 {
			continue
		}
		previous := points[previousIndex]
		current := points[currentIndex]
		dx := float64(current.position.X - previous.position.X)
		dy := float64(current.position.Y - previous.position.Y)
		if math.Hypot(dx, dy) < 0.15 {
			continue
		}
		samples = append(samples, movementDirectionSample{
			movementAngleDegrees: math.Atan2(dy, dx) * 180 / math.Pi,
			rotationDegrees:      current.rotation,
			viewingDegrees:       current.viewingDirectionDegrees,
			hasRotation:          current.hasRotation,
		})
	}
	return samples
}

func movementTimedDirectionPreviousIndex(points []movementDirectionPoint, currentIndex int, targetGapSeconds float64) int {
	if currentIndex <= 0 || currentIndex >= len(points) {
		return -1
	}
	current := points[currentIndex]
	if current.timeInSeconds == nil {
		return -1
	}
	bestIndex := -1
	bestGap := math.Inf(1)
	for previousIndex := currentIndex - 1; previousIndex >= 0; previousIndex-- {
		previous := points[previousIndex]
		if previous.timeInSeconds == nil {
			continue
		}
		gap := *previous.timeInSeconds - *current.timeInSeconds
		if gap < targetGapSeconds {
			continue
		}
		if gap < bestGap {
			bestGap = gap
			bestIndex = previousIndex
		}
	}
	return bestIndex
}

func movementCircularMeanDegrees(values []movementDirectionSample, project func(movementDirectionSample) float64) float64 {
	sinSum := 0.0
	cosSum := 0.0
	for _, value := range values {
		degrees := project(value)
		radians := degrees * math.Pi / 180
		sinSum += math.Sin(radians)
		cosSum += math.Cos(radians)
	}
	if sinSum == 0 && cosSum == 0 {
		return 0
	}
	return math.Atan2(sinSum, cosSum) * 180 / math.Pi
}

func movementWrapDegrees(value float64) float64 {
	wrapped := math.Mod(value, 360)
	if wrapped <= -180 {
		wrapped += 360
	}
	if wrapped > 180 {
		wrapped -= 360
	}
	return wrapped
}

func movementMean(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	total := 0.0
	for _, value := range values {
		total += value
	}
	return total / float64(len(values))
}

func movementPercentile(values []float64, percentile float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	index := int(float64(len(sorted)-1) * percentile)
	return sorted[index]
}

func movementQuaternionFromEuler(rotation Vector3) movementQuaternion {
	roll := float64(rotation.X)
	pitch := float64(rotation.Y)
	yaw := float64(rotation.Z)
	cy := math.Cos(yaw * 0.5)
	sy := math.Sin(yaw * 0.5)
	cp := math.Cos(pitch * 0.5)
	sp := math.Sin(pitch * 0.5)
	cr := math.Cos(roll * 0.5)
	sr := math.Sin(roll * 0.5)
	return movementQuaternion{
		W: float32((cr * cp * cy) + (sr * sp * sy)),
		X: float32((sr * cp * cy) - (cr * sp * sy)),
		Y: float32((cr * sp * cy) + (sr * cp * sy)),
		Z: float32((cr * cp * sy) - (sr * sp * cy)),
	}
}

func movementRotateVector(quat movementQuaternion, vector Vector3) Vector3 {
	qx := float64(quat.X)
	qy := float64(quat.Y)
	qz := float64(quat.Z)
	qw := float64(quat.W)
	vx := float64(vector.X)
	vy := float64(vector.Y)
	vz := float64(vector.Z)
	tx := 2 * (qy*vz - qz*vy)
	ty := 2 * (qz*vx - qx*vz)
	tz := 2 * (qx*vy - qy*vx)
	return Vector3{
		X: float32(vx + (qw * tx) + (qy * tz) - (qz * ty)),
		Y: float32(vy + (qw * ty) + (qz * tx) - (qx * tz)),
		Z: float32(vz + (qw * tz) + (qx * ty) - (qy * tx)),
	}
}
