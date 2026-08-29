import { isQuickChatSetupSessionId } from "./quick-chat-session";
import type { QuickChatSession, QuickTerminalTab } from "./types";

/** A live Quick Chat tab and its position in the stable creation baseline. */
export type QuickChatTabDescriptor = {
  reference: string;
  baselineOrder: number;
};

export type OrderedQuickChatTabs = {
  sessions: QuickChatSession[];
  terminalTabs: QuickTerminalTab[];
  order: string[];
};

/**
 * Resolves a saved mixed conversation/terminal order against the tabs that
 * exist now. Unknown and duplicate saved references are ignored. Tabs that
 * are not in the saved order are appended in the stable baseline order.
 */
export function resolveQuickChatTabOrder(
  baseline: readonly QuickChatTabDescriptor[],
  saved: readonly string[] | undefined,
): string[] {
  const known = new Map<string, QuickChatTabDescriptor>();
  for (const descriptor of baseline) {
    if (!known.has(descriptor.reference)) known.set(descriptor.reference, descriptor);
  }

  const ordered: string[] = [];
  const seen = new Set<string>();
  for (const reference of saved ?? []) {
    if (!known.has(reference) || seen.has(reference)) continue;
    seen.add(reference);
    ordered.push(reference);
  }

  const baselineOrder = [...known.values()].sort(
    (left, right) =>
      left.baselineOrder - right.baselineOrder || left.reference.localeCompare(right.reference),
  );
  for (const descriptor of baselineOrder) {
    if (seen.has(descriptor.reference)) continue;
    seen.add(descriptor.reference);
    ordered.push(descriptor.reference);
  }

  return ordered;
}

/** Resolves live Quick Chat tabs into one mixed conversation/terminal list. */
export function orderQuickChatTabs(
  sessions: QuickChatSession[],
  terminalTabs: QuickTerminalTab[],
  saved: string[] | undefined,
): OrderedQuickChatTabs {
  const reorderableSessions = sessions.filter(
    (session) => !isQuickChatSetupSessionId(session.sessionId),
  );
  const setupSessions = sessions.filter((session) => isQuickChatSetupSessionId(session.sessionId));
  const descriptors: QuickChatTabDescriptor[] = [
    ...reorderableSessions.map((session, index) => ({
      reference: `conversation:${session.sessionId}`,
      baselineOrder: index,
    })),
    ...terminalTabs.map((tab, index) => ({
      reference: `terminal:${tab.tabId}`,
      baselineOrder: reorderableSessions.length + index,
    })),
  ];
  const order = resolveQuickChatTabOrder(descriptors, saved);
  const sessionByReference = new Map(
    reorderableSessions.map((session) => [`conversation:${session.sessionId}`, session]),
  );
  const terminalByReference = new Map(terminalTabs.map((tab) => [`terminal:${tab.tabId}`, tab]));

  return {
    order,
    sessions: [
      ...order.flatMap((reference) => {
        const session = sessionByReference.get(reference);
        return session ? [session] : [];
      }),
      ...setupSessions,
    ],
    terminalTabs: order.flatMap((reference) => {
      const tab = terminalByReference.get(reference);
      return tab ? [tab] : [];
    }),
  };
}
