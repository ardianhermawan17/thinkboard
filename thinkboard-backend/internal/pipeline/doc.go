// Package pipeline owns the write side: the 7-stage orchestration and streaming (03-backend-folder-architecture.md §5.1).
// Stage/Order/Allowed (stage.go, transition.go) are implemented; orchestration (run.go, service.go, stage handlers) is not yet.
package pipeline
