package metrics

import (
	"fmt"
	"log"
	"time"

	"github.com/rchampourlier/kaizenizer/store"
)

// LeadTime implements `Generator` for the _Cycle Time_
// metric.
type LeadTime struct{}

// Generate generates the _Lead Time_ metrics.
func (g LeadTime) Generate(events chan store.Event, segmentPrefix string, s *store.PGStore) {
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
			} else {
				if evt.ValueTo == "done" {
					b := bounds{
						complete: true,
						start:    m[evt.IssueKey].start,
						end:      evt.Time,
					}
					m[evt.IssueKey] = b
					segment := fmt.Sprintf("%s", segmentPrefix)
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
	log.Printf("[metrics/leadtime] pushed %d metrics (for %d issues, %.1f%% with status changes after resolution)\n",
		countMetrics,
		countIssues,
		float32(len(reopenedIssues))/float32(countIssues)*100,
	)
}
