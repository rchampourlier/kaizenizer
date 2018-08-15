package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/rchampourlier/kaizenizer-jira-metrics/store"
)

const poolSize = 10

// MaxOpenConns defines the maximum number of open connections
// to the DB.
const MaxOpenConns = 5 // for Heroku Postgres

// Main program
//
// ### reset
//
// 1. Initializes the database (drops the necessary table `metrics`
//    if it exists and creates it).
// 2. Processes Jira data (from `jira_issues_events`) and generate
//    metrics.
//
// ### cleanup
//
// Drops the `metrics` table.
//
func main() {
	if len(os.Args) < 2 {
		usage()
	}

	db := openDB()
	defer db.Close()
	s := store.NewPGStore(db)

	switch os.Args[1] {

	case "reset":
		s.DropTables()
		s.CreateTables()
		generateMetrics(
			s,
			s.StreamEvents("issue_project", "JobTeaser"),
			"jt",
		)

	case "cleanup":
		s.DropTables()

	default:
		usage()
	}
}

func generateMetrics(s *store.PGStore, events chan store.Event, segmentPrefix string) {
	ltEvents := make(chan store.Event, 0)
	ctEvents := make(chan store.Event, 0)
	go generateMetricLeadTime(s, ltEvents, segmentPrefix)
	go generateMetricCycleTime(s, ctEvents, segmentPrefix)
	for evt := range events {
		ltEvents <- evt
		ctEvents <- evt
	}
	close(ltEvents)
	close(ctEvents)
	s.FlushMetrics()
}

func generateMetricLeadTime(s *store.PGStore, events chan store.Event, segmentPrefix string) {
	// m is a map issueKey -> {start, end of lead time}
	type bounds struct {
		complete   bool
		start, end time.Time
	}
	m := make(map[string]bounds)

	var countIssues, countEvts, countMetrics int
	reopenedIssues := make(map[string]bool)
	for evt := range events {
		countEvts++
		if _, ok := m[evt.IssueKey]; !ok {
			// Issue has not been seen yet
			countIssues++
			m[evt.IssueKey] = bounds{complete: false, start: evt.Time}
		} else {
			if m[evt.IssueKey].complete {
				reopenedIssues[evt.IssueKey] = true
				//log.Printf("processing event for resolved issue: %s\n", evt)
			} else {
				if evt.ValueTo == "done" {
					b := bounds{
						complete: true,
						start:    m[evt.IssueKey].start,
						end:      evt.Time,
					}
					m[evt.IssueKey] = b
					segment := fmt.Sprintf("%s/%s", segmentPrefix, evt.IssueKey)
					leadTime := b.end.Sub(b.start) / (24 * time.Hour)
					t := evt.Time
					name := "lead_time"
					value := float64(leadTime)
					countMetrics++
					s.WriteMetric(store.Metric{
						Time:    t,
						Name:    name,
						Segment: segment,
						Value:   value,
					})
				}
			}
		}
	}
	log.Printf("pushed %d `%s` metrics for %d issues (%.1f%% issues with status changes after resolution)\n",
		countMetrics,
		"lead_time",
		countIssues,
		float32(len(reopenedIssues))/float32(countIssues)*100,
	)
}

func generateMetricCycleTime(s *store.PGStore, events chan store.Event, segmentPrefix string) {
	// m is a map issueKey -> {start, end of cycle time}
	type bounds struct {
		wipSeen    bool
		doneSeen   bool
		start, end time.Time
	}
	m := make(map[string]bounds)

	var countIssues, countEvts, countMetrics int
	for evt := range events {
		countEvts++
		if _, ok := m[evt.IssueKey]; !ok {
			// Issue has not been seen yet
			// Setting `start` in case the issue does not go through
			// a `wip` status
			countIssues++
			m[evt.IssueKey] = bounds{
				wipSeen:  false,
				doneSeen: false,
				start:    evt.Time,
			}
		} else if !m[evt.IssueKey].wipSeen && evt.ValueTo == "wip" {
			// Issue has been seen but `wip` not yet
			// (we don't want to override `start` if the issue
			// goes through `wip` several times)
			m[evt.IssueKey] = bounds{
				wipSeen:  true,
				doneSeen: false,
				start:    evt.Time,
			}
		} else if !m[evt.IssueKey].doneSeen && evt.ValueTo == "done" {
			b := bounds{
				wipSeen:  m[evt.IssueKey].wipSeen,
				doneSeen: true,
				start:    m[evt.IssueKey].start,
				end:      evt.Time,
			}
			m[evt.IssueKey] = b
			segment := fmt.Sprintf("%s/%s", segmentPrefix, evt.IssueKey)
			cycleTime := b.end.Sub(b.start) / (24 * time.Hour)
			t := evt.Time
			name := "cycle_time"
			value := float64(cycleTime)
			countMetrics++
			s.WriteMetric(store.Metric{
				Time:    t,
				Name:    name,
				Segment: segment,
				Value:   value,
			})
		}
	}
	log.Printf("pushed %d `%s` metrics for %d issues\n",
		countMetrics,
		"cycle_time",
		countIssues,
	)
}

func usage() {
	fmt.Printf(`Usage: go run main.go <action>

Available actions:
  - reset
  - cleanup
`)
	os.Exit(1)
}

func openDB() *sql.DB {
	connStr := os.Getenv("DB_URL")
	db, err := sql.Open("postgres", connStr)
	db.SetMaxOpenConns(MaxOpenConns)
	if err != nil {
		log.Fatalln(fmt.Errorf("error in `openDB`: %s", err))
	}
	return db
}
