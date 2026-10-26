# Nomad API response fixtures

`nomad-2.0.7-job-stopped.json` and `nomad-2.0.7-allocation-stopped.json` are
sanitized snapshots of the full `GET /v1/job/:id` and
`GET /v1/allocation/:id` responses from the task-owned local Nomad 2.0.7
fixture used on 2026-10-25. The allocation snapshot includes the server's
embedded `Job` object. The job was already `dead` with `Stop=true`; these files
are negative compatibility evidence and must never authorize a ready reattach
or claim successful current-policy execution.

The source response passed the current job-field schema walk and showed the
2.0.7 no-op `Update` object and default network `HostNetwork` values. The
fixtures retain that shape and behavior, while replacing fixture IDs and
task-owned absolute paths. A scan of the captured task environment found no
credential/nonce/token/password/private-key-named variables; raw environment
values and the original machine-specific identifiers/paths were not copied.

These snapshots establish response-shape compatibility only. A current native
job that passes the full immutable-policy comparison still requires a fresh
normal-API scenario against the pinned 2.0.7 server and allocation.
