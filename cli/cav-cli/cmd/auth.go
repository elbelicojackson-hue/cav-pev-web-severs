package cmd

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"

	"github.com/anthropic-cav/cav-cli/internal/client"
	"github.com/anthropic-cav/cav-cli/internal/identity"
	"github.com/spf13/cobra"
)

var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "Authenticate with the CAV gateway",
	Long:  `Performs Ed25519 signature challenge to obtain a JWT session token.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		id, err := identity.Load()
		if err != nil {
			return err
		}

		c := client.New(gatewayURL, "")

		// Step 1: Get challenge nonce
		challenge, err := c.GetChallenge(id.DID)
		if err != nil {
			return fmt.Errorf("challenge request failed: %w", err)
		}

		// Step 2: Sign the nonce
		nonceBytes, err := hex.DecodeString(challenge.Nonce)
		if err != nil {
			return fmt.Errorf("invalid nonce format: %w", err)
		}

		signature, err := id.Sign(nonceBytes)
		if err != nil {
			return fmt.Errorf("signing failed: %w", err)
		}

		// Step 3: Submit signature
		result, err := c.SubmitVerify(id.DID, challenge.Nonce, signature)
		if err != nil {
			return fmt.Errorf("verification failed: %w", err)
		}

		// Step 4: Save session
		session := map[string]interface{}{
			"token":      result.Token,
			"expires_at": result.ExpiresAt,
			"did":        result.Citizen.DID,
			"level":      result.Citizen.Level,
			"gateway":    gatewayURL,
		}
		data, _ := json.MarshalIndent(session, "", "  ")
		if err := os.WriteFile(identity.SessionPath(), data, 0600); err != nil {
			return fmt.Errorf("failed to save session: %w", err)
		}

		if jsonOutput {
			fmt.Println(string(data))
		} else {
			fmt.Println("✓ Authenticated")
			fmt.Println()
			fmt.Printf("  DID:     %s\n", result.Citizen.DID)
			fmt.Printf("  Level:   %d\n", result.Citizen.Level)
			fmt.Printf("  Expires: %s\n", result.ExpiresAt)
			fmt.Printf("  Gateway: %s\n", gatewayURL)
			fmt.Println()
			fmt.Println("Session saved to ~/.cav/session.json")
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(authCmd)
}
