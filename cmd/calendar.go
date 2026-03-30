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

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	abstractions "github.com/microsoft/kiota-abstractions-go"
	kiotaazure "github.com/microsoft/kiota-authentication-azure-go"
	msgraphsdk "github.com/microsoftgraph/msgraph-sdk-go"
	"github.com/microsoftgraph/msgraph-sdk-go/models"
	graphusers "github.com/microsoftgraph/msgraph-sdk-go/users"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/spochic/graph-cli/internal/auth"
)

var calendarCmd = &cobra.Command{
	Use:   "calendar",
	Short: "Search your calendar",
	Long:  `Commands for searching your Microsoft 365 calendar.`,
}

var calendarSearchCmd = &cobra.Command{
	Use:   "search",
	Short: "Search calendar events",
	Long: `Search events in your calendar using various criteria.

Examples:
  graph-cli calendar search --after 2026-03-30 --top 10
  graph-cli calendar search --subject "standup" --organizer alice@example.com
  graph-cli calendar search --attendee bob@example.com --output json
  graph-cli calendar search --after 2026-01-01 --before 2026-12-31 --location "Teams"`,
	RunE: runCalendarSearch,
}

var (
	calSubject   string
	calOrganizer string
	calAttendee  string
	calLocation  string
	calAfter     string
	calBefore    string
	calTop       int
	calOutput    string
)

func init() {
	rootCmd.AddCommand(calendarCmd)
	calendarCmd.AddCommand(calendarSearchCmd)

	f := calendarSearchCmd.Flags()
	f.StringVar(&calSubject, "subject", "", "Event subject contains text")
	f.StringVar(&calOrganizer, "organizer", "", "Organizer email address")
	f.StringVar(&calAttendee, "attendee", "", "Attendee email address")
	f.StringVar(&calLocation, "location", "", "Location contains text")
	f.StringVar(&calAfter, "after", "", "Events starting after date (YYYY-MM-DD)")
	f.StringVar(&calBefore, "before", "", "Events starting before date (YYYY-MM-DD)")
	f.IntVar(&calTop, "top", 10, "Maximum number of results")
	f.StringVar(&calOutput, "output", "table", "Output format: table or json")
}

func runCalendarSearch(cmd *cobra.Command, args []string) error {
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

	authProvider, err := kiotaazure.NewAzureIdentityAuthenticationProviderWithScopes(cred, []string{"Calendars.Read"})
	if err != nil {
		return fmt.Errorf("failed to create auth provider: %w", err)
	}

	adapter, err := msgraphsdk.NewGraphRequestAdapter(authProvider)
	if err != nil {
		return fmt.Errorf("failed to create graph adapter: %w", err)
	}

	client := msgraphsdk.NewGraphServiceClient(adapter)
	ctx := context.Background()
	top32 := int32(calTop)
	selectFields := []string{"id", "subject", "organizer", "attendees", "start", "end", "location", "bodyPreview", "isAllDay"}

	filter := buildCalendarFilter()
	var filterPtr *string
	if filter != "" {
		filterPtr = &filter
	}

	requestConfig := &graphusers.ItemEventsRequestBuilderGetRequestConfiguration{
		QueryParameters: &graphusers.ItemEventsRequestBuilderGetQueryParameters{
			Filter: filterPtr,
			Select: selectFields,
			Top:    &top32,
		},
	}

	// Filtering on attendees requires ConsistencyLevel: eventual and $count=true.
	if calAttendee != "" {
		countTrue := true
		requestConfig.QueryParameters.Count = &countTrue
		requestConfig.Headers = abstractions.NewRequestHeaders()
		requestConfig.Headers.Add("ConsistencyLevel", "eventual")
	}

	result, err := client.Me().Events().Get(ctx, requestConfig)
	if err != nil {
		return fmt.Errorf("search failed: %w", err)
	}

	return printCalendarResults(result.GetValue(), calOutput)
}

func buildCalendarFilter() string {
	var clauses []string
	if calSubject != "" {
		clauses = append(clauses, fmt.Sprintf("contains(subject,'%s')", calSubject))
	}
	if calOrganizer != "" {
		clauses = append(clauses, fmt.Sprintf("organizer/emailAddress/address eq '%s'", calOrganizer))
	}
	if calAttendee != "" {
		clauses = append(clauses, fmt.Sprintf("attendees/any(a:a/emailAddress/address eq '%s')", calAttendee))
	}
	if calLocation != "" {
		clauses = append(clauses, fmt.Sprintf("contains(location/displayName,'%s')", calLocation))
	}
	if calAfter != "" {
		clauses = append(clauses, fmt.Sprintf("start/dateTime ge '%sT00:00:00'", calAfter))
	}
	if calBefore != "" {
		clauses = append(clauses, fmt.Sprintf("start/dateTime le '%sT23:59:59'", calBefore))
	}
	return strings.Join(clauses, " and ")
}

type calRecord struct {
	Subject     string   `json:"subject"`
	Organizer   string   `json:"organizer"`
	Attendees   []string `json:"attendees"`
	Start       string   `json:"start"`
	End         string   `json:"end"`
	Location    string   `json:"location"`
	IsAllDay    bool     `json:"isAllDay"`
	BodyPreview string   `json:"bodyPreview"`
}

func printCalendarResults(events []models.Eventable, format string) error {
	records := make([]calRecord, 0, len(events))
	for _, e := range events {
		rec := calRecord{}
		if e.GetSubject() != nil {
			rec.Subject = *e.GetSubject()
		}
		if e.GetOrganizer() != nil && e.GetOrganizer().GetEmailAddress() != nil &&
			e.GetOrganizer().GetEmailAddress().GetAddress() != nil {
			rec.Organizer = *e.GetOrganizer().GetEmailAddress().GetAddress()
		}
		for _, a := range e.GetAttendees() {
			if a.GetEmailAddress() != nil && a.GetEmailAddress().GetAddress() != nil {
				rec.Attendees = append(rec.Attendees, *a.GetEmailAddress().GetAddress())
			}
		}
		if e.GetStart() != nil && e.GetStart().GetDateTime() != nil {
			rec.Start = *e.GetStart().GetDateTime()
		}
		if e.GetEnd() != nil && e.GetEnd().GetDateTime() != nil {
			rec.End = *e.GetEnd().GetDateTime()
		}
		if e.GetLocation() != nil && e.GetLocation().GetDisplayName() != nil {
			rec.Location = *e.GetLocation().GetDisplayName()
		}
		if e.GetIsAllDay() != nil {
			rec.IsAllDay = *e.GetIsAllDay()
		}
		if e.GetBodyPreview() != nil {
			rec.BodyPreview = *e.GetBodyPreview()
		}
		records = append(records, rec)
	}

	if format == "json" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(records)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "START\tEND\tSUBJECT\tORGANIZER\tLOCATION")
	for _, rec := range records {
		start := formatCalDateTime(rec.Start)
		end := formatCalDateTime(rec.End)
		subject := rec.Subject
		if len(subject) > 40 {
			subject = subject[:37] + "..."
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", start, end, subject, rec.Organizer, rec.Location)
	}
	return w.Flush()
}

// formatCalDateTime trims the fractional seconds from a Graph event dateTime string
// and returns just "YYYY-MM-DD HH:MM" for table display.
func formatCalDateTime(dt string) string {
	if len(dt) < 16 {
		return dt
	}
	// dt is like "2026-03-30T09:00:00.0000000" — replace T with space, take first 16 chars.
	s := strings.Replace(dt, "T", " ", 1)
	return s[:16]
}
