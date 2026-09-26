import { ApiError, fetchJson, type ApiRequestOptions } from "@/lib/api/client";

// Mirrors internal/coordinator/validate.go's ProfileStatus.
export type ProfileStatus = "ok" | "missing" | "passthrough";

// Mirrors internal/coordinator/models.go's ProposalStatus.
export type ProposalStatus = "pending" | "approving" | "approved" | "rejected" | "failed";

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
};

// Mirrors internal/coordinator/dto.go's ProposalDTO (Build decision 10).
// claim_token is never serialized by the backend and has no field here.
export type Proposal = {
  id: string;
  coordinator_id: string;
  workspace_id: string;
  status: ProposalStatus;
  spec: ProposalSpec;
  final_spec: ProposalSpec | null;
  claimed_at: string | null;
  task_id: string | null;
  error: string | null;
  reject_reason: string | null;
  decided_by: string | null;
  created_at: string;
  updated_at: string;
};

export type ProposalListResponse = {
  proposals: Proposal[];
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
};

// Mirrors internal/coordinator/dto.go's ProposalConflictResponse (Build
// decision 16): the 409 body every approve or reject route returns for a
// settled or non-stale approving proposal. All three keys are always
// present.
export type ProposalConflictBody = {
  error: "proposal_conflict";
  error_code: "proposal_conflict";
  proposal: Proposal;
};

// Mirrors internal/coordinator/events.go's CoordinatorUpdatedPayload
// (Build decision 13), the coordinator.updated WS event's payload.
export type CoordinatorUpdatedPayload = {
  workspace_id: string;
  coordinator_id: string;
  open_proposals: number;
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

function workspacePath(workspaceId: string, suffix: string): string {
  return `/api/v1/workspaces/${encodeURIComponent(workspaceId)}${suffix}`;
}

function coordinatorPath(workspaceId: string, coordinatorId: string, suffix = ""): string {
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
): Promise<Proposal> {
  return fetchJson<Proposal>(proposalPath(workspaceId, coordinatorId, proposalId), options);
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
): Promise<Proposal> {
  return mutate<Proposal>(
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
): Promise<Proposal> {
  return mutate<Proposal>(
    proposalPath(workspaceId, coordinatorId, proposalId, "/reject"),
    "POST",
    reason === undefined ? {} : { reason },
    options,
  );
}

// getProposalConflict returns the embedded proposal from a 409
// proposal_conflict error thrown by approveProposal or rejectProposal, or
// null for any other error (including a 409 with a different error_code,
// such as the conversation route's coordinator_profile_unavailable).
export function getProposalConflict(error: unknown): Proposal | null {
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

// openConversation resolves to the conversation route's 200 body, and
// throws an ApiError on any non-2xx status: a 409 conversation_conflict or
// coordinator_profile_unavailable, both distinguishable via
// ApiError.errorCode. The route's handler lands in task 03; this client
// function is available now so that work order does not also need to touch
// this file.
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

function mutate<T>(
  path: string,
  method: "POST" | "PATCH" | "DELETE",
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
