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
	"os"

	"github.com/spf13/cobra"
)

// NewCompletionCmd creates the `completion` command
func NewCompletionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "completion [bash|zsh|fish|powershell]",
		Short: "Generate completion script",
		Long: `To load completions:

Bash:

  $ source <(speedwire-exporter completion bash)

  # To load completions for each session, execute once:
  # Linux:
  $ speedwire-exporter completion bash > /etc/bash_completion.d/speedwire-exporter
  # macOS:
  $ speedwire-exporter completion bash > /usr/local/etc/bash_completion.d/speedwire-exporter

Zsh:

  # If shell completion is not already enabled in your environment,
  # you will need to enable it.  You can execute the following once:

  $ echo "autoload -U compinit; compinit" >> ~/.zshrc

  # To load completions for each session, execute once:
  $ speedwire-exporter completion zsh > "${fpath[1]}/speedwire-exporter"

  # You will need to start a new shell for this setup to take effect.

fish:

  $ speedwire-exporter completion fish | source

  # To load completions for each session, execute once:
  $ speedwire-exporter completion fish > ~/.config/fish/completions/speedwire-exporter.fish

PowerShell:

  PS> speedwire-exporter completion powershell | Out-String | Invoke-Expression

  # To load completions for every new session, run:
  PS> speedwire-exporter completion powershell > speedwire-exporter.ps1
  # and source this file from your PowerShell profile.
`,
		DisableFlagsInUseLine: true,
		ValidArgs:             []string{"bash", "zsh", "fish", "powershell", "ps1"},
		Args:                  cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
		Run: func(cmd *cobra.Command, args []string) {
			switch args[0] {
			case "bash":
				_ = cmd.Root().GenBashCompletion(os.Stdout)
			case "zsh":
				_ = cmd.Root().GenZshCompletion(os.Stdout)
			case "fish":
				_ = cmd.Root().GenFishCompletion(os.Stdout, true)
			case "powershell":
				fallthrough
			case "ps1":
				_ = cmd.Root().GenPowerShellCompletion(os.Stdout)
			}
		},
	}

	return cmd
}

func init() {
	rootCmd.AddCommand(NewCompletionCmd())
}
