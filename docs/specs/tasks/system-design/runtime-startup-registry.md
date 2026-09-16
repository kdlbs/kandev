---
status: draft
system: tasks
requirements:
  - REQ-TASKS-RUNTIME-CLEANUP-001
created: 2026-09-06
updated: 2026-09-06
owners:
  - cfl
---
# Runtime Startup Registry

## Authority

This document is the exhaustive production starter, subscription, listener,
process, sweep, route-publication, readiness, and cleanup catalog for the archive
recovery gate. [Archive Cascade Boundary Contracts](archive-cascade-boundary-contracts.md#mandatory-recovery-runtime)
owns Runtime behavior; this registry owns exact backend composition symbols and
failure unwind.

`internal/backendapp/startup_registry.go` defines a closed typed catalog. Each
entry contains `ID`, enclosing `OwnerSymbol`, exact `StartSymbol`, stage
`bootstrap_bind|prerequisite|recovery|post_gate|publish`, side-effect kinds,
`Enabled` predicate, exact `StopSymbol`, readiness impact, and failure policy.
The sole production startup seam executes the entries in listed order. An entry
registers its idempotent stop closure before it can expose a second effect.
Disabled optional entries return `not_started` and register nothing.

A syntax/SSA check scans production `internal/backendapp` and its startup-owned
factories for `go`, `Start`, `Run`, `Provide`/`New` constructors documented as
self-starting, event-bus subscribe/register, listener/process creation, sweep,
and handler/readiness publication. Every discovered effect must resolve to one
catalog entry and every catalog entry must resolve to a symbol. Unregistered or
stale entries fail. Constructors in the pre-bind set have a separate purity
check rejecting any such effect. The filename is canonical and is shared by
the registry artifact, Task 04 verifier, and all work-order file lists.

## Exact ordered entries

| ID / stage | OwnerSymbol -> StartSymbol | EffectKinds | StopSymbol / failure boundary | Readiness |
| --- | --- | --- | --- | --- |
| B01a bootstrap_bind | `startApplication` -> `bindBootstrapListeners` | TCP listener bind | `serverListeners.Stop` | liveness true; readiness false |
| B01b bootstrap_bind | `startApplication` -> `bootstrapHTTPServe` | bootstrap handler/serve loop | `http.Server.Close` | liveness true; readiness false |
| Q01 prerequisite | `startRuntimePrerequisites` -> `runInitialAgentSetup` | synchronous agent/profile seed transaction | transaction rollback at return | unchanged |
| Q02 prerequisite | `startRuntimePrerequisites` -> `provideAgentctlLauncher` | agentctl process/control listener | `agentctlLauncherResult.cleanup` | unchanged |
| Q03 prerequisite | `startRuntimePrerequisites` -> `waitForAgentctlControlHealthy` | authenticated health calls | `agentctlHealthContext.Cancel` | unchanged; timeout fatal |
| R01 recovery | `startRecoveryGate` -> `archivecascade.Runtime.Start` | due sweep/claimers/wake workers and task/workspace outbox delivery | `archivecascade.Runtime.Close` | false until success |
| P01 post_gate | `startApplicationProducersAndPublishRoutes` -> `hostnames.Resolver.Start` | hostname worker | `hostnames.Resolver.Close` | false |
| P02 post_gate | `startApplicationProducersAndPublishRoutes` -> `eventBus.QueueSubscribe` | hostname settings subscription | `hostnameSettingsSubscription.Unsubscribe` | false |
| P03 post_gate | `startApplicationProducersAndPublishRoutes` -> `provideGateway` | gateway construction only | `gateway.Close` | false |
| P04 post_gate | `startApplicationProducersAndPublishRoutes` -> `gateways.RegisterSessionStreamNotifications` | session-stream subscriptions | `sessionStreamNotifications.Close` | false |
| P05 post_gate | `startApplicationProducersAndPublishRoutes` -> `hostutility.Manager.Start` | host utility worker/processes | `hostUtilityMgr.Stop` plus `hostUtilityWG.Wait` | false |
| P06a post_gate | `startApplicationProducersAndPublishRoutes` -> `ProfileReconciler.Run` | profile reconciliation | `profileReconciler.Close` | false |
| P06b post_gate | `startApplicationProducersAndPublishRoutes` -> `Utility.MigrateLegacyBindings` | utility binding migration | `utilityMigration.Rollback` | false |
| P06c post_gate | `startApplicationProducersAndPublishRoutes` -> `migrateDefaultUtilityProfile` | default profile migration | `defaultProfileMigration.Rollback` | false |
| P07 post_gate | `startApplicationProducersAndPublishRoutes` -> `orchestrator.Service.Start` | orchestrator recovery/workers | `orchestrator.Service.Stop` | false |
| P08 post_gate | `startApplicationProducersAndPublishRoutes` -> `automation.Service.Start` | automation scheduler/evaluator | `automation.Service.Stop` | false |
| P09 post_gate | `startApplicationProducersAndPublishRoutes` -> `github.Poller.Start` | GitHub polling | `github.Poller.Stop` | false |
| P10 post_gate | `startApplicationProducersAndPublishRoutes` -> `gitlab.Poller.Start` | GitLab polling | `gitlab.Poller.Stop` | false |
| P11 post_gate | `startApplicationProducersAndPublishRoutes` -> `azuredevops.RegisterLifecycleCleanup` | Azure lifecycle subscription | `azureLifecycle.Close` | false |
| P12 post_gate | `startApplicationProducersAndPublishRoutes` -> `azuredevops.Poller.Start` | Azure polling | `azurePoller.Stop` | false |
| P13 post_gate | `startApplicationProducersAndPublishRoutes` -> `jira.Poller.Start` | Jira polling | `jiraPoller.Stop` | false |
| P14 post_gate | `startApplicationProducersAndPublishRoutes` -> `linear.Poller.Start` | Linear polling | `linearPoller.Stop` | false |
| P15 post_gate | `startApplicationProducersAndPublishRoutes` -> `sentry.Poller.Start` | Sentry polling | `sentryPoller.Stop` | false |
| P16 post_gate | `startApplicationProducersAndPublishRoutes` -> `workflowsync.Poller.Start` | workflow sync polling | `workflowSyncPoller.Stop` | false |
| P17 post_gate | `startApplicationProducersAndPublishRoutes` -> `officeconfigsync.Poller.Start` | Office config sync polling | `officeConfigSyncPoller.Stop` | false |
| P18 post_gate | `startApplicationProducersAndPublishRoutes` -> `delivery.Deliverer.Refresh` | plugin event delivery | `pluginDeliverer.Stop` | false |
| P19 post_gate | `startApplicationProducersAndPublishRoutes` -> `plugins.Service.StartActivePlugins` | active plugin processes | `plugins.Service.Shutdown` | false |
| P20 post_gate | `startApplicationProducersAndPublishRoutes` -> `plugins.AutoUpdatePoller.Start` | plugin update polling | `autoUpdatePoller.Stop` | false |
| P22 post_gate | `startApplicationProducersAndPublishRoutes` -> `startTaskUsageWriter` | usage subscription/writer | `taskUsageWriter.Close` | false |
| P23a post_gate | `startApplicationProducersAndPublishRoutes` -> `officeinfra.Reconciler.ReconcileAll` | Office DB reconciliation | `officeReconciliation.Rollback` | false |
| P23b post_gate | `startApplicationProducersAndPublishRoutes` -> `syncSystemSkills` | system skill sync | `systemSkillSync.Rollback` | false |
| P23c post_gate | `startApplicationProducersAndPublishRoutes` -> `backfillAgentDefaultSkills` | default skill backfill | `defaultSkillBackfill.Rollback` | false |
| P24 post_gate | `startApplicationProducersAndPublishRoutes` -> `office.Service.RegisterEventSubscribers` | Office subscriptions | `officeEventSubscribers.Close` | false |
| P25a post_gate | `startApplicationProducersAndPublishRoutes` -> `runsscheduler.Scheduler.Start` | run tick/signal loop | `runScheduler.Stop` | false |
| P25b post_gate | `startApplicationProducersAndPublishRoutes` -> `startCronScheduler` | shared cron loop | `cronLoop.Stop` | false |
| P26 post_gate | `startApplicationProducersAndPublishRoutes` -> `task.Service.StartAutoArchiveLoop` | auto-archive loop | `task.Service.StopAutoArchiveLoop` | false |
| P27 post_gate | `startApplicationProducersAndPublishRoutes` -> `task.Service.StartArchivedSessionReconciliationLoop` | archived-session loop | `task.Service.StopArchivedSessionReconciliationLoop` | false |
| P28 post_gate | `startApplicationProducersAndPublishRoutes` -> `task.Service.StartQuickChatExpirationLoop` | quick-chat loop | `task.Service.StopQuickChatExpirationLoop` | false |
| P29a post_gate | `startApplicationProducersAndPublishRoutes` -> `provideStorageComposition` | artifact/quarantine reconciliation | `storageComposition.Close` | false |
| P29b post_gate | `startApplicationProducersAndPublishRoutes` -> `system.Service.StartBackground` | storage/update/metrics workers | `system.Service.StopBackground` | false |
| P30a post_gate | `startApplicationProducersAndPublishRoutes` -> `review.Runner.Start` | review worker | `review.Runner.Stop` | false |
| P30b post_gate | `startApplicationProducersAndPublishRoutes` -> `CancelInFlightTaskReviewRuns` | stale review repair | `reviewRepair.Rollback` | false |
| P31 post_gate | `startApplicationProducersAndPublishRoutes` -> `gateways.RegisterSystemNotifications` | system subscriptions | `systemNotifications.Close` | false |
| P32 post_gate | `startApplicationProducersAndPublishRoutes` -> `gateways.RegisterAgentRuntimeNotifications` | runtime subscriptions | `agentRuntimeNotifications.Close` | false |
| P33 post_gate | `startApplicationProducersAndPublishRoutes` -> `buildHTTPServer` | real route construction | `builtServer.Close` | false |
| U01 publish | `publishApplication` -> `publishReadiness` | ready flag and handler swap | `restoreBootstrapHandler` | readiness true only after all P entries |

`initOfficeServices`, `startPluginsSubsystems`, `provideGateway`,
`provideStorageComposition`, `buildHTTPServer`, and route-registration helpers
are split into pure construction and the exact entries above. In particular,
review startup cannot remain hidden in `registerMCPAndDebugRoutes`, host utility
cannot start twice, subscription helpers must return cleanup, and no
warning-only path may leak a partial resource. Optional feature absence skips an
entry; a configured entry's start/reconciliation error is fatal.

## Failure and reverse unwind

The runner wraps each entry in a partial-start guard. Panic, error, root
cancellation, or health timeout invokes the current entry's cleanup if acquired,
then completed entries in strict reverse ID order. B01 closes last. Q01 has no
long-lived resource; Q02 is registered before Q03. R01 is registered before P01.
U01 is never called after any failure. A failure after U01 first restores the
bootstrap not-ready handler, then unwinds P, R, Q, and B. Every stop is `sync.Once`
and all concurrent shutdown, reset, restore, and startup-failure callers receive
the stored result.

The failure matrix injects before start, after each individual side effect but
before cleanup registration, after cleanup registration, and after successful
return for B01, Q02, R01, every P entry, and U01. Assertions cover no surviving
goroutine, subscription, process, listener, timer, worker, real route, or ready
flag; cleanup order and count are exact. Health remains available only while B01
is intentionally retained for transient Runtime retry.

## Verification ownership

Work order 03 implements the typed catalog and sole runner. Work order 04 scans
all production starter effects independently, compares symbol/stage/cleanup/
readiness fields exactly, and executes every partial-failure cell with instrumented
factories. Tests also prove no pre-bind constructor starts, no producer precedes
R01 success, all post-gate entries precede U01, and optional-feature combinations
cannot change those invariants.
