/*
 * Copyright © 2023 Christian Fritz <mail@chr-fritz.de>
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package logging

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_loggerConfig_Initialize(t *testing.T) {
	tests := []struct {
		name              string
		level             string
		format            string
		expectedLevel     slog.Level
		expectedFormatter slog.Handler
	}{
		{
			"info text to stderr",
			"info",
			"text",
			slog.LevelInfo,
			&slog.TextHandler{},
		},
		{
			"info text as json",
			"info",
			"json",
			slog.LevelInfo,
			&slog.JSONHandler{},
		},
		{
			"unknown log formatter",
			"info",
			"unknown",
			slog.LevelInfo,
			&slog.TextHandler{},
		},
		{
			"invalid debug level",
			"not valid",
			"text",
			slog.LevelInfo,
			&slog.TextHandler{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			viper.Reset()
			t.Cleanup(viper.Reset)
			viper.Set("logging.level", tt.level)
			viper.Set("logging.format", tt.format)

			ctx := t.Context()
			lc := &loggerConfig{}
			lc.Initialize()
			logger := slog.With("dummy")
			assert.True(t, logger.Enabled(ctx, tt.expectedLevel))
			assert.False(t, logger.Enabled(ctx, tt.expectedLevel-1))
			handler := logger.Handler()
			assert.IsType(t, &tracingLogHandler{}, handler)
			logHandler := handler.(*tracingLogHandler)
			assert.IsType(t, tt.expectedFormatter, logHandler.parent)
		})
	}
}

// Test_loggerConfig_Initialize_ReadsFromViper is a focused regression test for
// the bug where Initialize() only ever looked at the flag-bound struct fields
// (which default to "info"/"text") and ignored whatever was configured via
// viper (e.g. read from a config file's logging.format/logging.level).
func Test_loggerConfig_Initialize_ReadsFromViper(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.Set("logging.format", "json")
	viper.Set("logging.level", "debug")

	lc := &loggerConfig{
		// Deliberately pre-populate with the flag defaults, to prove that
		// Initialize() overrides them from viper rather than using these.
		level:         "info",
		formatterName: "text",
	}
	lc.Initialize()

	logger := slog.Default()
	assert.True(t, logger.Enabled(t.Context(), slog.LevelDebug))
	handler := logger.Handler()
	require.IsType(t, &tracingLogHandler{}, handler)
	logHandler := handler.(*tracingLogHandler)
	assert.IsType(t, &slog.JSONHandler{}, logHandler.parent)
}

func Test_selectHandler(t *testing.T) {
	tests := []struct {
		name         string
		format       string
		expectedType slog.Handler
		expectJSON   bool
	}{
		{"json format", "json", &slog.JSONHandler{}, true},
		{"JSON case-insensitive", "JSON", &slog.JSONHandler{}, true},
		{"text format", "text", &slog.TextHandler{}, false},
		{"unknown format falls back to text", "unknown", &slog.TextHandler{}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			handler := selectHandler(tt.format, &buf, &slog.HandlerOptions{})
			assert.IsType(t, tt.expectedType, handler)

			logger := slog.New(handler)
			logger.Info("hello", "key", "value")

			if tt.expectJSON {
				var decoded map[string]any
				err := json.Unmarshal(buf.Bytes(), &decoded)
				require.NoError(t, err, "expected valid JSON output, got: %s", buf.String())
				assert.Equal(t, "hello", decoded["msg"])
				assert.Equal(t, "value", decoded["key"])
			} else {
				err := json.Unmarshal(buf.Bytes(), &map[string]any{})
				assert.Error(t, err, "expected non-JSON (text) output, got: %s", buf.String())
				assert.Contains(t, buf.String(), "hello")
			}
		})
	}
}
