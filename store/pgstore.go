package store

import (
	"database/sql"
	"fmt"
	"github.com/rchampourlier/golib/slices"
	"log"
	"regexp"
	"strings"
	"sync"
	"time"

	pq "github.com/lib/pq" // PG engine for database/sql
)

// BatchSize is the max size of slices sent to the database
// through bulk imports.
const BatchSize = 10000

// PGStore implements the application's `Store` with a
// Postgres DB backend.
type PGStore struct {
	*sql.DB
	*sync.WaitGroup // wait for all metrics received to be written
	metrics         chan Metric
}

// NewPGStore returns a `PGStore` storing the specified DB.
// The passed DB should already be open and ready to
// receive queries.
func NewPGStore(db *sql.DB) *PGStore {
	metrics := make(chan Metric, 0)
	s := PGStore{
		db,
		&sync.WaitGroup{},
		metrics,
	}
	go s.processMetricsFromChan(metrics)
	return &s
}

// Metric represents a metric to be stored to the DB.
type Metric struct {
	Time    time.Time
	Name    string
	Segment string
	Value   float64
	Comment string
}

// Event represents the Event loaded from the database,
// generated by [Jira Source]() (from the
// `jira_issues_events` table).
type Event struct {
	Time           time.Time
	Kind           string
	IssueKey       string
	IssueType      string
	Segment        string
	ValueFrom      string
	ValueTo        string
	IssueCreatedAt time.Time
}

func (e Event) String() string {
	return fmt.Sprintf("{EVENT:%s - %s - issue:%s - from:%s - to:%s}", e.Kind, e.Time.Format(time.RFC3339), e.IssueKey, e.ValueFrom, e.ValueTo)
}

// WriteMetric writes a metric record to the database.
func (s *PGStore) WriteMetric(metric Metric) {
	s.Add(1)
	s.metrics <- metric
}

// DoneAndWait should be called when all metrics have been sent for writing.
// It will close the `s.metrics` channel so the last batch can be written.
// Blocks until the last batch has been written to database.
func (s *PGStore) DoneAndWait() {
	close(s.metrics)
	s.Wait()
}

func (s *PGStore) processMetricsFromChan(metrics chan Metric) {
	i := 0
	metricsBatch := make([]Metric, BatchSize)
	for metric := range metrics {
		// Read in the channel to fill a batch
		metricsBatch[i] = metric
		i++
		if i == BatchSize {
			// The batch is filled
			s.writeMetricsBatch(metricsBatch)
			i = 0
		}
	}
	// The channel has been closed
	s.writeMetricsBatch(metricsBatch[:i])
}

func (s *PGStore) writeMetricsBatch(metricsBatch []Metric) {
	txn, err := s.Begin()
	if err != nil {
		log.Fatal(err)
	}

	stmt, err := txn.Prepare(pq.CopyIn("metrics", "time", "name", "segment", "value", "comment"))
	if err != nil {
		log.Fatal(err)
	}

	for _, metric := range metricsBatch {
		_, err = stmt.Exec(metric.Time, metric.Name, metric.Segment, metric.Value, metric.Comment)
		if err != nil {
			log.Fatal(err)
		}
	}

	_, err = stmt.Exec()
	if err != nil {
		log.Fatal(err)
	}

	err = stmt.Close()
	if err != nil {
		log.Fatal(err)
	}

	err = txn.Commit()
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("[store] %d metrics written\n", len(metricsBatch))
	s.Add(-len(metricsBatch)) // mark done for all written metrics
}

// StreamEvents will generate a stream of `Event` records from
// the database's `jira_issues_events` table, filtered with a `WHERE`
// clause on the `segmentColumn` and `segmentValue`.
//
// TODO: remove segment filtering, process all, add project to segment
func (s *PGStore) StreamEvents(segmentColumn, segmentValue string) chan Event {
	events := make(chan Event, 0)

	go func() {
		query := fmt.Sprintf(`
		SELECT 
			event_time,
			event_kind,
			issue_key, 
			issue_type,
			issue_tribe,
			status_change_from,
			status_change_to,
			assignee_change_from,
			assignee_change_to,
			issue_created_at
		FROM jira_issues_events
		WHERE %s = $1
		ORDER BY event_time ASC
		`, segmentColumn)
		rows, err := s.Query(query, segmentValue)
		if err != nil {
			log.Fatal(err)
		}
		defer rows.Close()
		for rows.Next() {
			var t, issueCreatedAt time.Time
			var kind, issueKey, issueType string
			var issueTribe, statusFrom, statusTo, assigneeFrom, assigneeTo *string
			err := rows.Scan(
				&t,
				&kind,
				&issueKey,
				&issueType,
				&issueTribe,
				&statusFrom,
				&statusTo,
				&assigneeFrom,
				&assigneeTo,
				&issueCreatedAt,
			)
			if err != nil {
				log.Fatal(err)
			}

			switch kind {
			case "status_changed":
				statusGroupFrom := statusGroup(statusFrom)
				statusGroupTo := statusGroup(statusTo)
				if statusGroupFrom != statusGroupTo {
					events <- Event{
						Time:           t,
						Kind:           kind,
						IssueKey:       issueKey,
						IssueType:      issueTypeGroup(issueType),
						Segment:        segment(issueTribe),
						ValueFrom:      statusGroupFrom,
						ValueTo:        statusGroupTo,
						IssueCreatedAt: issueCreatedAt,
					}
				}
			}
		}
		if err := rows.Err(); err != nil {
			log.Fatal(err)
		}
		close(events)
	}()

	return events
}

// CreateTables creates the `metrics` table.
func (s *PGStore) CreateTables() {
	queries := []string{
		`CREATE TABLE "metrics" (
			"id" SERIAL PRIMARY KEY NOT NULL,
			"inserted_at" TIMESTAMP(6) NOT NULL DEFAULT statement_timestamp(),
			"time" TIMESTAMP(6) NOT NULL,
			"name" TEXT,
			"segment" TEXT,
			"value" DOUBLE PRECISION,
			"comment" TEXT
		);`,
	}
	err := s.exec(queries)
	if err != nil {
		log.Fatalln(fmt.Errorf("error in `CreateTables`: %s", err))
	}
}

// DropTables drops the tables used by this source
// (`jira_issues_events` and `jira_issues_states`)
func (s *PGStore) DropTables() {
	queries := []string{
		`DROP TABLE IF EXISTS "metrics";`,
	}
	err := s.exec(queries)
	if err != nil {
		log.Fatalln(fmt.Errorf("error in `DropTables()`: %s", err))
	}
}

// exec executes the passed SQL commands on the DB using `Exec`.
func (s *PGStore) exec(cmds []string) (err error) {
	for _, c := range cmds {
		_, err = s.Exec(c)
		if err != nil {
			return
		}
	}
	return
}

/* ============================= *
 * MAPPING	                 *
 * ============================= */

// statusGroup maps the Jira status to a simpler status used for
// metrics.
//
// Currently, statuses used in metrics are:
// backlog, wip, done, released.
func statusGroup(status *string) string {
	if status == nil {
		return ""
	}
	groups := map[string][]string{
		"backlog": []string{
			"Wait/Watch",
			"Open",
			"Ready",
			"Selected for spec",
			"To Price",
			"To Do",
			"Reopened",
			"ToDo",
			"In Preparation",
			"Ready for development",
			"Ready for sprint",
			"Selected for Development",
			"Open / Ready for dev",
			"Backlog",
		},
		"wip": []string{
			"To be tested",
			"Developed",
			"Waiting for validation",
			"In Staging",
			"Ready for Testing",
			"In Spec Review",
			"Tech review",
			"In Spec",
			"Quality check",
			"Technical review",
			"Développement",
			"Release for Review",
			"Stand-by",
			"Ready for Review",
			"In Development",
			"In Progress",
			"Functional Review",
			"Pending",
			"Pemding",
			"In Testing",
			"Ready for Staging",
			"To validate",
			"In Review",
			"In Design",
			"In Dev",
			"In Functional Review",
		},
		"done": []string{
			"To announce",
			"Ready for deploy",
			"To be released",
			"Functional GO",
			"Ready for Release",
		},
		"resolved": []string{
			"Closed",
			"Canceled",
			"Terminé",
			"Done",
			"Released",
			"Resolved",
		},
	}
	for group, statuses := range groups {
		if slices.StringsContain(statuses, *status) {
			return group
		}
	}
	log.Fatalf("Status did not match any group: %s", *status)
	return ""
}

func issueTypeGroup(issueType string) string {
	usIssueType := toUnderscore(issueType)
	groups := map[string][]string{
		"product": []string{
			"epic",
			"spec",
			"improvement",
			"story",
			"new_feature",
		},
		"ops": []string{
			"sso_launch",
			"task",
		},
		"technical": []string{
			"technical_task",
			"sub_task",
		},
		"bug": []string{
			"bug",
		},
	}
	for group, types := range groups {
		if slices.StringsContain(types, usIssueType) {
			return group
		}
	}
	log.Fatalf("Status did not match any group: %s", issueType)
	return ""
}

func segment(issueTribe *string) string {
	tribe := "none"
	if issueTribe != nil {
		tribe = *issueTribe
	}
	usTribe := toUnderscore(tribe)

	return fmt.Sprintf("tribe_%s", usTribe)
}

func toUnderscore(str string) string {
	lower := strings.ToLower(str)
	return regexp.MustCompile("[\\s-/]").ReplaceAllString(lower, "_")
}
