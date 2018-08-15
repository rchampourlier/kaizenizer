package metrics

import (
	"fmt"
	"log"
	"time"

	"github.com/rchampourlier/kaizenizer-jira-metrics/store"
)

// LeadTime generates the _Lead Time_ metrics.
func LeadTime(s *store.PGStore, events chan store.Event, segmentPrefix string) {
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
				//log.Printf("processing event for resolved issue: %s\n", evt)
			} else {
				if evt.ValueTo == "done" {
					b := bounds{
						complete: true,
						start:    m[evt.IssueKey].start,
						end:      evt.Time,
					}
					m[evt.IssueKey] = b
					segment := fmt.Sprintf("%s/%s", segmentPrefix, evt.IssueKey)
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
	log.Printf("pushed %d `%s` metrics for %d issues (%.1f%% issues with status changes after resolution)\n",
		countMetrics,
		"lead_time",
		countIssues,
		float32(len(reopenedIssues))/float32(countIssues)*100,
	)
}
