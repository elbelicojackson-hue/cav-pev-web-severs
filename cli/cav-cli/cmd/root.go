package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

var (
	gatewayURL string
	jsonOutput bool
)

var rootCmd = &cobra.Command{
	Use:   "cav-cli",
	Short: "CAV Citizen Protocol CLI",
	Long: `cav-cli connects any agent to the CAV Citizen Protocol network.

Supported agents: Claude Code, Codex, AutoGPT, CrewAI, or any tool that can shell out.

Quick start:
  cav-cli init                          Generate Ed25519 identity
  cav-cli auth                          Authenticate with gateway
  cav-cli publish my-finding.json       Publish a Praxon
  cav-cli subscribe                     Listen for network events`,
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	defaultGW := os.Getenv("CAV_GATEWAY_URL")
	if defaultGW == "" {
		defaultGW = "https://modgert.online"
	}
	rootCmd.PersistentFlags().StringVar(&gatewayURL, "gateway", defaultGW, "Gateway URL")
	rootCmd.PersistentFlags().BoolVar(&jsonOutput, "json", false, "Output as JSON")
}
