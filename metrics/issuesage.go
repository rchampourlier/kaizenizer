package metrics

import (
	"fmt"
	"log"
	"time"

	"github.com/rchampourlier/kaizenizer/store"
)

type ageBucket struct {
	name   string
	maxAge time.Duration
}

const maxInt = int64(^uint64(0) >> 1)

// NB: the ageBuckets slice must be kept ordered by max age ascending
var ageBuckets = []ageBucket{
	ageBucket{"1d", 24 * time.Hour},
	ageBucket{"1w", 7 * 24 * time.Hour},
	ageBucket{"1m", 730 * time.Hour}, // 365,25d / 12m --> 30,4d * 24h
	ageBucket{"more", time.Duration(maxInt)},
}

// IssuesAge implements `Generator` for the _IssuesAge_ metric.
type IssuesAge struct {
	currentDay    time.Time           // the day of the last processed event
	backlogIssues map[string]issueAge // issue key -> issue struct
	wipIssues     map[string]issueAge // issue key -> issue struct
}

type issueAge struct {
	start     time.Time
	issueType string
	segment   string
}

// NewIssuesAge returns a `IssuesAge` struct initialized with internal
// data.
func NewIssuesAge() *IssuesAge {
	return &IssuesAge{
		backlogIssues: make(map[string]issueAge),
		wipIssues:     make(map[string]issueAge),
	}
}

// Generate generates metrics on issues age.
//
// NB: `events` must be sent in *ascending order on time*.
func (g *IssuesAge) Generate(events chan store.Event, segmentPrefix string, s *store.PGStore) {
	countMetrics := 0

	for evt := range events {
		//log.Printf("processing %s\n", evt)
		var emptyTime time.Time
		evtDay := time.Date(evt.Time.Year(), evt.Time.Month(), evt.Time.Day(), 0, 0, 0, 0, time.UTC)

		if emptyTime.Equal(g.currentDay) {
			// currentDay not set
			g.currentDay = evtDay
			//log.Printf("setting current day to %s\n", evtDay)
			g.updateIssuesLists(evt)

		} else if evtDay.Equal(g.currentDay) {
			// event still in current day
			g.updateIssuesLists(evt)

		} else if evtDay.After(g.currentDay) {
			// event in a day after current day
			for d := g.currentDay; d.Before(evtDay); d = d.Add(24 * time.Hour) {
				countMetrics += g.calculateAndPushMetricsForDay(d, s, segmentPrefix)
			}
			g.currentDay = evtDay
			g.updateIssuesLists(evt)

		} else {
			// event before current day --> ERROR
			log.Fatalf("received an event that happened before the day being processed: events should be ordered by time ascending!")
		}
	}
	// After the last event, calculate and push counters for the current day
	countMetrics += g.calculateAndPushMetricsForDay(g.currentDay, s, segmentPrefix)

	log.Printf("[metrics/issues_age] pushed %d metrics\n",
		countMetrics,
	)
}

func (g *IssuesAge) updateIssuesLists(evt store.Event) {
	switch evt.ValueFrom {
	case "backlog":
		delete(g.backlogIssues, evt.IssueKey)
	case "wip":
		delete(g.wipIssues, evt.IssueKey)
	}
	switch evt.ValueTo {
	case "backlog":
		g.backlogIssues[evt.IssueKey] = issueAge{
			start:     evt.IssueCreatedAt,
			issueType: evt.IssueType,
			segment:   evt.Segment,
		}
	case "wip":
		g.wipIssues[evt.IssueKey] = issueAge{
			start:     evt.Time, // here we use evt.Time to count from the moment the issue enters WIP
			issueType: evt.IssueType,
			segment:   evt.Segment,
		}
	}
}

// calculateAndPushMetricsForDay returns the number of metrics pushed.
func (g *IssuesAge) calculateAndPushMetricsForDay(day time.Time, s *store.PGStore, segmentPrefix string) int {
	countMetrics := 0
	counters := make(map[string]map[string]int)
	for _, ageBucket := range ageBuckets {
		counters[fmt.Sprintf("backlog_%s", ageBucket.name)] = make(map[string]int)
		counters[fmt.Sprintf("wip_%s", ageBucket.name)] = make(map[string]int)
	}

	for _, issue := range g.backlogIssues {
		issueAge := day.Sub(issue.start)
		for _, ageBucket := range ageBuckets {
			if issueAge < ageBucket.maxAge {
				counters[fmt.Sprintf("backlog_%s", ageBucket.name)][issue.segment]++
				break
				// Out of the ageBuckets loop.
				// It's ok because we iterate over ageBuckets by ascending max age
			}
		}
	}
	for _, issue := range g.wipIssues {
		issueAge := day.Sub(issue.start)
		for _, ageBucket := range ageBuckets {
			if issueAge < ageBucket.maxAge {
				counters[fmt.Sprintf("wip_%s", ageBucket.name)][issue.segment]++
				break
				// Out of the ageBuckets loop.
				// It's ok because we iterate over ageBuckets by ascending max age
			}
		}
	}
	for ageBucket, counters := range counters {
		for segment, value := range counters {
			countMetrics++
			metric := store.Metric{
				Time:    day,
				Name:    fmt.Sprintf("issuesAge/%s", ageBucket),
				Segment: fmt.Sprintf("%s/%s", segmentPrefix, segment),
				Value:   float64(value),
			}
			s.WriteMetric(metric)
		}
	}
	return countMetrics
}
