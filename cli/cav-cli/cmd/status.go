package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/anthropic-cav/cav-cli/internal/client"
	"github.com/anthropic-cav/cav-cli/internal/identity"
	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show current identity and session status",
	RunE: func(cmd *cobra.Command, args []string) error {
		id, err := identity.Load()
		if err != nil {
			return fmt.Errorf("no identity: %w", err)
		}

		token, tokenErr := loadSessionToken()

		if jsonOutput {
			info := map[string]interface{}{
				"did":           id.DID,
				"gateway":       gatewayURL,
				"authenticated": tokenErr == nil,
			}
			if tokenErr == nil {
				c := client.New(gatewayURL, token)
				if status, err := c.GetStatus(); err == nil {
					info["citizen"] = status
				}
			}
			out, _ := json.MarshalIndent(info, "", "  ")
			fmt.Println(string(out))
		} else {
			fmt.Printf("  DID:     %s\n", id.DID)
			fmt.Printf("  Gateway: %s\n", gatewayURL)
			if tokenErr != nil {
				fmt.Println("  Session: not authenticated (run 'cav-cli auth')")
			} else {
				fmt.Println("  Session: active ✓")
				c := client.New(gatewayURL, token)
				if status, err := c.GetStatus(); err == nil {
					if level, ok := status["level"]; ok {
						fmt.Printf("  Level:   %v\n", level)
					}
				}
			}
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(statusCmd)
}
