package cmd

import (
	"github.com/spf13/cobra"
)

// validateCmd represents the validate command
var tokenValidatorCmd = &cobra.Command{
	Use:   "validate <token>",
	Short: "Check if an access token is valid.",
	Run:   cmdHandler.Warden.IsAuthorized,
}

func init() {
	tokenCmd.AddCommand(tokenValidatorCmd)
	tokenValidatorCmd.Flags().StringSlice("scopes", []string{"core"}, "Additionally check if scope was granted")
}
