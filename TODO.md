# TODO

- [ ] Lead Time
- [ ] Cycle Time
- [ ] Number of Issues in _Backlog_ status
- [ ] Number of Issues in _WIP_ status
- [ ] Number of Issues in _Done_ status
- [ ] Number of Active Developers
- [ ] Ratio Active Developers / WIP

## Implementation

### Lead Time

For each `status_changed` event:

- Map the `valueTo` to a _metrics status_ (_Backlog_, _WIP_ or _Done_) -> `new_status`
- Retrieve the current status in the map -> `old_status`
- If `old_status` != `new_status`:
  - update the counters (decrement for `old_status` and increment for `new_status`)
  - update the status in the map with `new_status`
- Else do nothing
