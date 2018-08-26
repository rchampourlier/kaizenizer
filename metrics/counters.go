package metrics

import (
	"fmt"
	"log"
	"time"

	"github.com/rchampourlier/kaizenizer/store"
)

var metrics = []string{
	"cfd_wip", "cfd_backlog",
	"wip_product", "wip_bug", "wip_technical", "wip_ops",
	"backlog_product", "backlog_bug", "backlog_technical", "backlog_ops",
}

// Counters implements `Generator` for the _Counters_
// metric.
type Counters struct {
	counters map[string]map[string]int // name -> segment -> count
	statuses map[string]string         // issue key -> previous status
}

// NewCounters returns a `Counters` struct initialized with internal
// data.
func NewCounters() *Counters {
	counters := make(map[string]map[string]int)
	for _, m := range metrics {
		counters[m] = make(map[string]int)
	}

	return &Counters{
		counters,
		make(map[string]string),
	}
}

// Generate generates metrics for counts of issues based on their status.
//
// Generated metrics:
//   - Cumulative Flow Diagram: unresolved issues, split between backlog and WIP --> name=cfd_(wip|backlog)
//   - WIP composition: WIP issues, split between product, bug, technical, ops --> name=wip_(product|bug|technical|ops)
//   - Backlog composition: same as WIP composition, for backlog issues --> name=backlog_(product|bug|technical|ops)
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

		g.updateCounters(statusWas, statusTo, evt.IssueType, evt.Segment)
		// Using statusWas and not statusFrom to update counters because some
		// status change histories in Jira may be redondant (e.g. you may
		// have twice a change from "Open" to "In Development", maybe because
		// of workflow changes).

		countMetrics += g.pushMetrics(s, evt.Time, segmentPrefix)
	}

	log.Printf("[metrics/counters] pushed %d metrics\n",
		countMetrics,
	)
}

func (g *Counters) updateCounters(from, to, issueType, segment string) {
	switch from {
	case "backlog":
		g.counters["cfd_backlog"][segment]--
		g.counters[fmt.Sprintf("backlog_%s", issueType)][segment]--
	case "wip":
		g.counters["cfd_wip"][segment]--
		g.counters[fmt.Sprintf("wip_%s", issueType)][segment]--
	}

	switch to {
	case "backlog":
		g.counters["cfd_backlog"][segment]++
		g.counters[fmt.Sprintf("backlog_%s", issueType)][segment]++
	case "wip":
		g.counters["cfd_wip"][segment]++
		g.counters[fmt.Sprintf("wip_%s", issueType)][segment]++
	}
}

func (g *Counters) pushMetrics(s *store.PGStore, t time.Time, segmentPrefix string) int {
	var countMetrics int
	for metricName, segments := range g.counters {
		for segment, value := range segments {
			countMetrics++
			s.WriteMetric(store.Metric{
				Time:    t,
				Name:    fmt.Sprintf("counter/%s", metricName),
				Segment: fmt.Sprintf("%s/%s", segmentPrefix, segment),
				Value:   float64(value),
			})
		}
	}
	return countMetrics
}
