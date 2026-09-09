//go:build !linux

package continuity

import (
	"context"
	"fmt"
	"heimdall/internal/model"
)

func artifactHost() (string, error) {
	return "", fmt.Errorf("artifact observation currently requires Linux")
}
func ObserveArtifact(context.Context, model.Resource, string, bool) (model.ArtifactObservation, error) {
	return model.ArtifactObservation{}, fmt.Errorf("artifact observation currently requires Linux")
}
