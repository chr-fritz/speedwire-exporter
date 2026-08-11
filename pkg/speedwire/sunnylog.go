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
	"context"
	"fmt"
	"log/slog"

	"github.com/chr-fritz/speedwire-exporter/pkg/logging"
	"gitlab.com/bboehmke/sunny"
)

// sunnyLogger adapts sunny's Logger interface to slog. Everything sunny emits is internal
// trace output, so it all lands at debug level.
type sunnyLogger struct{ log *slog.Logger }

func (l sunnyLogger) Printf(format string, v ...interface{}) {
	// Guard the Sprintf: sunny logs one line per received datagram, and formatting them all
	// only to have slog drop them would be pure waste in the common (info-level) case.
	if !l.log.Enabled(context.Background(), slog.LevelDebug) {
		return
	}
	l.log.Debug(fmt.Sprintf(format, v...))
}

// InstallSunnyLogger routes sunny's internal trace output into log.
//
// sunny.Log defaults to a NopeLogger, which silently discards everything the Speedwire layer
// knows: malformed packets on the wire, which devices discovery found or skipped, and dropped
// packets. Without this, an exporter that receives nothing at all is indistinguishable from
// one that is working - the container simply logs nothing.
//
// Be aware of the volume: sunny logs one line per datagram it sends or receives
// unconditionally, so debug is already noisy on a live Speedwire segment, and the write
// happens synchronously on sunny's single receive goroutine. logging.LevelTrace (debug-4)
// adds the diagnostics for packets that were *dropped* - failed UDP reads and receivers whose
// channel was full - which is what you want when values arrive only intermittently.
//
// Call this once at startup, before any Listener runs: sunny.Log is a plain package global
// that sunny reads on every packet, without synchronisation.
func InstallSunnyLogger(log *slog.Logger) {
	sunny.Log = sunnyLogger{log: log}
	sunny.EnableDetailedPacketLogging(wantsPacketLogging(log))
}

// wantsPacketLogging reports whether log asks for sunny's dropped-packet diagnostics, which
// is the one part of InstallSunnyLogger worth deciding on its own.
func wantsPacketLogging(log *slog.Logger) bool {
	return log.Enabled(context.Background(), logging.LevelTrace)
}
