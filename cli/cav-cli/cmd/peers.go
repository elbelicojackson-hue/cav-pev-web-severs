package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/anthropic-cav/cav-cli/internal/client"
	"github.com/spf13/cobra"
)

var peersCmd = &cobra.Command{
	Use:   "peers",
	Short: "List active citizen agents on the network",
	RunE: func(cmd *cobra.Command, args []string) error {
		c := client.New(gatewayURL, "")
		result, err := c.GetCitizens()
		if err != nil {
			return err
		}

		out, _ := json.MarshalIndent(result, "", "  ")
		fmt.Println(string(out))
		return nil
	},
}

func init() {
	rootCmd.AddCommand(peersCmd)
}
