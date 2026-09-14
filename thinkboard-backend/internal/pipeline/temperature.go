package pipeline

import "errors"

// ErrTemperatureOutOfRange is returned by NewTemperature for a value outside [0,1].
var ErrTemperatureOutOfRange = errors.New("pipeline: temperature out of [0,1] range")

// Temperature is a per-conclusion confidence/risk score in [0,1] (thinkboard-schema-final.sql
// point_conclusions.temperature). The range is enforced here, not only by the DB CHECK.
type Temperature float64

// NewTemperature validates v is within [0,1] before returning a Temperature.
func NewTemperature(v float64) (Temperature, error) {
	if v < 0 || v > 1 {
		return 0, ErrTemperatureOutOfRange
	}
	return Temperature(v), nil
}
