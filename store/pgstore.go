package store

import (
	"database/sql"
	"fmt"
	"github.com/rchampourlier/golib/slices"
	"log"
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
	metrics chan Metric
	wg      *sync.WaitGroup
}

// NewPGStore returns a `PGStore` storing the specified DB.
// The passed DB should already be open and ready to
// receive queries.
func NewPGStore(db *sql.DB) *PGStore {
	metrics := make(chan Metric, 0)
	s := PGStore{
		db,
		metrics,
		&sync.WaitGroup{},
	}
	go s.processMetricsFromChan(metrics)
	return &s
}

// Event represents the event loaded from the database,
// generated by [Jira Source]() (from the
// `jira_issues_events` table).
type Event struct {
	Time      time.Time
	Kind      string
	IssueKey  string
	ValueFrom string
	ValueTo   string
}

func (e Event) String() string {
	return fmt.Sprintf("{EVENT:%s - %s - issue:%s - from:%s - to:%s}", e.Kind, e.Time.Format(time.RFC3339), e.IssueKey, e.ValueFrom, e.ValueTo)
}

// Metric represents a metric to be stored to the DB.
type Metric struct {
	Time    time.Time
	Name    string
	Segment string
	Value   float64
}

// WriteMetric writes a metric record to the database.
func (s *PGStore) WriteMetric(metric Metric) {
	s.wg.Add(1)
	s.metrics <- metric
}

// FlushMetrics ensures all metrics sent through `WriteMetric`
// have been processed.
func (s *PGStore) FlushMetrics() {
	close(s.metrics)
	s.wg.Wait()
}

func (s *PGStore) processMetricsFromChan(metrics chan Metric) {
	i := 0
	metricsBatch := make([]Metric, BatchSize)
	s.wg.Add(1) // for the last batch
	for metric := range metrics {
		// Read in the channel to fill a batch
		metricsBatch[i] = metric
		i++
		if i == BatchSize {
			// The batch is filled
			s.writeMetricsBatch(metricsBatch)
			i = 0
		}
		s.wg.Done()
	}
	// The channel has been closed
	s.writeMetricsBatch(metricsBatch[:i])
	s.wg.Done() // for the last batch
}

func (s *PGStore) writeMetricsBatch(metricsBatch []Metric) {
	log.Printf("writeMetricsBatch: %d items\n", len(metricsBatch))
	txn, err := s.Begin()
	if err != nil {
		log.Fatal(err)
	}

	stmt, err := txn.Prepare(pq.CopyIn("metrics", "time", "name", "segment", "value"))
	if err != nil {
		log.Fatal(err)
	}

	for _, metric := range metricsBatch {
		_, err = stmt.Exec(metric.Time, metric.Name, metric.Segment, metric.Value)
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
}

// StreamEvents will generate a stream of `Event` records from
// the database's `jira_issues_events` table, filtered with a `WHERE`
// clause on the `segmentColumn` and `segmentValue`.
func (s *PGStore) StreamEvents(segmentColumn, segmentValue string) chan Event {
	events := make(chan Event, 0)

	go func() {
		query := fmt.Sprintf(`
		SELECT 
			event_time,
			event_kind,
			issue_key, 
			status_change_from,
			status_change_to,
			assignee_change_from,
			assignee_change_to
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
			var t time.Time
			var kind, issueKey string
			var statusFrom, statusTo, assigneeFrom, assigneeTo *string
			err := rows.Scan(
				&t,
				&kind,
				&issueKey,
				&statusFrom,
				&statusTo,
				&assigneeFrom,
				&assigneeTo,
			)
			if err != nil {
				log.Fatal(err)
			}

			switch kind {
			case "status_changed":
				statusGroupFrom := statusGroup(statusFrom)
				statusGroupTo := statusGroup(statusTo)
				events <- Event{t, kind, issueKey, statusGroupFrom, statusGroupTo}
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
			"value" DOUBLE PRECISION
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

func statusGroup(status *string) string {
	if status == nil {
		return ""
	}
	groups := map[string][]string{
		"backlog": []string{
			"Wait/Watch",
			"Open",
			"Selected for spec",
			"To Price",
			"To Do",
			"Reopened",
			"ToDo",
			"In Preparation",
			"Ready for development",
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
			"Ready for sprint",
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
			"Ready",
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
