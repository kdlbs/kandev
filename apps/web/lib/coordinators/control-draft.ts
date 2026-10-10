import type {
  ControlAction,
  ControlSetting,
  CoordinatorSettings,
  PutSettingsRequest,
} from "@/lib/api/domains/coordinator-api";

export const CONTROL_ACTIONS: readonly ControlAction[] = [
  "create_task",
  "start_agent",
  "message",
  "move",
  "resume",
  "stop",
];

export const MAX_WATCHED_BOARDS = 50;

export type WatchesDraft = { scope: "all" | "selected"; workflowIds: string[] };

export type ControlDraft = {
  actions: Record<ControlAction, ControlSetting>;
  watches: WatchesDraft;
};

export function draftFromSettings(settings: CoordinatorSettings): ControlDraft {
  return {
    actions: { ...settings.policy.actions },
    watches: { scope: settings.watches.scope, workflowIds: [...settings.watches.workflow_ids] },
  };
}

/** Both the stored and the draft `all` scope ignore ids; selected sets compare as sets. */
export function sameWatches(a: WatchesDraft, b: WatchesDraft): boolean {
  if (a.scope !== b.scope) return false;
  if (a.scope === "all") return true;
  if (a.workflowIds.length !== b.workflowIds.length) return false;
  const ids = new Set(b.workflowIds);
  return a.workflowIds.every((id) => ids.has(id));
}

export function isPolicyDirty(draft: ControlDraft, stored: ControlDraft): boolean {
  return CONTROL_ACTIONS.some((action) => draft.actions[action] !== stored.actions[action]);
}

export function isWatchesDirty(draft: ControlDraft, stored: ControlDraft): boolean {
  return !sameWatches(draft.watches, stored.watches);
}

/** A selected draft with no board that the manager edited: it cannot be saved. */
export function isWatchesInvalid(draft: ControlDraft, stored: ControlDraft): boolean {
  return (
    draft.watches.scope === "selected" &&
    draft.watches.workflowIds.length === 0 &&
    isWatchesDirty(draft, stored)
  );
}

/** The PUT body: only the members that differ from stored; the policy names all six actions. */
export function buildPutRequest(draft: ControlDraft, stored: ControlDraft): PutSettingsRequest {
  const request: PutSettingsRequest = {};
  if (isPolicyDirty(draft, stored)) request.policy = { actions: { ...draft.actions } };
  if (isWatchesDirty(draft, stored)) {
    request.watches =
      draft.watches.scope === "all"
        ? { scope: "all" }
        : { scope: "selected", workflow_ids: [...draft.watches.workflowIds] };
  }
  return request;
}

/**
 * Folds a re-read stored value into the draft without discarding an edit:
 * an action the manager changed keeps its draft value, every other action
 * follows the new stored value, and Watches is one unit.
 */
export function mergeStored(
  draft: ControlDraft,
  oldStored: ControlDraft,
  newStored: ControlDraft,
): ControlDraft {
  const actions = { ...newStored.actions };
  for (const action of CONTROL_ACTIONS) {
    if (draft.actions[action] !== oldStored.actions[action])
      actions[action] = draft.actions[action];
  }
  const watches = sameWatches(draft.watches, oldStored.watches) ? newStored.watches : draft.watches;
  return { actions, watches };
}

/** After a 200: members edited since the request was sent stay as drafted, the rest take the response. */
export function settleAfterSave(
  current: ControlDraft,
  sent: ControlDraft,
  response: ControlDraft,
): ControlDraft {
  const actions = { ...response.actions };
  for (const action of CONTROL_ACTIONS) {
    if (current.actions[action] !== sent.actions[action]) actions[action] = current.actions[action];
  }
  const watches = sameWatches(current.watches, sent.watches) ? response.watches : current.watches;
  return { actions, watches };
}

export type SwitchOffResult =
  | { ok: true; watches: WatchesDraft; capped: boolean }
  | { ok: false; reason: "no-boards" };

/** Turning "watch every board" off starts from every listed board, at most 50, in workspace order. */
export function switchOffWatches(boardIds: readonly string[]): SwitchOffResult {
  if (boardIds.length === 0) return { ok: false, reason: "no-boards" };
  return {
    ok: true,
    watches: { scope: "selected", workflowIds: boardIds.slice(0, MAX_WATCHED_BOARDS) },
    capped: boardIds.length > MAX_WATCHED_BOARDS,
  };
}
