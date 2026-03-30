/*
Copyright © 2025 Sebastien Pochic <spochic@gmail.com>
*/
package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	kiotaazure "github.com/microsoft/kiota-authentication-azure-go"
	msgraphsdk "github.com/microsoftgraph/msgraph-sdk-go"
	"github.com/microsoftgraph/msgraph-sdk-go/models"
	graphusers "github.com/microsoftgraph/msgraph-sdk-go/users"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/spochic/graph-cli/internal/auth"
)

var mailCmd = &cobra.Command{
	Use:   "mail",
	Short: "Search your mailbox",
	Long:  `Commands for searching your Microsoft 365 mailbox.`,
}

var mailSearchCmd = &cobra.Command{
	Use:   "search",
	Short: "Search mailbox messages",
	Long: `Search messages in your mailbox using various criteria.

Examples:
  graph-cli mail search --top 5
  graph-cli mail search --from alice@example.com --unread
  graph-cli mail search --folder Inbox --after 2026-01-01 --has-attachments
  graph-cli mail search --query "project kickoff" --output json`,
	RunE: runMailSearch,
}

var (
	mailSubject        string
	mailFrom           string
	mailTo             string
	mailFolder         string
	mailAfter          string
	mailBefore         string
	mailUnread         bool
	mailHasAttachments bool
	mailQuery          string
	mailTop            int
	mailOutput         string
)

func init() {
	rootCmd.AddCommand(mailCmd)
	mailCmd.AddCommand(mailSearchCmd)

	f := mailSearchCmd.Flags()
	f.StringVar(&mailSubject, "subject", "", "Subject contains text")
	f.StringVar(&mailFrom, "from", "", "Sender email address")
	f.StringVar(&mailTo, "to", "", "Recipient email address")
	f.StringVar(&mailFolder, "folder", "", "Folder name: Inbox, SentItems, Drafts, DeletedItems, Junk, Archive")
	f.StringVar(&mailAfter, "after", "", "Received after date (YYYY-MM-DD)")
	f.StringVar(&mailBefore, "before", "", "Received before date (YYYY-MM-DD)")
	f.BoolVar(&mailUnread, "unread", false, "Only unread messages")
	f.BoolVar(&mailHasAttachments, "has-attachments", false, "Only messages with attachments")
	f.StringVar(&mailQuery, "query", "", "Free-text search (mutually exclusive with all other filter flags)")
	f.IntVar(&mailTop, "top", 10, "Maximum number of results")
	f.StringVar(&mailOutput, "output", "table", "Output format: table or json")
}

func runMailSearch(cmd *cobra.Command, args []string) error {
	record, hasRecord, err := auth.LoadRecord()
	if err != nil {
		return err
	}
	if !hasRecord {
		return fmt.Errorf("not logged in — run 'graph-cli login' first")
	}

	tenantID := viper.GetString("tenant_id")
	clientID := viper.GetString("client_id")
	if tenantID == "" {
		return fmt.Errorf("tenant-id required in config file or via --tenant-id flag")
	}
	if clientID == "" {
		return fmt.Errorf("client-id required in config file or via --client-id flag")
	}

	if mailQuery != "" {
		if mailSubject != "" || mailFrom != "" || mailTo != "" ||
			mailAfter != "" || mailBefore != "" || mailUnread || mailHasAttachments {
			return fmt.Errorf("--query cannot be combined with other filter flags")
		}
	}

	tokenCache, err := auth.NewTokenCache()
	if err != nil {
		return err
	}

	cred, err := azidentity.NewInteractiveBrowserCredential(&azidentity.InteractiveBrowserCredentialOptions{
		TenantID:             tenantID,
		ClientID:             clientID,
		Cache:                tokenCache,
		AuthenticationRecord: record,
	})
	if err != nil {
		return fmt.Errorf("failed to initialize credential: %w", err)
	}

	authProvider, err := kiotaazure.NewAzureIdentityAuthenticationProviderWithScopes(cred, []string{"Mail.Read"})
	if err != nil {
		return fmt.Errorf("failed to create auth provider: %w", err)
	}

	adapter, err := msgraphsdk.NewGraphRequestAdapter(authProvider)
	if err != nil {
		return fmt.Errorf("failed to create graph adapter: %w", err)
	}

	client := msgraphsdk.NewGraphServiceClient(adapter)
	ctx := context.Background()
	top32 := int32(mailTop)
	selectFields := []string{"id", "subject", "from", "toRecipients", "receivedDateTime", "isRead", "hasAttachments", "bodyPreview"}

	var messages []models.Messageable

	if mailQuery != "" {
		search := mailQuery
		if mailFolder != "" {
			result, err := client.Me().MailFolders().ByMailFolderId(mailFolder).Messages().Get(ctx,
				&graphusers.ItemMailFoldersItemMessagesRequestBuilderGetRequestConfiguration{
					QueryParameters: &graphusers.ItemMailFoldersItemMessagesRequestBuilderGetQueryParameters{
						Search: &search,
						Select: selectFields,
						Top:    &top32,
					},
				})
			if err != nil {
				return fmt.Errorf("search failed: %w", err)
			}
			messages = result.GetValue()
		} else {
			result, err := client.Me().Messages().Get(ctx,
				&graphusers.ItemMessagesRequestBuilderGetRequestConfiguration{
					QueryParameters: &graphusers.ItemMessagesRequestBuilderGetQueryParameters{
						Search: &search,
						Select: selectFields,
						Top:    &top32,
					},
				})
			if err != nil {
				return fmt.Errorf("search failed: %w", err)
			}
			messages = result.GetValue()
		}
	} else {
		filter := buildMailFilter()
		var filterPtr *string
		if filter != "" {
			filterPtr = &filter
		}

		if mailFolder != "" {
			result, err := client.Me().MailFolders().ByMailFolderId(mailFolder).Messages().Get(ctx,
				&graphusers.ItemMailFoldersItemMessagesRequestBuilderGetRequestConfiguration{
					QueryParameters: &graphusers.ItemMailFoldersItemMessagesRequestBuilderGetQueryParameters{
						Filter: filterPtr,
						Select: selectFields,
						Top:    &top32,
					},
				})
			if err != nil {
				return fmt.Errorf("search failed: %w", err)
			}
			messages = result.GetValue()
		} else {
			result, err := client.Me().Messages().Get(ctx,
				&graphusers.ItemMessagesRequestBuilderGetRequestConfiguration{
					QueryParameters: &graphusers.ItemMessagesRequestBuilderGetQueryParameters{
						Filter: filterPtr,
						Select: selectFields,
						Top:    &top32,
					},
				})
			if err != nil {
				return fmt.Errorf("search failed: %w", err)
			}
			messages = result.GetValue()
		}
	}

	return printMailResults(messages, mailOutput)
}

func buildMailFilter() string {
	var clauses []string
	if mailSubject != "" {
		clauses = append(clauses, fmt.Sprintf("contains(subject,'%s')", mailSubject))
	}
	if mailFrom != "" {
		clauses = append(clauses, fmt.Sprintf("from/emailAddress/address eq '%s'", mailFrom))
	}
	if mailTo != "" {
		clauses = append(clauses, fmt.Sprintf("toRecipients/any(r:r/emailAddress/address eq '%s')", mailTo))
	}
	if mailAfter != "" {
		clauses = append(clauses, fmt.Sprintf("receivedDateTime ge %sT00:00:00Z", mailAfter))
	}
	if mailBefore != "" {
		clauses = append(clauses, fmt.Sprintf("receivedDateTime le %sT23:59:59Z", mailBefore))
	}
	if mailUnread {
		clauses = append(clauses, "isRead eq false")
	}
	if mailHasAttachments {
		clauses = append(clauses, "hasAttachments eq true")
	}
	return strings.Join(clauses, " and ")
}

type mailRecord struct {
	Subject        string   `json:"subject"`
	From           string   `json:"from"`
	To             []string `json:"to"`
	ReceivedAt     string   `json:"receivedDateTime"`
	IsRead         bool     `json:"isRead"`
	HasAttachments bool     `json:"hasAttachments"`
	BodyPreview    string   `json:"bodyPreview"`
}

func printMailResults(messages []models.Messageable, format string) error {
	records := make([]mailRecord, 0, len(messages))
	for _, m := range messages {
		rec := mailRecord{}
		if m.GetSubject() != nil {
			rec.Subject = *m.GetSubject()
		}
		if m.GetFrom() != nil && m.GetFrom().GetEmailAddress() != nil &&
			m.GetFrom().GetEmailAddress().GetAddress() != nil {
			rec.From = *m.GetFrom().GetEmailAddress().GetAddress()
		}
		for _, r := range m.GetToRecipients() {
			if r.GetEmailAddress() != nil && r.GetEmailAddress().GetAddress() != nil {
				rec.To = append(rec.To, *r.GetEmailAddress().GetAddress())
			}
		}
		if m.GetReceivedDateTime() != nil {
			rec.ReceivedAt = m.GetReceivedDateTime().Format(time.RFC3339)
		}
		if m.GetIsRead() != nil {
			rec.IsRead = *m.GetIsRead()
		}
		if m.GetHasAttachments() != nil {
			rec.HasAttachments = *m.GetHasAttachments()
		}
		if m.GetBodyPreview() != nil {
			rec.BodyPreview = *m.GetBodyPreview()
		}
		records = append(records, rec)
	}

	if format == "json" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(records)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "RECEIVED\tFROM\tSUBJECT\tREAD\tATTACH")
	for _, rec := range records {
		received := ""
		if rec.ReceivedAt != "" {
			if t, err := time.Parse(time.RFC3339, rec.ReceivedAt); err == nil {
				received = t.Format("2006-01-02 15:04")
			}
		}
		subject := rec.Subject
		if len(subject) > 50 {
			subject = subject[:47] + "..."
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%v\t%v\n", received, rec.From, subject, rec.IsRead, rec.HasAttachments)
	}
	return w.Flush()
}
