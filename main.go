package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"
	_ "time/tzdata" // embed timezone information in the program

	"github.com/google/go-github/v67/github"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"
)

func main() {
	ctx := context.Background()
	allIssues, err := getGitHubIssues(ctx)
	if err != nil {
		log.Fatalf("Unable to get GitHub issues: %v", err)
	}
	fmt.Printf("Total issues: %d\n", len(allIssues))

	err = updateSpreadsheet(len(allIssues))
	if err != nil {
		log.Fatalf("Unable to update spreadsheet: %v", err)
	}
}

func getGitHubIssues(ctx context.Context) ([]*github.Issue, error) {
	githubToken := os.Getenv("GITHUB_TOKEN")
	client := github.NewClient(nil).WithAuthToken(githubToken)

	// Get issues.
	var allIssues []*github.Issue
	opts := &github.IssueListByRepoOptions{
		State:  "open",
		Labels: []string{"#g-mdm", ":release", "bug"},
	}
	for {
		issues, resp, err := client.Issues.ListByRepo(ctx, "fleetdm", "fleet", opts)
		if err != nil {
			return nil, err
		}
		allIssues = append(allIssues, issues...)
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	return allIssues, nil
}

func updateSpreadsheet(value int) error {
	ctx := context.Background()

	// Get the spreadsheet ID from the environment variable
	spreadsheetId := os.Getenv("SPREADSHEET_ID")
	if spreadsheetId == "" {
		return fmt.Errorf("spreadsheet ID not found")
	}

	serviceAccountKey, err := os.ReadFile("credentials.json")
	switch {
	case os.IsNotExist(err):
		// credentials.json file not found, try to read from environment variable
		serviceAccountKey = []byte(os.Getenv("GOOGLE_SERVICE_ACCOUNT_KEY"))
	case err != nil:
		return fmt.Errorf("unable to read client secret file: %w", err)
	}
	if len(serviceAccountKey) == 0 {
		return fmt.Errorf("service account key not found")
	}

	cfg, err := google.JWTConfigFromJSON(serviceAccountKey, sheets.SpreadsheetsScope)
	if err != nil {
		return fmt.Errorf("unable to parse client secret file to config: %w", err)
	}
	client := cfg.Client(ctx)

	srv, err := sheets.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return fmt.Errorf("unable to retrieve Sheets client: %w", err)
	}

	readRange := "Sheet1!A2:B2"
	resp, err := srv.Spreadsheets.Values.Get(spreadsheetId, readRange).Do()
	if err != nil {
		return fmt.Errorf("unable to retrieve data from sheet: %v", err)
	}

	if len(resp.Values) == 0 {
		fmt.Println("No data found.")
	} else {
		fmt.Println("Previous data found.")
		fmt.Println("Date, Value:")
		for _, row := range resp.Values {
			fmt.Printf("%s, %s\n", row[0], row[1])
		}
	}
	loc, _ := time.LoadLocation("America/Chicago")
	valuesToWrite := [][]interface{}{{time.Now().In(loc).Format("2006-01-02 15:04:05 MST"), value}}

	// Insert new row
	_, err = srv.Spreadsheets.BatchUpdate(spreadsheetId, &sheets.BatchUpdateSpreadsheetRequest{
		Requests: []*sheets.Request{
			{
				InsertDimension: &sheets.InsertDimensionRequest{
					Range: &sheets.DimensionRange{
						SheetId:    0, // default Sheet1
						Dimension:  "ROWS",
						StartIndex: 1,
						EndIndex:   2,
					},
					InheritFromBefore: false,
				},
			},
		},
	}).Do()
	if err != nil {
		return fmt.Errorf("unable to insert row: %w", err)
	}

	_, err = srv.Spreadsheets.Values.Update(spreadsheetId, readRange, &sheets.ValueRange{
		Values: valuesToWrite,
	}).ValueInputOption("USER_ENTERED").Do()
	if err != nil {
		return fmt.Errorf("unable to write data to sheet: %w", err)
	}
	return nil

}
