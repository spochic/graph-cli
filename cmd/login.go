/*
Copyright © 2025 Sebastien Pochic <spochic@gmail.com>
*/
package cmd

import (
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/spochic/graph-cli/internal/auth"
)

const graphScope = "https://graph.microsoft.com/.default"

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "Log in to your Microsoft 365 account",
	Long:  `Authenticate with your Microsoft 365 account using the device code flow.`,
	RunE:  runLogin,
}

func init() {
	rootCmd.AddCommand(loginCmd)

	loginCmd.Flags().String("tenant-id", "", "Azure tenant ID")
	loginCmd.Flags().String("client-id", "", "Azure application (client) ID")
	_ = viper.BindPFlag("tenant_id", loginCmd.Flags().Lookup("tenant-id"))
	_ = viper.BindPFlag("client_id", loginCmd.Flags().Lookup("client-id"))
}

func runLogin(cmd *cobra.Command, args []string) error {
	tenantID := viper.GetString("tenant_id")
	clientID := viper.GetString("client_id")

	if tenantID == "" {
		return fmt.Errorf("tenant-id is required (set via --tenant-id flag or 'tenant_id' in config file)")
	}
	if clientID == "" {
		return fmt.Errorf("client-id is required (set via --client-id flag or 'client_id' in config file)")
	}

	tokenCache, err := auth.NewTokenCache()
	if err != nil {
		return err
	}

	record, hasRecord, err := auth.LoadRecord()
	if err != nil {
		return err
	}

	cred, err := azidentity.NewDeviceCodeCredential(&azidentity.DeviceCodeCredentialOptions{
		TenantID:             tenantID,
		ClientID:             clientID,
		Cache:                tokenCache,
		AuthenticationRecord: record,
		UserPrompt: func(ctx context.Context, message azidentity.DeviceCodeMessage) error {
			fmt.Println(message.Message)
			return nil
		},
	})
	if err != nil {
		return fmt.Errorf("failed to initialize credential: %w", err)
	}

	ctx := context.Background()

	// If a record exists, try to use the cached token silently first.
	if hasRecord {
		_, err = cred.GetToken(ctx, policy.TokenRequestOptions{Scopes: []string{graphScope}})
		if err == nil {
			fmt.Println("Already logged in as", record.Username)
			return nil
		}
	}

	// No record or silent refresh failed — trigger the device code flow.
	newRecord, err := cred.Authenticate(ctx, &policy.TokenRequestOptions{
		Scopes: []string{graphScope},
	})
	if err != nil {
		return fmt.Errorf("authentication failed: %w", err)
	}

	if err := auth.SaveRecord(newRecord); err != nil {
		return fmt.Errorf("login succeeded but failed to save auth record: %w", err)
	}

	fmt.Println("Successfully logged in as", newRecord.Username)
	return nil
}
