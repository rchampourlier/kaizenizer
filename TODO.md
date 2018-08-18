# TODO

## Ongoing

- [x] Fix race condition with new metrics generator (Counters). Batches are not fully written, some events are sent for `WriteMetric` after the store's events chan has been closed.

## Next

- [ ] Fix ratio of issues with status change after resolution (now at > 60%)

