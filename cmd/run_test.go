package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGoroutineThreshold(t *testing.T) {
	tests := []struct {
		numDevices int
		want       int
	}{
		{0, 100},
		{2, 140},
		{5, 200},
		{10, 300},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, goroutineThreshold(tt.numDevices),
			"goroutineThreshold(%d)", tt.numDevices)
	}
}
