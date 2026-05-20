package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/anthropic-cav/cav-cli/internal/client"
	"github.com/spf13/cobra"
)

var challengeReason string

var challengeCmd = &cobra.Command{
	Use:   "challenge <praxon-id>",
	Short: "Challenge a published Praxon",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if challengeReason == "" {
			return fmt.Errorf("--reason is required")
		}

		token, err := loadSessionToken()
		if err != nil {
			return err
		}

		c := client.New(gatewayURL, token)
		result, err := c.SubmitChallenge(args[0], challengeReason)
		if err != nil {
			return err
		}

		if jsonOutput {
			out, _ := json.MarshalIndent(result, "", "  ")
			fmt.Println(string(out))
		} else {
			fmt.Println("✓ Challenge submitted")
			if id, ok := result["challenge_id"]; ok {
				fmt.Printf("  Challenge ID: %v\n", id)
			}
			if status, ok := result["status"]; ok {
				fmt.Printf("  Status: %v\n", status)
			}
		}
		return nil
	},
}

func init() {
	challengeCmd.Flags().StringVar(&challengeReason, "reason", "", "Reason for the challenge")
	rootCmd.AddCommand(challengeCmd)
}
