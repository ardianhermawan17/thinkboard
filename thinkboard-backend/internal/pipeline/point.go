package pipeline

import (
	"errors"

	"thinkboard-backend/internal/shared/kernel"
)

// ErrEmptyPointContent is returned by NewPoint when content is empty.
var ErrEmptyPointContent = errors.New("pipeline: point content must not be empty")

// ErrPointSelfParent is returned by NewPoint when a point is given as its own parent.
var ErrPointSelfParent = errors.New("pipeline: point cannot be its own parent")

// Point is a node in the run's point tree (thinkboard-schema-final.sql points, parent_point_id
// self-reference). ParentPointID's zero value means the point is a root (no parent).
type Point struct {
	ID            kernel.PointID
	RunID         kernel.RunID
	ParentPointID kernel.PointID
	Content       string
	Position      int
}

// NewPoint validates content is non-empty and that a point is not its own parent.
func NewPoint(id kernel.PointID, runID kernel.RunID, parentPointID kernel.PointID, content string, position int) (Point, error) {
	if content == "" {
		return Point{}, ErrEmptyPointContent
	}
	if parentPointID != "" && parentPointID == id {
		return Point{}, ErrPointSelfParent
	}
	return Point{ID: id, RunID: runID, ParentPointID: parentPointID, Content: content, Position: position}, nil
}

// IsRoot reports whether the point has no parent.
func (p Point) IsRoot() bool {
	return p.ParentPointID == ""
}
