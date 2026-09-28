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

  const [routeSession, setRouteSession] = useState<ConversationResponse | null>(null);
  const [pendingDraft, setPendingDraft] = useState<string | undefined>(undefined);
  const [askKey, setAskKey] = useState(0);

  const launcher = useCoordinatorLauncher(
    workspaceId,
    effectiveId,
    routeSession?.session_id ?? null,
  );
  const openSequence = useCopilotOpenSequence(workspaceId, effectiveId);

  useEffect(() => {
    setRouteSession(null);
    setPendingDraft(undefined);
  }, [coordinatorId]);

  useEffect(() => {
    // `entry.chip`'s reference (not just `entry.open`) re-triggers this so a
    // second Ask about this while already open also refreshes the profile
    // statuses, per "again each time the popover opens (launcher, Ask about
    // this, or Try again)".
    if (entry.open) openSequence.open();
    // eslint-disable-next-line react-hooks/exhaustive-deps -- openSequence.open reads workspaceId/effectiveId itself; including the whole object would re-open on every state transition.
  }, [entry.open, entry.chip]);

  useEffect(() => {
    if (openSequence.state.kind === "ready") setRouteSession(openSequence.state.session);
  }, [openSequence.state]);

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
