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

package speedwire

import (
	"bytes"
	"log/slog"
	"testing"

	"github.com/chr-fritz/speedwire-exporter/pkg/logging"
	"github.com/stretchr/testify/assert"
	"gitlab.com/bboehmke/sunny"
)

func debugLogger() (*slog.Logger, *bytes.Buffer) {
	var buf bytes.Buffer
	return slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})), &buf
}

// These tests deliberately exercise the pieces of InstallSunnyLogger rather than calling it:
// assigning sunny.Log races with the discovery goroutines TestListenerDiscoveryDoesNotLeakGoroutines
// leaves draining (see the comment there). InstallSunnyLogger itself is then two assignments
// over behaviour that is covered here.

func TestSunnyLoggerForwardsFormattedMessages(t *testing.T) {
	log, buf := debugLogger()

	sunnyLogger{log: log}.Printf("found device %d at %s", 42, "10.0.0.1")

	assert.Contains(t, buf.String(), "found device 42 at 10.0.0.1")
}

// TestSunnyLoggerIsSilentAboveDebug keeps sunny's per-packet chatter out of normal operation:
// the connection logs one line per received datagram, which at info level would drown the
// exporter's own output.
func TestSunnyLoggerIsSilentAboveDebug(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))

	sunnyLogger{log: log}.Printf("recv 10.0.0.1: [proto.GroupPacketEntry]")

	assert.Empty(t, buf.String())
}

// TestSunnyLoggerSatisfiesSunnyLoggerInterface pins the adapter to the interface it is
// installed into; a signature drift in sunny would otherwise only show up at the call site.
func TestSunnyLoggerSatisfiesSunnyLoggerInterface(t *testing.T) {
	log, _ := debugLogger()

	var _ sunny.Logger = sunnyLogger{log: log}
}

// TestWantsPacketLoggingOnlyAtTraceLevel keeps sunny's dropped-packet diagnostics behind
// debug-4. They fire per dropped datagram on a busy segment, and they only matter when you
// are already asking why values arrive intermittently.
func TestWantsPacketLoggingOnlyAtTraceLevel(t *testing.T) {
	debug, _ := debugLogger()
	assert.False(t, wantsPacketLogging(debug), "per-packet logging is too noisy for plain debug")

	var buf bytes.Buffer
	trace := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: logging.LevelTrace}))
	assert.True(t, wantsPacketLogging(trace),
		"debug-4 is the level that asks for every received and dropped packet")
}
