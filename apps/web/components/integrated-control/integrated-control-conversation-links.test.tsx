import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Coordinator } from "@/lib/api/domains/coordinator-api";
import { ControlCoordinatorConversation } from "./integrated-control-conversation-links";

const fixture = vi.hoisted(() => ({
  refresh: vi.fn(() => Promise.resolve()),
  push: vi.fn(),
  open: vi.fn(),
}));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string, values?: Record<string, unknown>) =>
      values?.name ? `${key}:${String(values.name)}` : key,
  }),
}));
vi.mock("./integrated-control-provider", () => ({
  useIntegratedControl: () => ({ refresh: fixture.refresh, data: undefined }),
}));
vi.mock("@/lib/routing/client-router", () => ({
  useRouter: () => ({ push: fixture.push }),
  useSearchParams: () => new URLSearchParams(),
}));
vi.mock("@/hooks/domains/coordinator/use-copilot-open-sequence", () => ({
  runOpenSequence: fixture.open,
}));

const coordinator = (over: Partial<Coordinator> = {}) =>
  ({
    id: "c1",
    workspace_id: "w1",
    name: "Ops",
    conversation_task_id: null,
    ...over,
  }) as Coordinator;

beforeEach(() => {
  fixture.refresh.mockClear();
  fixture.push.mockClear();
  fixture.open.mockReset();
});
afterEach(cleanup);

describe("ControlCoordinatorConversation", () => {
  it("links straight to an existing conversation without opening a new one", () => {
    render(
      <ControlCoordinatorConversation
        workspaceId="w1"
        coordinator={coordinator({ conversation_task_id: "t1" })}
        named={false}
        canManage
        view="overview"
      />,
    );
    const link = screen.getByTestId("control-conversation-c1");
    expect(link.getAttribute("href")).toBe("/control/overview?workspace=w1&coordinator=c1");
    expect(link.textContent).toBe("sidebar:integratedControl.coordinatorConversation");
    expect(fixture.open).not.toHaveBeenCalled();
  });

  it("opens a missing conversation, refreshes the snapshot, then shows it", async () => {
    const order: string[] = [];
    fixture.open.mockImplementation(async () => {
      order.push("open");
      return { kind: "ready", session: { task_id: "t2", session_id: "s2", archive_state: false } };
    });
    fixture.refresh.mockImplementation(async () => {
      order.push("refresh");
    });
    fixture.push.mockImplementation(() => order.push("push"));
    const onNavigate = vi.fn();
    render(
      <ControlCoordinatorConversation
        workspaceId="w1"
        coordinator={coordinator()}
        named
        canManage
        view="activity"
        onNavigate={onNavigate}
      />,
    );
    const button = screen.getByTestId("control-open-conversation-c1");
    expect(button.textContent).toBe("sidebar:integratedControl.coordinatorConversationNamed:Ops");
    await act(async () => {
      fireEvent.click(button);
      fireEvent.click(button);
    });
    expect(fixture.open).toHaveBeenCalledTimes(1);
    expect(fixture.open).toHaveBeenCalledWith("w1", "c1");
    expect(order).toEqual(["open", "refresh", "push"]);
    expect(fixture.push).toHaveBeenCalledWith("/control/activity?workspace=w1&coordinator=c1");
    expect(onNavigate).toHaveBeenCalledTimes(1);
  });

  it.each([
    [
      { kind: "profile-unavailable", agentStatus: "missing", executorStatus: "ok" },
      ["coordinator:copilotAgentMissingMessage"],
    ],
    [
      { kind: "profile-unavailable", agentStatus: "ok", executorStatus: "missing" },
      ["coordinator:copilotExecutorMissingMessage"],
    ],
    [{ kind: "gone" }, ["coordinator:copilotGoneMessage"]],
    [{ kind: "error", error: "conflict" }, ["coordinator:copilotConflictMessage"]],
    [{ kind: "error", error: "open-failed" }, ["coordinator:copilotOpenFailedMessage"]],
  ])("stays in place and says why opening failed: %o", async (state, messages) => {
    fixture.open.mockResolvedValue(state);
    render(
      <ControlCoordinatorConversation
        workspaceId="w1"
        coordinator={coordinator()}
        named={false}
        canManage
        view="overview"
      />,
    );
    await act(async () => {
      fireEvent.click(screen.getByTestId("control-open-conversation-c1"));
    });
    expect(fixture.refresh).not.toHaveBeenCalled();
    expect(fixture.push).not.toHaveBeenCalled();
    expect(screen.getAllByRole("alert").map((item) => item.textContent)).toEqual(messages);
  });

  it("offers no open action without workspace management access", () => {
    render(
      <ControlCoordinatorConversation
        workspaceId="w1"
        coordinator={coordinator()}
        named={false}
        canManage={false}
        view="overview"
      />,
    );
    expect(screen.queryByTestId("control-open-conversation-c1")).toBeNull();
    expect(fixture.open).not.toHaveBeenCalled();
  });
});
