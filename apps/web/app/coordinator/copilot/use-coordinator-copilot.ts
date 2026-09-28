import { useCallback, useEffect, useState } from "react";
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

  const [ownedRouteSession, setOwnedRouteSession] = useState<{
    coordinatorId: string;
    session: ConversationResponse;
  } | null>(null);
  const [pendingDraft, setPendingDraft] = useState<string | undefined>(undefined);
  const [askKey, setAskKey] = useState(0);

  // Guards against a coordinator switch: this hook's own state (not just the
  // `[coordinatorId]` effect below) must never hand a previous coordinator's
  // session to a render that already reflects the new `coordinatorId`.
  const routeSession =
    ownedRouteSession && ownedRouteSession.coordinatorId === coordinatorId
      ? ownedRouteSession.session
      : null;

  const launcher = useCoordinatorLauncher(
    workspaceId,
    effectiveId,
    routeSession?.session_id ?? null,
  );
  const openSequence = useCopilotOpenSequence(workspaceId, effectiveId);

  useEffect(() => {
    setOwnedRouteSession(null);
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
    if (entry.open) openSequence.open();
    // eslint-disable-next-line react-hooks/exhaustive-deps -- openSequence.open reads workspaceId/effectiveId itself; including the whole object would re-open on every state transition.
  }, [entry.open, entry.chip]);

  useEffect(() => {
    if (openSequence.state.kind !== "ready") return;
    const nextSession = openSequence.state.session;
    const priorSession =
      ownedRouteSession && ownedRouteSession.coordinatorId === coordinatorId
        ? ownedRouteSession.session
        : null;
    if (priorSession && priorSession.session_id !== nextSession.session_id) {
      // Retry (or anything else that hands this coordinator a brand new
      // session) starts clean: a seed captured for the session that just
      // ended must not resurrect an already-consumed question into its
      // replacement.
      setPendingDraft(undefined);
    }
    setOwnedRouteSession({ coordinatorId, session: nextSession });
    // eslint-disable-next-line react-hooks/exhaustive-deps -- ownedRouteSession is read for its current value only; adding it would re-run this effect on every ready-session update rather than just on an actual incoming state change.
  }, [openSequence.state, coordinatorId]);

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
