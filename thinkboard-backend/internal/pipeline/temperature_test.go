package pipeline

import "testing"

func TestNewTemperature(t *testing.T) {
	cases := []struct {
		name    string
		v       float64
		want    Temperature
		wantErr error
	}{
		{"lower bound", 0, 0, nil},
		{"upper bound", 1, 1, nil},
		{"mid range", 0.42, 0.42, nil},
		{"below range", -0.01, 0, ErrTemperatureOutOfRange},
		{"above range", 1.01, 0, ErrTemperatureOutOfRange},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := NewTemperature(c.v)
			if err != c.wantErr {
				t.Fatalf("NewTemperature(%v) error = %v, want %v", c.v, err, c.wantErr)
			}
			if err == nil && got != c.want {
				t.Errorf("NewTemperature(%v) = %v, want %v", c.v, got, c.want)
			}
		})
	}
}
