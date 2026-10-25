"use client";

import { useCallback, useMemo } from "react";
import { useOptionalAppStore } from "@/components/state-provider";
import { PluginSlot } from "@/components/plugins/plugin-slot";
import { usePluginRegistry } from "@/lib/plugins/registry";
import type { AppState } from "@/lib/state/store";
import type { TaskSession } from "@/lib/types/http";
import type { ChatSubmitDecorationSlotProps, PluginPresentation } from "@/lib/plugins/types";

const SLOT = "chat-submit-decoration";

/**
 * Re-exported for co-located consumers; the canonical declaration lives in
 * `lib/plugins/types.ts` next to the other slot contracts, and is pinned to
 * the `@kandev/plugin-sdk` export of the same name by `sdk-contract.test.ts`.
 */
export type { ChatSubmitDecorationSlotProps };

const EMPTY_SESSIONS: TaskSession[] = [];

/**
 * Plugin extension point layered over the chat composer's send button.
 *
 * The host owns the geometry: the layer is absolutely positioned to the send
 * button's box (a 28px circle) so a decoration can size itself against
 * `inset-0` without measuring anything.
 *
 * Stay within that box. The layer itself does not clip, but the desktop
 * toolbar takes `overflow-x-auto` when it collapses at narrow widths, and CSS
 * computes the other axis to `auto` alongside it — so a decoration drawn on a
 * negative inset is clipped vertically exactly there, where the toolbar has
 * only `pb-0.5` of slack. A ring belongs on the button's rim (`inset-0`), not
 * around it.
 *
 * The layer is `pointer-events-none` so a decoration can never swallow a click
 * meant for send. Keep visual disclosures inert and observe the host button's
 * hover or focus from an effect, cleaning up the listeners on unmount. The
 * plugin renders inside this layer, not beside the button, so its immediate
 * parent is not the positioned button wrapper. `pointer-events-auto` is a last
 * resort for a separate hit target that does not obstruct send.
 */
export function ChatSubmitPluginDecoration(props: {
  sessionId: string | null;
  taskId: string | null;
  taskTitle?: string;
  presentation: PluginPresentation;
  isSending: boolean;
  isAgentBusy: boolean;
  isDisabled: boolean;
  planModeEnabled: boolean;
}) {
  const {
    sessionId,
    taskId,
    taskTitle,
    presentation,
    isSending,
    isAgentBusy,
    isDisabled,
    planModeEnabled,
  } = props;
  // Gate the positioned layer on there being something to draw: an empty
  // absolute span over the send button is dead weight in every composer, and
  // the surrounding markup must stay identical to today when no plugin
  // contributes. Reactive through the registry's useSyncExternalStore.
  const registry = usePluginRegistry();
  const hasDecoration = registry.getSlotRegistrations(SLOT).length > 0;
  // itemsByTaskId holds a stable per-task array reference (updated only when
  // that task's sessions change), so selecting it avoids a new-array-per-render.
  // Read optionally: the composer always renders under a StateProvider in the
  // app, but rendering the toolbar in isolation (unit tests) must not crash.
  const selectSessions = useCallback(
    (s: AppState): TaskSession[] =>
      taskId ? (s.taskSessionsByTask.itemsByTaskId[taskId] ?? EMPTY_SESSIONS) : EMPTY_SESSIONS,
    [taskId],
  );
  const taskSessions = useOptionalAppStore(selectSessions, EMPTY_SESSIONS);

  const slotProps = useMemo<ChatSubmitDecorationSlotProps>(() => {
    const sessionIds: string[] = taskSessions.map((session) => session.id);
    // The active session may not yet be in the store list (freshly prepared);
    // make sure the plugin always receives it.
    if (sessionId && !sessionIds.includes(sessionId)) sessionIds.unshift(sessionId);
    return {
      taskId,
      taskTitle,
      activeSessionId: sessionId,
      sessionIds,
      presentation,
      isSending,
      isAgentBusy,
      disabled: isDisabled,
      planModeEnabled,
    };
    // Every dependency is a primitive pulled out of props: listing `props`
    // itself would defeat the memo, since the parent hands us a fresh object
    // on every toolbar render.
  }, [
    taskSessions,
    sessionId,
    taskId,
    taskTitle,
    presentation,
    isSending,
    isAgentBusy,
    isDisabled,
    planModeEnabled,
  ]);

  if (!hasDecoration) return null;

  return (
    <span
      data-testid="chat-submit-decoration-layer"
      className="pointer-events-none absolute inset-0 z-10"
    >
      <PluginSlot name={SLOT} slotProps={slotProps} />
    </span>
  );
}
