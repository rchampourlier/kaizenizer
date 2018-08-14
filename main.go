package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"

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
// 1. Initializes the database (drops the necessary table `jira_metrics`
//    if it exists and creates it).
// 2. Processes Jira data (from `jira_issues_events`) and generate
//    metrics.
//
// ### cleanup
//
// Drops the `jira_metrics` table.
//
func main() {
	if len(os.Args) < 2 {
		usage()
	}

	db := openDB()
	defer db.Close()
	store := store.NewPGStore(db)

	switch os.Args[1] {

	case "reset":
		store.DropTables()
		store.CreateTables()
		generateMetrics(store.StreamEvents("issue_project", "JobTeaser"))

	case "cleanup":
		store.DropTables()

	default:
		usage()
	}
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

func generateMetrics(events chan store.Event) {
	for event := range events {
		log.Println(event)
	}
}
