package pipeline

import "context"

// LLM is the subset of the future internal/llm.Service (Complete/Stream) that pipeline stage
// handlers call. Retriever, Memory, and Personas join this file once a stage needs them.
type LLM interface {
	Complete(ctx context.Context, prompt string) (string, error)
}
