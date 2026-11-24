import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { StateProvider, useAppStore } from "@/components/state-provider";
import type { QuickChatSession } from "@/lib/state/slices/ui/types";
import { InboxOpenTaskLink } from "./inbox-open-task-link";

const INBOX_PATH = "/needs-you-inbox?tab=history";
const TASK_PATH = "/t/task-1?sessionId=session-1";
const CONVERSATION: QuickChatSession = {
  kind: "chat",
  taskId: "task-1",
  sessionId: "session-1",
  workspaceId: "workspace-1",
};

function Selection() {
  const active = useAppStore((state) => state.quickChat.isOpen && state.quickChat.activeSessionId);
  return <output data-testid="selection">{active}</output>;
}

function renderAction(sessions: QuickChatSession[], target?: string) {
  render(
    <StateProvider
      initialState={{
        workspaces: { items: [], activeId: CONVERSATION.workspaceId },
        quickChat: { sessions },
      }}
    >
      <InboxOpenTaskLink taskId="task-1" sessionId="session-1" target={target}>
        Open task
      </InboxOpenTaskLink>
      <Selection />
    </StateProvider>,
  );
  return screen.getByRole("link", { name: "Open task" });
}

beforeEach(() => {
  window.history.replaceState({}, "", INBOX_PATH);
  vi.spyOn(window, "scrollTo").mockImplementation(() => {});
});

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

// @covers AC-UI-INBOX-OPEN-TASK-001.1
it("opens the exact session and keeps the Inbox tab in the URL", () => {
  const link = renderAction([{ ...CONVERSATION, sessionId: "other-session" }, CONVERSATION]);
  fireEvent.click(link);
  expect(window.location.pathname + window.location.search).toBe(INBOX_PATH);
  expect(screen.getByTestId("selection").textContent).toBe("session-1");
});

// @covers AC-UI-INBOX-OPEN-TASK-001.2
it.each([
  ["no match", []],
  ["a different session", [{ ...CONVERSATION, sessionId: "other-session" }]],
  ["a different workspace", [{ ...CONVERSATION, workspaceId: "other-workspace" }]],
  ["a configuration chat", [{ ...CONVERSATION, kind: "config" as const }]],
])("uses the task link with %s", (_label, sessions) => {
  fireEvent.click(renderAction(sessions));
  expect(window.location.pathname + window.location.search).toBe(TASK_PATH);
  expect(screen.getByTestId("selection").textContent).toBe("");
});

it.each([
  ["modified click", { ctrlKey: true }, undefined],
  ["new tab target", {}, "_blank"],
] as const)("leaves %s to the browser", (_label, event, target) => {
  const link = renderAction([CONVERSATION], target);
  expect(fireEvent.click(link, event)).toBe(true);
  expect(link.getAttribute("href")).toBe(TASK_PATH);
  expect(screen.getByTestId("selection").textContent).toBe("");
});
