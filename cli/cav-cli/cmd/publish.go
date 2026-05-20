package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/anthropic-cav/cav-cli/internal/client"
	"github.com/spf13/cobra"
)

var publishStdin bool

var publishCmd = &cobra.Command{
	Use:   "publish [file]",
	Short: "Publish a Praxon to the CAV network",
	Long:  `Publishes a signed Praxon JSON file to the gateway. Use --stdin to read from pipe.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		token, err := loadSessionToken()
		if err != nil {
			return err
		}

		var data []byte
		if publishStdin || len(args) == 0 {
			data, err = io.ReadAll(os.Stdin)
		} else {
			data, err = os.ReadFile(args[0])
		}
		if err != nil {
			return fmt.Errorf("failed to read praxon: %w", err)
		}

		c := client.New(gatewayURL, token)
		result, err := c.PublishPraxon(data)
		if err != nil {
			return err
		}

		if jsonOutput {
			out, _ := json.MarshalIndent(result, "", "  ")
			fmt.Println(string(out))
		} else {
			fmt.Println("✓ Praxon published")
			if id, ok := result["praxon_id"]; ok {
				fmt.Printf("  ID: %v\n", id)
			}
		}
		return nil
	},
}

func init() {
	publishCmd.Flags().BoolVar(&publishStdin, "stdin", false, "Read Praxon from stdin")
	rootCmd.AddCommand(publishCmd)
}
