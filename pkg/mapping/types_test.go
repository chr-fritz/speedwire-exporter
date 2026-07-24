package mapping

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestToFloatHandlesSunnyTypes(t *testing.T) {
	cases := []struct {
		in   interface{}
		want float64
		ok   bool
	}{
		{float64(1.5), 1.5, true},
		{uint64(42), 42, true},
		{uint32(7), 7, true},
		{int64(-3), -3, true},
		{nil, 0, false},
		{"nope", 0, false},
		// ordinary values must keep working, including zero
		{0, 0, true},
		{float64(0), 0, true},
		{50.1, 50.1, true},
		// SMA "not available" sentinels must be filtered out
		{uint64(math.MaxUint64), 0, false},
		{uint32(math.MaxUint32), 0, false},
		{int64(math.MinInt64), 0, false},
		{int32(math.MinInt32), 0, false},
		{math.NaN(), 0, false},
		{math.Inf(1), 0, false},
		{math.Inf(-1), 0, false},
		// observed pv_energy sentinel: float64(math.MaxUint64)/1000 ≈ 1.8446744e16
		{float64(uint64(math.MaxUint64)) / 1000, 0, false},
	}
	for _, c := range cases {
		got, ok := toFloat(c.in)
		assert.Equal(t, c.ok, ok, "input %#v", c.in)
		if c.ok {
			assert.Equal(t, c.want, got)
		}
	}
}
