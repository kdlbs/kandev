import { useMemo } from "react";
import type { Message, TaskSessionState } from "@/lib/types/http";
import type { RenderItem } from "@/hooks/use-processed-messages";
import {
  buildActivityItems,
  deriveRunningTurn,
  deriveStatusLine,
  type ActivityStatusLine,
} from "./activity-display";

type PanelState = {
  groupedItems: RenderItem[];
  messages: Message[];
  activeTurnId?: string | null;
  session?: { state?: TaskSessionState } | null;
};

/** The transcript items and status line for the coordinator copilot. With the
 *  display off it returns the default grouping untouched. */
export function useCopilotActivity(
  enabled: boolean,
  panelState: PanelState,
): { items: RenderItem[]; statusLine: ActivityStatusLine | null } {
  const { groupedItems, messages, activeTurnId, session } = panelState;
  const sessionState = session?.state;
  return useMemo(() => {
    if (!enabled) return { items: groupedItems, statusLine: null };
    const running = deriveRunningTurn({ messages, sessionState, activeTurnId });
    return {
      items: buildActivityItems(groupedItems, running),
      statusLine: deriveStatusLine(messages, running),
    };
  }, [enabled, groupedItems, messages, sessionState, activeTurnId]);
}
