import { ApiError, fetchJson, type ApiRequestOptions } from "@/lib/api/client";

// Mirrors internal/coordinator/validate.go's ProfileStatus.
export type ProfileStatus = "ok" | "missing" | "passthrough";

// Mirrors internal/coordinator/models.go's ProposalStatus.
export type ProposalStatus =
  | "pending"
  | "approving"
  | "approved"
  | "rejected"
  | "failed"
  | "returned";

// Mirrors internal/coordinator/models.go's ProposalSpec.
export type ProposalSpec = {
  title: string;
  description: string;
  rationale: string;
  workflow_id: string;
  step_id: string;
  repository_id: string;
  source_task_id: string;
};

// Mirrors internal/coordinator/dto.go's CoordinatorDTO (Build decision 9).
// open_proposals is set only by the list route; agent_profile_status and
// executor_profile_status are set only by GET.
export type Coordinator = {
  id: string;
  workspace_id: string;
  name: string;
  agent_profile_id: string;
  executor_profile_id: string;
  context: string;
  conversation_task_id: string | null;
  created_at: string;
  updated_at: string;
  open_proposals?: number;
  agent_profile_status?: ProfileStatus;
  executor_profile_status?: ProfileStatus;
  // Present only while features.coordinatorPhase2 is on.
  policy?: { actions: Record<string, string> };
  policy_revision?: number;
  watches?: { scope: "all" | "selected"; workflow_ids: string[] };
  // Present only while features.coordinatorPhase3 is effective. The ceiling is
  // a dollar string with two decimals, or null when unset.
  autonomy_enabled?: boolean;
  cost_ceiling_usd?: string | null;
  // Present only on the list route while features.coordinatorPhase2 is on.
  summary?: CoordinatorSummary;
};

// Mirrors internal/coordinator/dto.go's SummaryDTO.
export type CoordinatorSummary = {
  watch_scope: "all" | "selected";
  watched_count: number;
  approval_actions: number;
  active_orders: number;
};

export type ControlAction = "create_task" | "start_agent" | "message" | "move" | "resume" | "stop";
export type ControlSetting = "denied" | "requires_approval" | "automatic";

// Mirrors GET/PUT .../settings (docs/specs/coordinator/system-design/permissions.md#settings-routes).
export type CoordinatorSettings = {
  policy: { actions: Record<ControlAction, ControlSetting> };
  policy_revision: number;
  watches: { scope: "all" | "selected"; workflow_ids: string[] };
};

export type PutSettingsRequest = {
  policy?: { actions: Record<ControlAction, ControlSetting> };
  watches?: { scope: "all" | "selected"; workflow_ids?: string[] };
};

export type CoordinatorListResponse = {
  coordinators: Coordinator[];
};

// Mirrors internal/coordinator/dto.go's CreateCoordinatorRequest.
export type CreateCoordinatorRequest = {
  name: string;
  agent_profile_id: string;
  executor_profile_id: string;
  context?: string;
};

// Mirrors internal/coordinator/dto.go's PatchCoordinatorRequest (Build
// decision 7). A field absent from this object is dropped by
// JSON.stringify, which the backend reads as "unchanged"; the backend
// rejects JSON null for any of these fields with a 400, so this type never
// allows null.
export type PatchCoordinatorRequest = {
  name?: string;
  agent_profile_id?: string;
  executor_profile_id?: string;
  context?: string;
  // Phase 3: the ceiling is a two-place dollar string, or null to clear it.
  autonomy_enabled?: boolean;
  cost_ceiling_usd?: string | null;
};

// Mirrors internal/coordinator/models.go's ProposalKind* constants.
export type OtherProposalKind = "message" | "move" | "resume";

// Mirrors internal/coordinator/dto.go's ProposalDTO (Build decision 10).
// claim_token is never serialized by the backend and has no field here.
// The phase-2 fields are present only while features.coordinatorPhase2 is on.
type ProposalBase = {
  id: string;
  coordinator_id: string;
  workspace_id: string;
  status: ProposalStatus;
  claimed_at: string | null;
  task_id: string | null;
  error: string | null;
  reject_reason: string | null;
  decided_by: string | null;
  created_at: string;
  updated_at: string;
  target_task_id?: string | null;
  standing_order_ids?: string[];
  starts_agent?: boolean;
  // Parsed outcome_json; null until the proposal has an outcome.
  outcome?: unknown;
  // Present only while features.coordinatorPhase3 is effective.
  reply_text?: string | null;
  reply_delivered_at?: string | null;
  in_reply_to?: string | null;
};

export type CreateTaskProposal = ProposalBase & {
  kind?: "create_task";
  spec: ProposalSpec;
  final_spec: ProposalSpec | null;
};

// A proposal of a phase-2 kind, or of a kind this client does not know (a
// newer backend). The spec is the raw stored JSON; it is never edited here.
export type OtherKindProposal = ProposalBase & {
  kind: OtherProposalKind | (string & {});
  spec: Record<string, unknown>;
  final_spec: Record<string, unknown> | null;
};

// Every proposal shape the wire can carry while phase 2 is on. Phase-1
// surfaces keep reading Proposal, which is the create_task shape.
export type WireProposal = CreateTaskProposal | OtherKindProposal;

export type Proposal = CreateTaskProposal;

// Mirrors the stored spec of each phase-2 kind (internal/coordinator/kind_*.go).
export type ResumeSpec = { task_id: string; rationale: string };
export type MessageSpec = { task_id: string; text: string; rationale: string };
export type MoveSpec = {
  task_id: string;
  workflow_id: string;
  from_step_id: string;
  to_step_id: string;
  rationale: string;
};

type KindProposalOf<K extends OtherProposalKind, S> = ProposalBase & {
  kind: K;
  spec: S;
  final_spec: S | null;
};

export type ResumeProposal = KindProposalOf<"resume", ResumeSpec>;
export type MessageProposal = KindProposalOf<"message", MessageSpec>;
export type MoveProposal = KindProposalOf<"move", MoveSpec>;
export type KindProposal = ResumeProposal | MessageProposal | MoveProposal;

// Every proposal the client caches and renders: create_task, or a phase-2 kind
// it knows. A row of any other kind is never stored.
export type StoredProposal = CreateTaskProposal | KindProposal;

export function isKindProposal(p: WireProposal): p is KindProposal {
  return p.kind === "resume" || p.kind === "message" || p.kind === "move";
}

export function isStoredProposal(p: WireProposal): p is StoredProposal {
  return isCreateTaskProposal(p) || isKindProposal(p);
}

// isCreateTaskProposal narrows to the create_task kind; phase-1 rows carry
// no kind and count as create_task.
export function isCreateTaskProposal(p: WireProposal): p is CreateTaskProposal {
  return p.kind === undefined || p.kind === "create_task";
}

export type ProposalListResponse = {
  proposals: WireProposal[];
};

// Build decision 3: omit this parameter for the default ("pending"); never
// send the empty string.
export type ProposalListStatus = "pending" | "all";

// Mirrors internal/coordinator/dto.go's StallDTO
// (docs/specs/coordinator/system-design/needs-you.md#inputs).
export type Stall = {
  task_id: string;
  stalled_for_ms: number;
  last_event_at: string;
  detected_at: string;
};

export type StallListResponse = {
  stalls: Stall[];
};

// Mirrors internal/coordinator/dto.go's ApproveProposalRequest edits (Build
// decision 16, proposals.md#edits). A field absent here is dropped by
// JSON.stringify and read as "unchanged"; the backend rejects JSON null for
// any of these fields with a 400, so this type never allows null.
export type ApproveProposalEdits = {
  title?: string;
  description?: string;
  workflow_id?: string;
  step_id?: string;
  repository_id?: string;
  // A message proposal's text; the only edit a non-create kind accepts.
  text?: string;
};

// Mirrors internal/coordinator/dto.go's ProposalConflictResponse (Build
// decision 16): the 409 body every approve or reject route returns for a
// settled or non-stale approving proposal. All three keys are always
// present.
export type ProposalConflictBody = {
  error: "proposal_conflict";
  error_code: "proposal_conflict";
  proposal: WireProposal;
};

// Mirrors internal/coordinator/events.go's CoordinatorUpdatedPayload
// (Build decision 13), the coordinator.updated WS event's payload.
export type CoordinatorUpdatedPayload = {
  workspace_id: string;
  coordinator_id: string;
  open_proposals: number;
  // Present only when the event is an autonomy re-read signal.
  autonomy_changed?: boolean;
};

// Mirrors internal/coordinator/dto.go's ConversationResponse: the
// conversation route's 200 body. archive_state is always false from this
// route (docs/specs/coordinator/system-design/copilot.md#conversation-task).
// The route's handler lands in task 03.
export type ConversationResponse = {
  task_id: string;
  session_id: string;
  archive_state: boolean;
};

// Mirrors internal/coordinator/dto.go's ConversationConflictResponse: the
// 409 body the conversation route returns for both race outcomes of
// copilot.md's conversation-route steps 4 and 7. Both keys are always
// present.
export type ConversationConflictBody = {
  error: "conversation_conflict";
  error_code: "conversation_conflict";
};

// Mirrors internal/coordinator/dto.go's CoordinatorProfileUnavailableResponse:
// the 409 body returned when either coordinator profile is not ok
// (docs/specs/coordinator/system-design/coordinators.md#validation).
export type CoordinatorProfileUnavailableResponse = {
  error: "coordinator_profile_unavailable";
  agent_profile_status: ProfileStatus;
  executor_profile_status: ProfileStatus;
};

// Mirrors internal/coordinator/standing_orders.go's Order wire shape. number
// is the 1-based active position and null for a retired order; created_by is
// the creator's user id (empty with auth off).
export type StandingOrder = {
  id: string;
  number: number | null;
  text: string;
  created_at: string;
  created_by: string;
  retired_at: string | null;
  last_applied_at: string | null;
};

export type StandingOrderListResponse = {
  orders: StandingOrder[];
};

export type AddStandingOrderRequest = {
  text: string;
  source_proposal_id?: string;
};

export type GoalCriterion = {
  id: string;
  text: string;
  done: boolean;
};

// Mirrors internal/coordinator/reads_phase2.go's Goal. due_on is a calendar
// date (YYYY-MM-DD) as stored; set_at and met_at are timestamps.
export type Goal = {
  id: string;
  coordinator_id: string;
  name: string;
  due_on: string | null;
  status: "active" | "met" | (string & {});
  criteria: GoalCriterion[];
  baseline: unknown;
  set_at: string;
  met_at: string | null;
  met_by: string | null;
  created_at: string;
  updated_at: string;
};

export type MeasureDirection = "up" | "down" | "none_small" | "none_no_baseline";

export type GoalMeasure = {
  current: number;
  baseline: number | null;
  direction: MeasureDirection;
};

export type GoalMeasures = {
  open_tasks: GoalMeasure;
  approved_7d: GoalMeasure;
  rejected_7d: GoalMeasure;
};

export type GoalResponse = {
  active: Goal | null;
  last_met: Goal | null;
  measures: GoalMeasures | null;
};

export type PutGoalRequest = {
  goal_id?: string;
  name: string;
  due_on?: string | null;
  criteria: { id?: string; text: string }[];
};

function workspacePath(workspaceId: string, suffix: string): string {
  return `/api/v1/workspaces/${encodeURIComponent(workspaceId)}${suffix}`;
}

export function coordinatorPath(workspaceId: string, coordinatorId: string, suffix = ""): string {
  return workspacePath(workspaceId, `/coordinators/${encodeURIComponent(coordinatorId)}${suffix}`);
}

function proposalPath(
  workspaceId: string,
  coordinatorId: string,
  proposalId: string,
  suffix = "",
): string {
  return coordinatorPath(
    workspaceId,
    coordinatorId,
    `/proposals/${encodeURIComponent(proposalId)}${suffix}`,
  );
}

export function listCoordinators(
  workspaceId: string,
  options?: ApiRequestOptions,
): Promise<CoordinatorListResponse> {
  return fetchJson<CoordinatorListResponse>(workspacePath(workspaceId, "/coordinators"), options);
}

export function createCoordinator(
  workspaceId: string,
  req: CreateCoordinatorRequest,
  options?: ApiRequestOptions,
): Promise<Coordinator> {
  return mutate<Coordinator>(workspacePath(workspaceId, "/coordinators"), "POST", req, options);
}

export function getCoordinator(
  workspaceId: string,
  coordinatorId: string,
  options?: ApiRequestOptions,
): Promise<Coordinator> {
  return fetchJson<Coordinator>(coordinatorPath(workspaceId, coordinatorId), options);
}

export function patchCoordinator(
  workspaceId: string,
  coordinatorId: string,
  patch: PatchCoordinatorRequest,
  options?: ApiRequestOptions,
): Promise<Coordinator> {
  return mutate<Coordinator>(coordinatorPath(workspaceId, coordinatorId), "PATCH", patch, options);
}

export function deleteCoordinator(
  workspaceId: string,
  coordinatorId: string,
  options?: ApiRequestOptions,
): Promise<void> {
  return mutate<void>(coordinatorPath(workspaceId, coordinatorId), "DELETE", undefined, options);
}

export function listProposals(
  workspaceId: string,
  coordinatorId: string,
  status?: ProposalListStatus,
  options?: ApiRequestOptions,
): Promise<ProposalListResponse> {
  const query = status ? `?status=${encodeURIComponent(status)}` : "";
  return fetchJson<ProposalListResponse>(
    coordinatorPath(workspaceId, coordinatorId, `/proposals${query}`),
    options,
  );
}

export function getProposal(
  workspaceId: string,
  coordinatorId: string,
  proposalId: string,
  options?: ApiRequestOptions,
): Promise<WireProposal> {
  return fetchJson<WireProposal>(proposalPath(workspaceId, coordinatorId, proposalId), options);
}

export function listCoordinatorStalls(
  workspaceId: string,
  options?: ApiRequestOptions,
): Promise<StallListResponse> {
  return fetchJson<StallListResponse>(workspacePath(workspaceId, "/coordinator-stalls"), options);
}

// approveProposal resolves to the approved Proposal on 200, and throws an
// ApiError on any non-2xx status. On a 409, use getProposalConflict to read
// the current row out of the thrown error (Build decision 16). The route's
// handler lands in task 07; this client function is available now so that
// work order does not also need to touch this file.
export function approveProposal(
  workspaceId: string,
  coordinatorId: string,
  proposalId: string,
  edits?: ApproveProposalEdits,
  options?: ApiRequestOptions,
): Promise<WireProposal> {
  return mutate<WireProposal>(
    proposalPath(workspaceId, coordinatorId, proposalId, "/approve"),
    "POST",
    edits,
    options,
  );
}

// rejectProposal resolves to the rejected Proposal on 200, and throws an
// ApiError on any non-2xx status (Build decision 16). The route's handler
// lands in task 07.
export function rejectProposal(
  workspaceId: string,
  coordinatorId: string,
  proposalId: string,
  reason?: string,
  options?: ApiRequestOptions,
): Promise<WireProposal> {
  return mutate<WireProposal>(
    proposalPath(workspaceId, coordinatorId, proposalId, "/reject"),
    "POST",
    reason === undefined ? {} : { reason },
    options,
  );
}

// replyToProposal resolves to the returned Proposal on 200, delivered or not,
// and throws an ApiError on any non-2xx status.
export function replyToProposal(
  workspaceId: string,
  coordinatorId: string,
  proposalId: string,
  text: string,
  options?: ApiRequestOptions,
): Promise<WireProposal> {
  return mutate<WireProposal>(
    proposalPath(workspaceId, coordinatorId, proposalId, "/reply"),
    "POST",
    { text },
    options,
  );
}

// deliverProposalReply re-runs delivery of a returned proposal's reply.
export function deliverProposalReply(
  workspaceId: string,
  coordinatorId: string,
  proposalId: string,
  options?: ApiRequestOptions,
): Promise<WireProposal> {
  return mutate<WireProposal>(
    proposalPath(workspaceId, coordinatorId, proposalId, "/reply/deliver"),
    "POST",
    {},
    options,
  );
}

// getProposalConflict returns the embedded proposal from a 409
// proposal_conflict error thrown by approveProposal or rejectProposal, or
// null for any other error (including a 409 with a different error_code,
// such as the conversation route's coordinator_profile_unavailable).
export function getProposalConflict(error: unknown): WireProposal | null {
  if (!(error instanceof ApiError) || error.status !== 409) return null;
  if (!error.body || typeof error.body !== "object") return null;
  const body = error.body as Partial<ProposalConflictBody>;
  if (
    body.error_code !== "proposal_conflict" ||
    typeof body.proposal !== "object" ||
    !body.proposal
  ) {
    return null;
  }
  return body.proposal;
}

// getCoordinatorProfileUnavailable returns the two profile statuses from a
// 409 coordinator_profile_unavailable error thrown by openConversation, or
// null for any other error. Unlike proposal_conflict, this body carries no
// error_code, so it is narrowed by its error field and shape instead of
// ApiError.errorCode.
export function getCoordinatorProfileUnavailable(
  error: unknown,
): CoordinatorProfileUnavailableResponse | null {
  if (!(error instanceof ApiError) || error.status !== 409) return null;
  if (!error.body || typeof error.body !== "object") return null;
  const body = error.body as Partial<CoordinatorProfileUnavailableResponse>;
  if (
    body.error !== "coordinator_profile_unavailable" ||
    typeof body.agent_profile_status !== "string" ||
    typeof body.executor_profile_status !== "string"
  ) {
    return null;
  }
  return {
    error: "coordinator_profile_unavailable",
    agent_profile_status: body.agent_profile_status,
    executor_profile_status: body.executor_profile_status,
  };
}

// openConversation resolves to the conversation route's 200 body, and
// throws an ApiError on any non-2xx status: a 409 conversation_conflict or
// coordinator_profile_unavailable. Only conversation_conflict carries
// error_code, so ApiError.errorCode identifies that case; distinguish
// coordinator_profile_unavailable by its error field or body shape
// (CoordinatorProfileUnavailableResponse) instead. The route's handler lands
// in task 03; this client function is available now so that work order does
// not also need to touch this file.
export function openConversation(
  workspaceId: string,
  coordinatorId: string,
  options?: ApiRequestOptions,
): Promise<ConversationResponse> {
  return mutate<ConversationResponse>(
    coordinatorPath(workspaceId, coordinatorId, "/conversation"),
    "POST",
    undefined,
    options,
  );
}

export function listStandingOrders(
  workspaceId: string,
  coordinatorId: string,
  options?: ApiRequestOptions & { includeRetired?: boolean },
): Promise<StandingOrderListResponse> {
  const { includeRetired, ...requestOptions } = options ?? {};
  const suffix = includeRetired ? "/standing-orders?include=retired" : "/standing-orders";
  return fetchJson<StandingOrderListResponse>(
    coordinatorPath(workspaceId, coordinatorId, suffix),
    requestOptions,
  );
}

export function addStandingOrder(
  workspaceId: string,
  coordinatorId: string,
  req: AddStandingOrderRequest,
  options?: ApiRequestOptions,
): Promise<StandingOrder> {
  return mutate<StandingOrder>(
    coordinatorPath(workspaceId, coordinatorId, "/standing-orders"),
    "POST",
    req,
    options,
  );
}

export function retireStandingOrder(
  workspaceId: string,
  coordinatorId: string,
  orderId: string,
  options?: ApiRequestOptions,
): Promise<StandingOrder> {
  return mutate<StandingOrder>(
    coordinatorPath(
      workspaceId,
      coordinatorId,
      `/standing-orders/${encodeURIComponent(orderId)}/retire`,
    ),
    "POST",
    undefined,
    options,
  );
}

export function restoreStandingOrder(
  workspaceId: string,
  coordinatorId: string,
  orderId: string,
  options?: ApiRequestOptions,
): Promise<StandingOrder> {
  return mutate<StandingOrder>(
    coordinatorPath(
      workspaceId,
      coordinatorId,
      `/standing-orders/${encodeURIComponent(orderId)}/restore`,
    ),
    "POST",
    undefined,
    options,
  );
}

export function getGoal(
  workspaceId: string,
  coordinatorId: string,
  options?: ApiRequestOptions,
): Promise<GoalResponse> {
  return fetchJson<GoalResponse>(coordinatorPath(workspaceId, coordinatorId, "/goal"), options);
}

export function putGoal(
  workspaceId: string,
  coordinatorId: string,
  req: PutGoalRequest,
  options?: ApiRequestOptions,
): Promise<Goal> {
  return mutate<Goal>(coordinatorPath(workspaceId, coordinatorId, "/goal"), "PUT", req, options);
}

export function setGoalCriterionDone(
  workspaceId: string,
  coordinatorId: string,
  criterionId: string,
  done: boolean,
  options?: ApiRequestOptions,
): Promise<Goal> {
  return mutate<Goal>(
    coordinatorPath(
      workspaceId,
      coordinatorId,
      `/goal/criteria/${encodeURIComponent(criterionId)}`,
    ),
    "POST",
    { done },
    options,
  );
}

export function markGoalMet(
  workspaceId: string,
  coordinatorId: string,
  goalId: string,
  options?: ApiRequestOptions,
): Promise<Goal> {
  return mutate<Goal>(
    coordinatorPath(workspaceId, coordinatorId, "/goal/met"),
    "POST",
    { goal_id: goalId },
    options,
  );
}

function mutate<T>(
  path: string,
  method: "POST" | "PATCH" | "PUT" | "DELETE",
  body: unknown,
  options?: ApiRequestOptions,
): Promise<T> {
  return fetchJson<T>(path, {
    ...options,
    init: {
      ...(options?.init ?? {}),
      method,
      ...(body === undefined ? {} : { body: JSON.stringify(body) }),
    },
  });
}

export function getCoordinatorSettings(
  workspaceId: string,
  coordinatorId: string,
  options?: ApiRequestOptions,
): Promise<CoordinatorSettings> {
  return fetchJson<CoordinatorSettings>(
    coordinatorPath(workspaceId, coordinatorId, "/settings"),
    options,
  );
}

export function putCoordinatorSettings(
  workspaceId: string,
  coordinatorId: string,
  req: PutSettingsRequest,
  options?: ApiRequestOptions,
): Promise<CoordinatorSettings> {
  return mutate<CoordinatorSettings>(
    coordinatorPath(workspaceId, coordinatorId, "/settings"),
    "PUT",
    req,
    options,
  );
}

// Mirrors internal/coordinator/setup.go's body: one request creates the
// coordinator with its policy, Watches and optional goal, or none of them.
export type SetupCoordinatorRequest = {
  name: string;
  agent_profile_id: string;
  executor_profile_id: string;
  context: string;
  watches: { scope: "all" | "selected"; workflow_ids?: string[] };
  policy: { actions: Record<ControlAction, ControlSetting> };
  goal?: {
    name: string;
    due_on: string | null;
    criteria: Array<{ text: string }>;
  };
};

export function setupCoordinator(
  workspaceId: string,
  req: SetupCoordinatorRequest,
  options?: ApiRequestOptions,
): Promise<Coordinator> {
  return mutate<Coordinator>(
    workspacePath(workspaceId, "/coordinators/setup"),
    "POST",
    req,
    options,
  );
}
