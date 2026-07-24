// Copyright © 2020-2022 Christian Fritz <mail@chr-fritz.de>
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
	"github.com/chr-fritz/speedwire-exporter/pkg/logging"
)

// loggerConfiguration is initialized from the persistent flags in init() below
// and later invoked explicitly from initConfig() (cmd/root.go) once viper has
// read the config file, so config values are honored (see cmd/root.go).
var loggerConfiguration logging.LoggerConfiguration

func init() {
	loggerConfiguration = logging.InitFlags(rootCmd.PersistentFlags(), rootCmd)
}
