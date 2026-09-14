package pipeline

import (
	"context"

	"github.com/google/uuid"

	"thinkboard-backend/internal/shared/kernel"
)

// runAnalytic is the analytic stage stub: one LLM call, wrapped as a single root Point. Real
// multi-point decomposition depends on internal/llm's actual response contract (build order
// step 6) and is deferred until that package exists.
func runAnalytic(ctx context.Context, llm LLM, runID kernel.RunID, content string) ([]Point, error) {
	out, err := llm.Complete(ctx, content)
	if err != nil {
		return nil, err
	}
	p, err := NewPoint(kernel.PointID(uuid.NewString()), runID, "", out, 0)
	if err != nil {
		return nil, err
	}
	return []Point{p}, nil
}
