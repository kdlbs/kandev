# Startup listener before recovery

The authoritative lifecycle contract is now [startup lifecycle requirements](../platform/requirements/startup-lifecycle.md)
and [startup lifecycle design](../platform/system-design/startup-lifecycle.md).

The background lifecycle-token sweep still starts after watcher and scheduler
subscriptions. It does not gate readiness. Its per-task locks and atomic metadata
claims prevent duplicate effects during live event delivery. This change does not
alter its bounded dispatch deadline or in-flight resume behavior.
