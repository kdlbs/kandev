import type { QueueGroupKind } from "@/lib/coordinator/attention";

/** `/workspaces/:id/coordinator`, the generic route (redirects, or the no-coordinator state). */
export function linkToCoordinator(workspaceId: string): string {
  return `/workspaces/${encodeURIComponent(workspaceId)}/coordinator`;
}

/** `/workspaces/:id/coordinator/:coordinatorId`, the coordinator's Needs you screen. */
export function linkToCoordinatorNeedsYou(workspaceId: string, coordinatorId: string): string {
  return `/workspaces/${encodeURIComponent(workspaceId)}/coordinator/${encodeURIComponent(coordinatorId)}`;
}

/**
 * `/workspaces/:id/coordinator/:coordinatorId/queue`, optionally with a
 * `group` query param the Queue screen uses to scroll to and expand that
 * group (AC-COORDINATOR-NEEDS-YOU-003.2).
 */
export function linkToCoordinatorQueue(
  workspaceId: string,
  coordinatorId: string,
  group?: QueueGroupKind,
): string {
  const base = `${linkToCoordinatorNeedsYou(workspaceId, coordinatorId)}/queue`;
  return group ? `${base}?group=${group}` : base;
}

/** The Queue's What it did section filtered to one action class (`?class=`). */
export function linkToCoordinatorActivityClass(
  workspaceId: string,
  coordinatorId: string,
  activityClass: string,
): string {
  return `${linkToCoordinatorNeedsYou(workspaceId, coordinatorId)}/queue?class=${encodeURIComponent(activityClass)}`;
}

/**
 * The Needs-you screen with `?proposal=<id>&form=edit|reject`
 * (proposal-cards.md#cards "Forms and navigation"), used by the chat card's
 * Edit/Reject to deep-link into the same card's inline form there.
 */
export function linkToCoordinatorNeedsYouForm(
  workspaceId: string,
  coordinatorId: string,
  proposalId: string,
  form: "edit" | "reject",
): string {
  const base = linkToCoordinatorNeedsYou(workspaceId, coordinatorId);
  return `${base}?proposal=${encodeURIComponent(proposalId)}&form=${form}`;
}

/** `/settings/workspaces/:id/coordinators`, the workspace's coordinator list in settings. */
export function linkToCoordinatorSettingsList(workspaceId: string): string {
  return `/settings/workspaces/${encodeURIComponent(workspaceId)}/coordinators`;
}

/** `/settings/workspaces/:id/coordinators/:coordinatorId`, one coordinator's settings page. */
export function linkToCoordinatorSettings(workspaceId: string, coordinatorId: string): string {
  return `${linkToCoordinatorSettingsList(workspaceId)}/${encodeURIComponent(coordinatorId)}`;
}

/** `/settings/workspaces/:id/coordinators/new`, the add-coordinator page. */
export function linkToCoordinatorAdd(workspaceId: string): string {
  return `${linkToCoordinatorSettingsList(workspaceId)}/new`;
}
