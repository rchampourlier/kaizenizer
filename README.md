# Kaizenizer Metrics

## Metrics

- [x] Lead Time
- [x] Cycle Time
- [x] Number of Issues in `backlog` status
- [x] Number of Issues in `wip` status
- [x] Number of Issues in `done` status
- [x] Number of Issues in `resolved` status
- [ ] Number of Active Developers
- [ ] Ratio Active Developers / WIP

### Implementation

For all metrics, events are loaded from the database's `jira_issues_events` table and converted to `store.Event` structs. 

For status changes, the `valueFrom` and `valueTo` fields will contain one of the following statuses:

- `backlog`
- `wip`
- `done`
- `resolved`

These statuses are mapped from Jira original statuses. The mapping is done in `store/pgstore.go:statusGroup()`.

### Lead Time

_Lead Time_ is the time an issue took to get from creation to _resolved_.

- The creation is detected by using the first `status_changed` event (ordered on `event_time`).
- The resolution is defined by the `status_changed` event when the issue moves to `resolved`.
- The time of the metric is the time of resolution of the issue.

#### Implementation details

- Create a map that will store the `start` and `end` time of the Lead Time for each issue.
- Process each `status_changed` event. For each event:
  - If the issue is not in the map:
    - Add the issue to the map with `event.Time` as `start`
  - Else:
    - If the issue is already in the map:
      - If `end` is already set:
        - Log a message indicating an event has been processed for an issue with the Lead Time already pushed
      - Else:
        - If the `event.ValueTo` (new status) is `resolved`, update `end` with `event.Time`
        - Push a metric with the Lead Time for this issue

#### Limitations

- Status changes after the first resolution are ignored (however, they are counted and a statistics is displayed when the metric is generated)

### Cycle Time

_Cycle Time_ is the duration an issue took to get from _wip_ to _done_.

The implementation is similar to Lead Time's.

### Flow Diagram Counters

Counts the number of issues in each status. A metric is pushed for all counters each time the status of an issue changes.

#### Implementation details

- Create a map (issue -> current status)
- Create a map (status -> count) 
- For each `status_changed` event:
  - If the issue is not in the map:
    - Add it to the map with `event.ValueTo` as value
  - Else:
    - If the status in the map is different from `event.ValueTo`:
      - Decrement the counter for the issue status from the map
      - Update the value with `event.ValueTo`
      - Increment the counter for the status `event.ValueTo`
  - Push a metric for each counter

## License

MIT

