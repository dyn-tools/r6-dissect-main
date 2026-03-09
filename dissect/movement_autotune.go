package dissect

import "fmt"

func movementAutoTuneSinglePlayerRotation(header Header, feedback []MatchUpdate, buf []byte, start int, positionProp []byte, rotationProp []byte, zeroActorPrefixes map[string]int, ordered []MovementTrack, primary []MovementTrack) ([]MovementTrack, []MovementTrack) {
	if len(header.Players) != 1 || len(primary) == 0 {
		return ordered, primary
	}
	if len(buf) > 3_000_000 {
		return ordered, primary
	}
	bestOrdered := ordered
	bestPrimary := primary
	bestScore := movementValidationScore(primary)
	if bestScore >= 0.6 {
		return bestOrdered, bestPrimary
	}
	for _, model := range movementImplicitQuaternionModels() {
		if model == movementDefaultImplicitQuaternionModel {
			continue
		}
		candidateOrdered, candidatePrimary := movementTracksForPropsWithModel(header, feedback, buf, start, positionProp, rotationProp, zeroActorPrefixes, true, model)
		score := movementValidationScore(candidatePrimary)
		if score <= bestScore {
			continue
		}
		bestOrdered = candidateOrdered
		bestPrimary = candidatePrimary
		bestScore = score
	}
	return bestOrdered, bestPrimary
}

func movementValidationScore(tracks []MovementTrack) float64 {
	validation := movementValidation(tracks)
	if validation == nil || len(validation.TrackChecks) == 0 {
		return -1
	}
	check := validation.TrackChecks[0]
	return check.MeanCosineAgreement - (check.MeanErrorDegrees / 360)
}

func movementImplicitQuaternionModels() []movementImplicitQuaternionModel {
	permutations := [][3]int{
		{0, 1, 2},
		{0, 2, 1},
		{1, 0, 2},
		{1, 2, 0},
		{2, 0, 1},
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
	modes := []string{"implicit_w", "tangent_half", "axis_angle"}
	models := make([]movementImplicitQuaternionModel, 0, len(permutations)*len(signs)*len(modes))
	for _, order := range permutations {
		for _, sign := range signs {
			for _, mode := range modes {
				models = append(models, movementImplicitQuaternionModel{
					mode:  mode,
					order: order,
					signs: sign,
				})
			}
		}
	}
	return models
}

func movementImplicitQuaternionModelKey(model movementImplicitQuaternionModel) string {
	return fmt.Sprintf(
		"%s:%d%d%d:%d%d%d",
		model.mode,
		model.order[0],
		model.order[1],
		model.order[2],
		int(model.signs[0]),
		int(model.signs[1]),
		int(model.signs[2]),
	)
}
