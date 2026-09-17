# Historical local preview testing

The original coordinator implementation was tested in an isolated preview with
separate storage, configuration, ports and assets. It did not replace the live
installation. Installation-specific paths and conversation-derived examples are
not part of the review repository.

For future testing and live rollout, use the versioned candidate, migration
rehearsal and matched rollback process in
[the delivery plan](../orchestration-delivery/plan.md). A successful old preview
does not establish that a new candidate can migrate current live data.
