package mapping

import (
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
	}
	for _, c := range cases {
		got, ok := toFloat(c.in)
		assert.Equal(t, c.ok, ok)
		if c.ok {
			assert.Equal(t, c.want, got)
		}
	}
}
