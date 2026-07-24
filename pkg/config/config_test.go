package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLabelsForReturnsConfiguredLabels(t *testing.T) {
	c := Config{Devices: []DeviceConfig{
		{Serial: 1234567890, Labels: map[string]string{"meter": "grid"}},
	}}
	labels, ok := c.LabelsFor(1234567890)
	assert.True(t, ok)
	assert.Equal(t, "grid", labels["meter"])

	_, ok = c.LabelsFor(999)
	assert.False(t, ok)
}
