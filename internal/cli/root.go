package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "ais",
	Short: "API Integration Suite",
	Long: `AIS (API Integration Suite) is a programmable API gateway
for routing and integrating heterogeneous backend services.`,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
