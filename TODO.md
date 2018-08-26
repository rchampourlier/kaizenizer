# TODO

## Ongoing

- [x] Handle issues that are reopened when calculating lead and cycle time
  - [x] Add a comment column in metrics and specify the issue key

## Next

- [ ] Deploy

### Other metrics

- [ ] Age of backlog issues (e.g. >1yr, 6-12mo, >1mo...)
- [ ] Lead/Cycle Time per issue type
  - [ ] as a Grafana dashboard parameter
- [ ] Lead/Cycle Time per tribe
  - [ ] as a Grafana dashboard parameter
- [ ] Age of backlog issues
- [ ] Age of WIP issues

## Future

- [ ] Make segments, statuses and other data customizable on Jira a configuration
- [ ] Add index on `segment` column
- [ ] Tests (for lead/cycle time, can use issue JT-7833 as an example)
