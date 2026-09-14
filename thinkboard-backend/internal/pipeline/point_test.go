package pipeline

import (
	"testing"

	"thinkboard-backend/internal/shared/kernel"
)

func TestNewPoint(t *testing.T) {
	runID := kernel.RunID("run-1")

	t.Run("root point", func(t *testing.T) {
		p, err := NewPoint(kernel.PointID("p1"), runID, "", "atomic point", 0)
		if err != nil {
			t.Fatalf("NewPoint() unexpected error %v", err)
		}
		if !p.IsRoot() {
			t.Errorf("IsRoot() = false, want true for empty ParentPointID")
		}
	})

	t.Run("child point", func(t *testing.T) {
		p, err := NewPoint(kernel.PointID("p2"), runID, kernel.PointID("p1"), "child point", 1)
		if err != nil {
			t.Fatalf("NewPoint() unexpected error %v", err)
		}
		if p.IsRoot() {
			t.Errorf("IsRoot() = true, want false for set ParentPointID")
		}
		if p.ParentPointID != kernel.PointID("p1") {
			t.Errorf("ParentPointID = %q, want %q", p.ParentPointID, "p1")
		}
	})

	t.Run("empty content rejected", func(t *testing.T) {
		if _, err := NewPoint(kernel.PointID("p1"), runID, "", "", 0); err != ErrEmptyPointContent {
			t.Errorf("NewPoint() error = %v, want ErrEmptyPointContent", err)
		}
	})

	t.Run("self parent rejected", func(t *testing.T) {
		if _, err := NewPoint(kernel.PointID("p1"), runID, kernel.PointID("p1"), "content", 0); err != ErrPointSelfParent {
			t.Errorf("NewPoint() error = %v, want ErrPointSelfParent", err)
		}
	})
}
