// Copyright © 2026 Christian Fritz <mail@chr-fritz.de>
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cmd

import (
	"bytes"
	"log/slog"
	"testing"

	"github.com/chr-fritz/speedwire-exporter/pkg/speedwire"
	"github.com/stretchr/testify/assert"
	"gitlab.com/bboehmke/sunny"
)

// TestInstallSunnyLoggerReplacesNopeLogger lives here rather than next to the code it tests:
// assigning sunny.Log races with the discovery goroutines that the speedwire package's leak
// test leaves draining, and this test binary runs no Listener at all.
//
// By default sunny.Log is a NopeLogger, so everything the Speedwire layer knows - invalid
// packets, discovery results, dropped packets - is silently discarded, which is what made a
// 12-hour outage produce no log output whatsoever.
func TestInstallSunnyLoggerReplacesNopeLogger(t *testing.T) {
	t.Cleanup(func() {
		sunny.Log = new(sunny.NopeLogger)
		sunny.EnableDetailedPacketLogging(false)
	})

	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	speedwire.InstallSunnyLogger(log)
	sunny.Log.Printf("new energy meter at %s - Serial=%d", "10.0.0.1", 42)

	assert.Contains(t, buf.String(), "new energy meter at 10.0.0.1 - Serial=42")
}
