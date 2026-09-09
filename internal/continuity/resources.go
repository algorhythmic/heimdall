package continuity

import (
	"context"
	"heimdall/internal/model"
	"heimdall/internal/resourceobs"
)

const MaxResourceFiles = resourceobs.MaxResourceFiles
const MaxResourceBytes = resourceobs.MaxResourceBytes

func normalizeResource(r *model.Resource) error { return resourceobs.Normalize(r) }
func Observe(ctx context.Context, r model.Resource) (model.Snapshot, error) {
	return resourceobs.Observe(ctx, r)
}
