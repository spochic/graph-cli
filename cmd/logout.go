/*
Copyright © 2025 Sebastien Pochic <spochic@gmail.com>
*/
package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/spochic/graph-cli/internal/auth"
)

var logoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Log out of your Microsoft 365 account",
	Long:  `Remove the saved authentication session for your Microsoft 365 account.`,
	RunE:  runLogout,
}

func init() {
	rootCmd.AddCommand(logoutCmd)
}

func runLogout(cmd *cobra.Command, args []string) error {
	record, hasRecord, err := auth.LoadRecord()
	if err != nil {
		return err
	}
	if !hasRecord {
		fmt.Println("Not logged in.")
		return nil
	}

	if err := auth.DeleteRecord(); err != nil {
		return fmt.Errorf("failed to remove auth record: %w", err)
	}

	fmt.Println("Logged out", record.Username)
	return nil
}
