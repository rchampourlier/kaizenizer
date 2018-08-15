package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"

	"github.com/rchampourlier/kaizenizer-jira-metrics/metrics"
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
	metricsGenerators := []func(*store.PGStore, chan store.Event, string){
		metrics.LeadTime,
		metrics.CycleTime,
	}
	eventsChans := make([](chan store.Event), 0)
	for _, mg := range metricsGenerators {
		eventsChan := make(chan store.Event, 0)
		eventsChans = append(eventsChans, eventsChan)
		go mg(s, eventsChan, segmentPrefix)
	}
	for evt := range events {
		for _, eventsChan := range eventsChans {
			eventsChan <- evt
		}
	}
	for _, eventsChan := range eventsChans {
		close(eventsChan)
	}
	s.FlushMetrics()
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
