package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"sync"

	"github.com/rchampourlier/kaizenizer/metrics"
	"github.com/rchampourlier/kaizenizer/store"
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
	wgGenerators := sync.WaitGroup{}

	// TODO: initialize the generator with the events chan
	metricsGenerators := []metrics.Generator{
		metrics.LeadTime{},
		metrics.CycleTime{},
		metrics.Counters{},
	}
	wgGenerators.Add(3)

	eventsChans := make([](chan store.Event), len(metricsGenerators))
	metricsChans := make([](chan store.Metric), len(metricsGenerators))
	for i, gen := range metricsGenerators {
		eventsChans[i] = make(chan store.Event, 0)
		metricsChans[i] = make(chan store.Metric, 0)

		go func(ms chan store.Metric) {
			for m := range ms {
				s.WriteMetric(store.Metric(m))
			}
		}(metricsChans[i])

		go func(g metrics.Generator, j int) {
			g.Generate(eventsChans[j], segmentPrefix, s)
			wgGenerators.Done()
		}(gen, i)
	}
	for evt := range events {
		for _, eventsChan := range eventsChans {
			eventsChan <- store.Event(evt)
		}
	}
	for _, eventsChan := range eventsChans {
		close(eventsChan)
	}
	wgGenerators.Wait()
	s.DoneAndWait() // tell it's done and wait for everything to be written
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
		log.Fatalln(fmt.Errorf("[main] error in `openDB`: %s", err))
	}
	return db
}
