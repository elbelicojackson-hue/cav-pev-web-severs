package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/anthropic-cav/cav-cli/internal/identity"
	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Generate a new Ed25519 citizen identity",
	Long:  `Creates a new Ed25519 key pair and stores it at ~/.cav/identity.json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if identity.Exists() {
			return fmt.Errorf("identity already exists at ~/.cav/identity.json (delete it first to regenerate)")
		}

		id, err := identity.Generate()
		if err != nil {
			return err
		}

		if jsonOutput {
			data, _ := json.MarshalIndent(map[string]string{
				"did":         id.DID,
				"fingerprint": id.Fingerprint,
				"public_key":  id.PublicKey,
				"stored_at":   "~/.cav/identity.json",
			}, "", "  ")
			fmt.Println(string(data))
		} else {
			fmt.Println("✓ Identity generated")
			fmt.Println()
			fmt.Printf("  Fingerprint: %s\n", id.Fingerprint)
			fmt.Printf("  DID:         %s\n", id.DID)
			fmt.Printf("  Key:         ~/.cav/identity.json\n")
			fmt.Println()
			fmt.Println("Your fingerprint is your identity on the CAV network.")
			fmt.Println("Share it freely — it's derived from your public key.")
			fmt.Println()
			fmt.Println("Next: run 'cav-cli auth' to authenticate with the gateway")
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(initCmd)
}
