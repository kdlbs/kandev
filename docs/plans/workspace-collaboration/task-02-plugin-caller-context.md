---
id: "02-plugin-caller-context"
title: "Carry verified conversation callers through plugin Host callbacks"
status: done
wave: 2
depends_on:
  - 01-conversation-grants
plan: plan.md
requirements:
  - REQ-TASKS-WORKSPACE-COLLABORATION-001
acceptance_criteria:
  - AC-TASKS-WORKSPACE-COLLABORATION-001.1
  - AC-TASKS-WORKSPACE-COLLABORATION-001.2
  - AC-TASKS-WORKSPACE-COLLABORATION-001.4
  - AC-TASKS-WORKSPACE-COLLABORATION-001.5
  - AC-TASKS-WORKSPACE-COLLABORATION-001.6
  - AC-TASKS-WORKSPACE-COLLABORATION-001.7
  - AC-TASKS-WORKSPACE-COLLABORATION-001.8
system_design:
  - ../../specs/tasks/system-design/workspace-collaboration.md
legacy_specs: []
---

# Task 02: Plugin caller context

Use the existing SDK/gRPC connection and broker, with a bounded per-Host live
invocation map and metadata propagation from real handler contexts. Restore the
server's principal and owner context, keeping native plugin capabilities and
domain action checks. Reject ambiguous domain callbacks only while enabled.
Preserve off behavior, saved grants, concurrent native RPC work and Task 01's
domain corrections. Do not infer identity from tool JSON or a shared Host field.

Record focused existing SDK/plugin checks and disposable actual RPC/domain
proofs here. No permanent tests, new poller, queue, protocol framework, provider
execution, operating permission changes or Git publication by this worker.
At this Task 02 checkpoint, profile inheritance, delegation/taskless callers,
aggregate navigation and the full grant-editor UI remained subsequent work.
Later changes in this PR add the desktop/mobile grant-editor entry points and
`message_task_kandev` report delivery. Profile inheritance, delegation/taskless
callers and aggregate navigation remain follow-up scope.

## Implemented behavior

The SDK owns a live per-Host RPC context map, the outbound plugin connection
wrapper, incoming Host interceptor and return-metadata forwarding. Backend
composition validates the existing conversation principal against stored task,
session and current workspace rows. Host capabilities and the original native
owner remain independent checks. There is no proto, persistent state or operating
configuration change. Tool InvocationID remains correlation data; fresh server
references prevent reuse from reviving a returned call. Explicit off preserves
the native path and grants. Unmarked legacy domain callbacks fail closed while
enabled; native event/action/webhook callbacks keep their actual server context.

Actual broker verification found a control-only legacy update incorrectly
requiring read for its internal task/version and management-claim preflight.
The task domain now exposes only a TaskWrite-authorized placement/version
snapshot and TaskWrite-authorized claim read to that adapter. Update and Move
retain their existing CAS/version/claim-generation fences. General task/claim
reads still require read permission; no implicit grant is added.

## Executed verification (2026-10-15)

Go working directory: `apps/backend`. `OVERLAY` is the disposable machine-local
overlay outside normal test collection. The proof uses the existing
`hcplugin.TestPluginGRPCConn` broker fixture, real `RemotePlugin`, plugin service,
production plugin-bound Host, existing SQLite fixture and actual task service.
The author plugin and message-launch boundary are fakes; no model, installed
plugin, provider, operating database or permission setting is exercised.

```text
go test -trimpath ./pkg/pluginsdk ./internal/plugins ./internal/backendapp -run '^Test(AgentTool|RemotePlugin|GRPC|Host|PluginTool)' -count=1 -timeout 120s
ok pkg/pluginsdk 0.367s
ok internal/plugins 0.449s
ok internal/backendapp 0.326s

go test -trimpath ./pkg/pluginsdk -run '^TestServe_EndToEnd$' -count=1 -timeout 90s
ok pkg/pluginsdk 0.068s

go test -trimpath ./internal/backendapp -run '^Test(PluginsTaskWriter_.*(Update|Move)|ResolvePluginMoveWorkflowID|ConcurrentReassignmentSurvivesMatchingStepStaleMove|ClassifyPluginMoveError)' -count=1 -timeout 90s
ok internal/backendapp 3.150s

go test -trimpath ./internal/task/service -run '^Test(TaskManagementClaim(Fencing|FenceSerializesTransferAndReleaseEffects)|ExactTaskUpdate)' -count=1 -timeout 90s
ok internal/task/service 0.436s

go test -trimpath ./internal/runtimeflags -run '^Test.*(Registry|Feature|Definition)' -count=1 -timeout 90s
ok internal/runtimeflags 0.039s

go run -trimpath ./cmd/sqlguard ./internal
exit 0
```

The first complete disposable broker invocation used:

```text
go test -trimpath -overlay OVERLAY ./internal/backendapp -run '^TestPluginCallerProbeActualBrokerDomain$' -count=1 -timeout 90s -v
```

Its control-only Update exposed the preflight defect above, so that overall run
failed. Its four independent missing-origin/returned-reference, native-permission,
concurrent-conversation/native-event/other-plugin, and cancel/off cases passed.
Those valid results were retained instead of recreating or repeating them after
the isolated preflight correction. Corrected and strengthened cases ran as:

```text
go test -trimpath -overlay OVERLAY ./internal/backendapp -run '^TestPluginCallerProbeActualBrokerDomain$/^independent-domain-actions-and-payload-identity$' -count=1 -timeout 90s -v
PASS (0.53s); ok internal/backendapp 0.801s

go test -trimpath -p 2 -race -overlay OVERLAY ./internal/backendapp -run '^TestPluginCallerProbeActualBrokerDomain$/^concurrent-conversations-native-event-and-other-plugin$' -count=1 -timeout 90s -v
PASS (0.34s); ok internal/backendapp 1.570s

go test -trimpath -p 2 -overlay OVERLAY ./internal/backendapp -run '^TestPluginCallerProbeActualBrokerDomain$/^missing-legacy-origin-and-returned-reference$' -count=1 -timeout 90s -v
PASS (0.29s); ok internal/backendapp 0.476s
```

The real broker/domain cases confirmed own-workspace preservation, independent
read/prompt/control effects, report-only denial of generic Host prompts and
mutations, request task fields not replacing the principal, native owner and
plugin-capability denial, live concurrent conversation separation, concurrent
native event work, other-plugin reference rejection, dropped context rejection,
cancel/late callback rejection and explicit off retaining saved grants. The
strengthened late case kept a following invocation with the same correlation ID
active while attempting the old reference. Actual SQLite message/title effects
and active/released nonzero management-claim fences were read back.

The dedicated positive report HTTP/WS primitive remains covered by Task 01's
valid proof; adding an advertised plugin report consumer is separate original
work. No generic Host message was reclassified as a report. Recheck only changed
source/input or new failure evidence, not the absence of deleted proof files.

Final changed-package lint completed naturally with exit 0 and `0 issues.`:

```text
golangci-lint run ./pkg/pluginsdk ./internal/plugins ./internal/backendapp ./internal/task/service ./internal/runtimeflags --new-from-rev=HEAD --timeout=5m --concurrency=2
```

It used process-local `GOMEMLIMIT=3GiB`, `GOMAXPROCS=2`, and `GOFLAGS=-p=2` after
the previous lint owner returned the shared slot. No parallel-lock bypass or
suppression was used. The slot was returned after collecting the actual result.
Integration retains normal staged hooks, Git actions and publication.

Existing documentation catalog validation passed (383 decisions and 1617
specifications), specification lint and architecture lint passed, and existing
`pr-docs.cjs.validateCoverage` accepted the actual 14 changed paths and four
document bodies with `status: covered`, `errors: []`. Diff whitespace checks
passed. Disposable overlay/probe/helper files were removed after readback;
their absence does not invalidate the recorded execution. No permanent tests or
new verification runner were added. Source installation, real agent/provider
execution and operating activation remain untested and unchanged.

## Plugin-authored event origin correction

Independent source review found one laundering path: EmitEvent was exempt from
context restoration, and the existing delivery worker discarded publication
context before opening a native OnEvent RPC. An agent without a target-workspace
grant could therefore emit its own subscribed event and access that target from
OnEvent, despite direct Host denial.

EmitEvent remains available for notification. While enabled, its server-created
bus envelope carries only the emitting plugin and a live opaque reference, never
payload caller IDs. The existing delivery queue, ring buffer and RuntimeTransport
resolve that reference against the issuing RemotePlugin's original live map on
every attempt. Valid contexts retain conversation/native owner restrictions and
cancellation. Unknown or expired references, including after serialization,
retry or plugin restart, deliver notifications with domain Host authority denied.
This adds no map, table, queue, poller or proto change. Native domain events after
an authorized state change retain the registered automation owner's independent
authority; this correction does not add causal taint to ordinary task events.

The disposable event probe and overlays are outside normal test collection in
the assigned transient probe location and are retained for integration readback.
They use actual go-plugin broker RPCs, production Host and RuntimeTransport,
existing event delivery, native owner authorization and SQLite/task service.
The author plugin, runtime lookup and JSON round-trip wrapper are fixtures; no
model, installed plugin, NATS server or operating database was used.

```text
go test -trimpath -p 2 -overlay BEFORE_OVERLAY ./internal/backendapp -run '^TestPluginEventOriginActualBrokerSQLite$/^no-grant$' -count=1 -timeout 60s -v
FAIL: expected direct=false;read=false;write=false, actual direct=false;read=true;write=true.
BEFORE_OVERLAY replaces only internal/plugins/host.go with its pre-correction source.

go test -trimpath -p 2 -race -overlay OVERLAY ./internal/backendapp -run '^TestPluginEventOriginActualBrokerSQLite$' -count=1 -timeout 90s -v
PASS (2.11s); ok internal/backendapp 3.369s.
```

The seven subcases are `no-grant`, `read-only`, `read-control`,
`serialized-no-grant`, `serialized-read-only`, `retry-after-return` and
`legacy-off`. They check actual read/write results and SQLite title effects,
forged payload identity, returned-reference notification without authority,
unmarked startup/background emission, and independent native event access.
The serialized read case also replaces the broker/RemotePlugin and rejects the
old reference. The retry case holds its second resolution until the issuing RPC
has returned, then confirms read and mutation denial. Existing direct domain,
concurrency, cancellation, report and native-permission proofs above remain
valid and were not recreated.

```text
go test -trimpath -p 2 ./pkg/pluginsdk ./internal/plugins/delivery ./internal/events/bus -run '^Test(Serve_EndToEnd|Deliverer|MemoryEventBus_PublishStampsSubject|NATS)' -count=1 -timeout 90s
ok pkg/pluginsdk 0.050s; internal/plugins/delivery 0.981s; internal/events/bus 0.059s.

golangci-lint run ./pkg/pluginsdk ./internal/plugins ./internal/plugins/delivery ./internal/events/bus --new-from-rev=HEAD --timeout=5m --concurrency=2
exit 0; 0 issues. Process-local resource settings were the same as above.
```

The existing `pr-docs.cjs.validateCoverage` accepted the final 20 changed paths
and four document bodies with `status: covered`, `errors: []`. Diff whitespace
checks passed. The integration owner retains the probe sources and raw red/green
outputs until readback and owns cleanup and normal commit hooks.

## Integration correction (2026-10-29)

The fixed method list that excluded only state/config/secrets/events made every
newer plugin-owned Host operation require a caller. Detached and background
plugin work therefore failed with `PermissionDenied` when collaboration was
enabled even though those capabilities had kept native authority before the
feature. The SDK now classifies every Host service method explicitly:
conversation-domain task, workspace, session, message, interaction, and
execution operations require the originating server context; plugin-owned
capability operations keep their existing rules.

The committed broker tests cover the generated-service classification, a
background plugin-owned call succeeding while enabled, a background
conversation-domain call and a dropped handler context staying closed while
enabled, a forwarded handler context reaching the Host, and the disabled path
keeping native background behavior. The existing event-origin, concurrent
caller, cancellation, expired-reference, and real-domain probe results remain
valid because the restore and fail-closed paths are unchanged.

The review rounds that followed hardened the reference channel: the SDK strips
the call reference from handler-visible metadata and carries it in a private,
non-formatted context value, so accidental metadata or log exposure does not
hand it to plugin code. The reference remains a bearer capability for the live
invocation. A handler that deliberately wraps its context can still recover it,
and using it is the handler's existing authority. The approved scope forbids a
new signature/hash scheme, so no cryptographic binding is added. Missing,
expired, returned, canceled, and other-plugin references stay closed.
