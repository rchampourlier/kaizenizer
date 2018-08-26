package metrics

import (
	"fmt"
	"log"
	"time"

	"github.com/rchampourlier/kaizenizer/store"
)

// Counters implements `Generator` for the _Counters_
// metric.
type Counters struct {
	counters map[string]int    // name -> count
	statuses map[string]string // issue key -> previous status
}

// NewCounters returns a `Counters` struct initialized with internal
// data.
func NewCounters() *Counters {
	return &Counters{
		make(map[string]int),
		make(map[string]string),
	}
}

// Generate generates metrics for counts of issues based on their status.
//
// Generated metrics:
//   - Cumulative Flow Diagram: unresolved issues, split between backlog and WIP --> name=counter/cfd_(wip|backlog)
//   - WIP composition: WIP issues, split between product, bug, technical, ops --> name=counter/wip_(product|bug|technical|ops)
//   - Backlog composition: same as WIP composition, for backlog issues --> name=counter/backlog_(product|bug|technical|ops)
//
// TODO: pass a parameter to enable mismatch logging
func (g *Counters) Generate(events chan store.Event, segmentPrefix string, s *store.PGStore) {
	countMetrics := 0

	for evt := range events {
		loggingMismatches := false

		statusWas := g.statuses[evt.IssueKey] // issue previous status
		statusFrom, statusTo := evt.ValueFrom, evt.ValueTo
		if loggingMismatches && statusWas != statusFrom {
			log.Printf("**MISMATCH** was=%s from=%s for issue %s\n", statusWas, statusFrom, evt.IssueKey)
			// TODO: add info in README/Troubleshooting to explain how to handle these messages
		}

		// Updating issue's previous status
		g.statuses[evt.IssueKey] = statusTo

		g.updateCounters(statusWas, statusTo, evt.IssueType)
		// Using statusWas and not statusFrom to update counters because some
		// status change histories in Jira may be redondant (e.g. you may
		// have twice a change from "Open" to "In Development", maybe because
		// of workflow changes).

		segment := fmt.Sprintf("%s", segmentPrefix)
		countMetrics += g.pushMetrics(s, evt.Time, segment)
	}

	log.Printf("[metrics/counters] pushed %d metrics\n",
		countMetrics,
	)
}

func (g *Counters) updateCounters(from, to, issueType string) {
	switch from {
	case "backlog":
		g.counters["cfd_backlog"]--
	case "wip":
		g.counters["cfd_wip"]--
		g.counters[fmt.Sprintf("wip_%s", issueType)]--
	}

	switch to {
	case "backlog":
		g.counters["cfd_backlog"]++
	case "wip":
		g.counters["cfd_wip"]++
		g.counters[fmt.Sprintf("wip_%s", issueType)]++
	}

	for n, v := range g.counters {
		log.Printf("  counter/%s: %d\n", n, v)
	}
	log.Println("---")
}

func (g *Counters) pushMetrics(s *store.PGStore, t time.Time, segment string) int {
	var countMetrics int
	for name, count := range g.counters {
		countMetrics++
		s.WriteMetric(store.Metric{
			Time:    t,
			Name:    fmt.Sprintf("counter/%s", name),
			Segment: segment,
			Value:   float64(count),
		})
	}
	return countMetrics
}
