package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/anthropic-cav/cav-cli/internal/client"
	"github.com/spf13/cobra"
)

var fetchCmd = &cobra.Command{
	Use:   "fetch <praxon-id>",
	Short: "Fetch a Praxon by ID",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		c := client.New(gatewayURL, "")
		result, err := c.FetchPraxon(args[0])
		if err != nil {
			return err
		}

		out, _ := json.MarshalIndent(result, "", "  ")
		fmt.Println(string(out))
		return nil
	},
}

func init() {
	rootCmd.AddCommand(fetchCmd)
}
