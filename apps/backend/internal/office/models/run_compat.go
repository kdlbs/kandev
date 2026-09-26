package models

import runsmodels "github.com/kandev/kandev/internal/runs/models"

// Run preserves the Office package name for consumers during the migration.

// Deprecated: Use internal/runs/models.Run.
type Run = runsmodels.Run

// RunStatus preserves the Office package name for consumers during the migration.

// Deprecated: Use internal/runs/models.RunStatus.
type RunStatus = runsmodels.RunStatus

// RunEvent preserves the Office package name for consumers during the migration.

// Deprecated: Use internal/runs/models.RunEvent.
type RunEvent = runsmodels.RunEvent

// RunEventLevel preserves the Office package name for consumers during the migration.

// Deprecated: Use internal/runs/models.RunEventLevel.
type RunEventLevel = runsmodels.RunEventLevel

// RunEventType preserves the Office package name for consumers during the migration.

// Deprecated: Use internal/runs/models.RunEventType.
type RunEventType = runsmodels.RunEventType

// ActorKind preserves the Office package name for consumers during the migration.

// Deprecated: Use internal/runs/models.ActorKind.
type ActorKind = runsmodels.ActorKind

// PriorityClass preserves the Office package name for consumers during the migration.

// Deprecated: Use internal/runs/models.PriorityClass.
type PriorityClass = runsmodels.PriorityClass

// RoutingBlockedStatus preserves the Office package name for consumers during the migration.

// Deprecated: Use internal/runs/models.RoutingBlockedStatus.
type RoutingBlockedStatus = runsmodels.RoutingBlockedStatus

// Deprecated: Use internal/runs/models.RunStatusQueued.
const RunStatusQueued = runsmodels.RunStatusQueued

// Deprecated: Use internal/runs/models.RunStatusClaimed.
const RunStatusClaimed = runsmodels.RunStatusClaimed

// Deprecated: Use internal/runs/models.RunStatusFinished.
const RunStatusFinished = runsmodels.RunStatusFinished

// Deprecated: Use internal/runs/models.RunStatusFailed.
const RunStatusFailed = runsmodels.RunStatusFailed

// Deprecated: Use internal/runs/models.RunEventLevelInfo.
const RunEventLevelInfo = runsmodels.RunEventLevelInfo

// Deprecated: Use internal/runs/models.RunEventLevelWarn.
const RunEventLevelWarn = runsmodels.RunEventLevelWarn

// Deprecated: Use internal/runs/models.RunEventLevelError.
const RunEventLevelError = runsmodels.RunEventLevelError

// Deprecated: Use internal/runs/models.RunEventTypeInit.
const RunEventTypeInit = runsmodels.RunEventTypeInit

// Deprecated: Use internal/runs/models.RunEventTypeAdapterInvoke.
const RunEventTypeAdapterInvoke = runsmodels.RunEventTypeAdapterInvoke

// Deprecated: Use internal/runs/models.RunEventTypeStep.
const RunEventTypeStep = runsmodels.RunEventTypeStep

// Deprecated: Use internal/runs/models.RunEventTypeComplete.
const RunEventTypeComplete = runsmodels.RunEventTypeComplete

// Deprecated: Use internal/runs/models.RunEventTypeError.
const RunEventTypeError = runsmodels.RunEventTypeError

// Deprecated: Use internal/runs/models.RunEventTypeRuntimeDenied.
const RunEventTypeRuntimeDenied = runsmodels.RunEventTypeRuntimeDenied

// Deprecated: Use internal/runs/models.RunEventTypeRuntimeAction.
const RunEventTypeRuntimeAction = runsmodels.RunEventTypeRuntimeAction

// Deprecated: Use internal/runs/models.ActorKindUser.
const ActorKindUser = runsmodels.ActorKindUser

// Deprecated: Use internal/runs/models.ActorKindAgent.
const ActorKindAgent = runsmodels.ActorKindAgent

// Deprecated: Use internal/runs/models.ActorKindSystem.
const ActorKindSystem = runsmodels.ActorKindSystem

// Deprecated: Use internal/runs/models.PriorityClassRecovery.
const PriorityClassRecovery = runsmodels.PriorityClassRecovery

// Deprecated: Use internal/runs/models.PriorityClassEvent.
const PriorityClassEvent = runsmodels.PriorityClassEvent

// Deprecated: Use internal/runs/models.PriorityClassPeriodic.
const PriorityClassPeriodic = runsmodels.PriorityClassPeriodic

// Deprecated: Use internal/runs/models.PriorityClassHuman.
const PriorityClassHuman = runsmodels.PriorityClassHuman

// Deprecated: Use internal/runs/models.RoutingBlockedWaitingForCapacity.
const RoutingBlockedWaitingForCapacity = runsmodels.RoutingBlockedWaitingForCapacity

// Deprecated: Use internal/runs/models.RoutingBlockedActionRequired.
const RoutingBlockedActionRequired = runsmodels.RoutingBlockedActionRequired
