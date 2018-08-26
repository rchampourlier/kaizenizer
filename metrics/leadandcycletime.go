package metrics

import (
	"fmt"
	"log"
	"time"

	"github.com/rchampourlier/kaizenizer/store"
)

type period struct {
	startSet, endSet bool
	start, end       time.Time
}

// LeadAndCycleTime implements `Generator` for the _Cycle Time_
// metric.
type LeadAndCycleTime struct {
	cyclePeriods map[string]period // issue key -> cycle time period
	leadPeriods  map[string]period // issue key -> lead time period
}

// NewLeadAndCycleTime returns an initialized LeadAndCycleTime struct.
func NewLeadAndCycleTime() *LeadAndCycleTime {
	return &LeadAndCycleTime{
		make(map[string]period),
		make(map[string]period),
	}
}

// Generate generates the Lead Time metrics.
func (g *LeadAndCycleTime) Generate(events chan store.Event, segmentPrefix string, s *store.PGStore) {
	var countIssues, countMetrics int

	for evt := range events {
		ik, to := evt.IssueKey, evt.ValueTo

		if _, ok := g.cyclePeriods[ik]; !ok {
			// First time the issue appears in an event
			countIssues++
			g.cyclePeriods[ik] = period{}
			g.leadPeriods[ik] = period{startSet: true, start: evt.Time}
		}

		switch to {

		case "backlog":
			// Doing nothing, only lead is affected but start
			// is already set when the issue is seen the first
			// time (whichever status it's in).

		case "wip":
			p := g.cyclePeriods[ik]
			if !p.startSet {
				// Ignoring if the cycle already started, so we
				// don't overwrite it.
				p.startSet, p.start = true, evt.Time
				g.cyclePeriods[ik] = p
			}

		case "done":
			p := g.cyclePeriods[ik]
			p.endSet, p.end = true, evt.Time
			// Here we want to overwrite in case the cycle was "done"
			// once but the issue was reopened and is now "done" again.
			g.cyclePeriods[ik] = p

		case "resolved":
			p := g.leadPeriods[ik]
			p.endSet, p.end = true, evt.Time
			// Same here for the lead time.
			g.leadPeriods[ik] = p
		}
	}

	for k := range g.cyclePeriods {
		leadPeriod, cyclePeriod := g.leadPeriods[k], g.cyclePeriods[k]

		if ok, dur := periodDurationInDays(leadPeriod); ok {
			countMetrics++
			s.WriteMetric(store.Metric{
				Time:    leadPeriod.end,
				Name:    "lead_time",
				Segment: fmt.Sprintf("%s", segmentPrefix),
				Value:   float64(dur),
				Comment: k,
			})
		}

		if ok, dur := periodDurationInDays(cyclePeriod); ok {
			countMetrics++
			s.WriteMetric(store.Metric{
				Time:    cyclePeriod.end,
				Name:    "cycle_time",
				Segment: fmt.Sprintf("%s", segmentPrefix),
				Value:   float64(dur),
				Comment: k,
			})
		}
	}

	log.Printf("[metrics/leadtime] pushed %d metrics (for %d issues)\n",
		countMetrics,
		countIssues,
	)
}

// Returns (true, <duration>) if period has both start and
// end set, otherwise returns (false, 0).
func periodDurationInDays(p period) (bool, float64) {
	if !p.startSet || !p.endSet {
		return false, 0
	}
	return true, float64(p.end.Sub(p.start)) / float64(24*time.Hour)
}
