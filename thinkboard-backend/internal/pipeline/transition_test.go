package pipeline

import "testing"

func TestAllowed(t *testing.T) {
	cases := []struct {
		name     string
		from, to Stage
		want     bool
	}{
		{"scope_anchor -> analytic", ScopeAnchor, Analytic, true},
		{"analytic -> research", Analytic, Research, true},
		{"research -> validation", Research, Validation, true},
		{"validation -> questioning", Validation, Questioning, true},
		{"questioning -> research_scoped", Questioning, ResearchScoped, true},
		{"research_scoped -> result", ResearchScoped, Result, true},

		{"backward", Research, Analytic, false},
		{"skip", ScopeAnchor, Research, false},
		{"self", Analytic, Analytic, false},
		{"past the end", Result, Stage("nonexistent"), false},
		{"unknown from", Stage("nonexistent"), Analytic, false},
		{"unknown to", ScopeAnchor, Stage("nonexistent"), false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Allowed(c.from, c.to); got != c.want {
				t.Errorf("Allowed(%q, %q) = %v, want %v", c.from, c.to, got, c.want)
			}
		})
	}
}

func TestOrderIsCanonicalSequence(t *testing.T) {
	want := []Stage{ScopeAnchor, Analytic, Research, Validation, Questioning, ResearchScoped, Result}
	if len(Order) != len(want) {
		t.Fatalf("Order has %d stages, want %d", len(Order), len(want))
	}
	for i, s := range want {
		if Order[i] != s {
			t.Errorf("Order[%d] = %q, want %q", i, Order[i], s)
		}
	}
}
