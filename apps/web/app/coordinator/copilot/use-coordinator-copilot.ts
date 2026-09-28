import { useCallback, useEffect, useRef, useState } from "react";
import { useFeature } from "@/hooks/domains/features/use-feature";
import {
  useCopilotEntry,
  useCopilotStore,
  type CopilotChip,
} from "@/hooks/domains/coordinator/copilot-store";
import {
  useCoordinatorLauncher,
  type CoordinatorLauncherState,
} from "@/hooks/domains/coordinator/use-coordinator-launcher";
import {
  useCopilotOpenSequence,
  type UseCopilotOpenSequenceResult,
} from "@/hooks/domains/coordinator/use-copilot-open-sequence";
import type { ConversationResponse } from "@/lib/api/domains/coordinator-api";

export type UseCoordinatorCopilotResult = {
  /** `features.coordinator` and `workspace.manage` together; the caller
   *  renders nothing else when this is false. */
  enabled: boolean;
  open: boolean;
  launcher: CoordinatorLauncherState;
  openSequence: UseCopilotOpenSequenceResult;
  routeSession: ConversationResponse | null;
  chip: CopilotChip | null;
  /** The one-shot composer seed for the current `askKey`; `undefined` once
   *  nothing is pending. */
  pendingDraft: string | undefined;
  /** Bumped on every event that must force the composer to re-apply
   *  `pendingDraft` (including an identical repeat), so the caller can key
   *  `QuickChatSessionView` on it. */
  askKey: number;
  handleOpenChange: (open: boolean) => void;
  removeChip: () => void;
  suggest: (text: string) => void;
};

/**
 * Tracks the ready session handed back by the open sequence, scoped to
 * `coordinatorId`, and distinguishes a stale replacement (Retry, or
 * anything else that swaps sessions without this hook itself having just
 * reopened) from a replacement produced by this hook's own chip/open-driven
 * reopen. `openSequence.retry` (wired directly to the popover's retry
 * action) never touches `entry.open`/`entry.chip`, so only the latter can
 * legitimately hand back a different `session_id` for a draft that was only
 * just seeded for this same open (a terminal session replaced by the very
 * "Ask about this" that reopened it) — that case must keep the draft.
 */
function useReadySessionForCoordinator(
  state: UseCopilotOpenSequenceResult["state"],
  coordinatorId: string,
  onStaleReplacement: () => void,
) {
  const [ownedRouteSession, setOwnedRouteSession] = useState<{
    coordinatorId: string;
    session: ConversationResponse;
  } | null>(null);
  const selfInitiatedOpenRef = useRef(false);

  useEffect(() => {
    setOwnedRouteSession(null);
    selfInitiatedOpenRef.current = false;
  }, [coordinatorId]);

  useEffect(() => {
    if (state.kind !== "ready") return;
    const nextSession = state.session;
    const priorSession =
      ownedRouteSession && ownedRouteSession.coordinatorId === coordinatorId
        ? ownedRouteSession.session
        : null;
    const resolvedFromSelfInitiatedOpen = selfInitiatedOpenRef.current;
    selfInitiatedOpenRef.current = false;
    if (
      priorSession &&
      priorSession.session_id !== nextSession.session_id &&
      !resolvedFromSelfInitiatedOpen
    ) {
      onStaleReplacement();
    }
    setOwnedRouteSession({ coordinatorId, session: nextSession });
    // eslint-disable-next-line react-hooks/exhaustive-deps -- ownedRouteSession is read for its current value only; adding it would re-run this effect on every ready-session update rather than just on an actual incoming state change.
  }, [state, coordinatorId]);

  // Guards against a coordinator switch: this hook's own state (not just the
  // `[coordinatorId]` effect above) must never hand a previous coordinator's
  // session to a render that already reflects the new `coordinatorId`.
  const routeSession =
    ownedRouteSession && ownedRouteSession.coordinatorId === coordinatorId
      ? ownedRouteSession.session
      : null;

  return {
    routeSession,
    markSelfInitiatedOpen: () => {
      selfInitiatedOpenRef.current = true;
    },
  };
}

/**
 * Composes the coordinator launcher, coordinator read, open sequence and the
 * copilot store's per-coordinator entry into what `CoordinatorCopilot` needs
 * to render (docs/specs/coordinator/system-design/copilot-popover.md).
 */
export function useCoordinatorCopilot(
  workspaceId: string,
  coordinatorId: string,
  canManage: boolean,
): UseCoordinatorCopilotResult {
  const featureOn = useFeature("coordinator");
  const enabled = featureOn && canManage;
  const effectiveId = enabled ? coordinatorId : null;

  const entry = useCopilotEntry(coordinatorId);
  const setOpen = useCopilotStore((s) => s.setOpen);
  const clearDraft = useCopilotStore((s) => s.clearDraft);
  const removeChipAction = useCopilotStore((s) => s.removeChip);
  const removeEntry = useCopilotStore((s) => s.removeEntry);
  const clearChipAndDraft = useCopilotStore((s) => s.clearChipAndDraft);

  const [pendingDraft, setPendingDraft] = useState<string | undefined>(undefined);
  const [askKey, setAskKey] = useState(0);

  const openSequence = useCopilotOpenSequence(workspaceId, effectiveId);
  const { routeSession, markSelfInitiatedOpen } = useReadySessionForCoordinator(
    openSequence.state,
    coordinatorId,
    () => setPendingDraft(undefined),
  );

  const launcher = useCoordinatorLauncher(
    workspaceId,
    effectiveId,
    routeSession?.session_id ?? null,
  );

  useEffect(() => {
    setPendingDraft(undefined);
  }, [coordinatorId]);

  useEffect(() => {
    // `entry.chip`'s reference (not just `entry.open`) re-triggers this so a
    // second Ask about this while already open also refreshes the profile
    // statuses, per "again each time the popover opens (launcher, Ask about
    // this, or Try again)". The mount site keys this whole controller on
    // `coordinatorId`, so a coordinator switch always remounts this hook
    // fresh; this effect's own initial-mount run is what opens the
    // newly-viewed coordinator, not a `coordinatorId` dependency here.
    if (entry.open) {
      markSelfInitiatedOpen();
      openSequence.open();
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps -- openSequence.open reads workspaceId/effectiveId itself; including the whole object would re-open on every state transition.
  }, [entry.open, entry.chip]);

  useEffect(() => {
    if (openSequence.state.kind === "gone") clearChipAndDraft(coordinatorId);
  }, [openSequence.state.kind, coordinatorId, clearChipAndDraft]);

  useEffect(() => {
    if (launcher.gone && !entry.open) removeEntry(coordinatorId);
  }, [launcher.gone, entry.open, coordinatorId, removeEntry]);

  useEffect(() => {
    if (!entry.chip) return;
    setPendingDraft(entry.draft || undefined);
    setAskKey((k) => k + 1);
    if (entry.draft) clearDraft(coordinatorId);
    // eslint-disable-next-line react-hooks/exhaustive-deps -- entry.chip's identity is the "new ask" signal; entry.draft is read once per new chip, not tracked as its own trigger.
  }, [entry.chip]);

  const handleOpenChange = useCallback(
    (open: boolean) => {
      if (open) {
        setOpen(coordinatorId, true);
        return;
      }
      // The popover fully unmounts on close (no `forceMount`), so a consumed
      // one-shot seed must not survive to reapply itself into the composer
      // on the next mount; unsent typed text is the composer's own concern
      // (its per-session draft storage), not this seed.
      setPendingDraft(undefined);
      if (openSequence.state.kind === "gone") removeEntry(coordinatorId);
      else setOpen(coordinatorId, false);
    },
    [coordinatorId, openSequence.state.kind, setOpen, removeEntry],
  );

  const removeChip = useCallback(
    () => removeChipAction(coordinatorId),
    [removeChipAction, coordinatorId],
  );

  const suggest = useCallback((text: string) => {
    setPendingDraft(text || undefined);
    setAskKey((k) => k + 1);
  }, []);

  return {
    enabled,
    open: entry.open,
    launcher,
    openSequence,
    routeSession,
    chip: entry.chip,
    pendingDraft,
    askKey,
    handleOpenChange,
    removeChip,
    suggest,
  };
}
