package pipeline

// Stage mirrors the Postgres pipeline_stage enum (thinkboard-schema-final.sql) 1:1.
type Stage string

const (
	ScopeAnchor    Stage = "scope_anchor"
	Analytic       Stage = "analytic"
	Research       Stage = "research"
	Validation     Stage = "validation"
	Questioning    Stage = "questioning"
	ResearchScoped Stage = "research_scoped"
	Result         Stage = "result"
)

// Order is the canonical stage sequence — the only declaration of stage order
// in this package. Nothing else may hardcode or repeat this list.
var Order = []Stage{
	ScopeAnchor,
	Analytic,
	Research,
	Validation,
	Questioning,
	ResearchScoped,
	Result,
}
