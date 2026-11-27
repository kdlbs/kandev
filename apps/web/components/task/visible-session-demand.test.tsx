import { cleanup, render } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({
  isVisible: false,
  visiblePanelIds: null as Set<string> | null,
  state: {
    tasks: { activeSessionId: "session-1", activeTaskId: "task-1" },
    taskSessions: { items: { "session-1": { task_id: "task-1" } } },
  },
  chatProps: [] as Array<{ isVisible?: boolean; detailActive?: boolean }>,
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: typeof mocks.state) => unknown) => selector(mocks.state),
}));

vi.mock("@/hooks/use-panel-active", () => ({
  usePanelActive: (panelId: string) => mocks.visiblePanelIds?.has(panelId) ?? mocks.isVisible,
}));

vi.mock("@/hooks/use-file-editors", () => ({
  useFileEditors: () => ({ openFile: vi.fn() }),
}));

vi.mock("@/components/task/task-chat-panel", () => ({
  TaskChatPanel: (props: { isVisible?: boolean; detailActive?: boolean }) => {
    mocks.chatProps.push(props);
    return <div data-testid="chat-panel" />;
  },
}));

vi.mock("@/lib/layout/panel-portal-manager", () => ({ setPanelTitle: vi.fn() }));
vi.mock("@/lib/i18n", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/i18n")>()),
  t: (key: string) => key,
}));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (key: string) => key }) }));

import { renderPanel } from "./dockview-shared";

afterEach(() => {
  cleanup();
  mocks.isVisible = false;
  mocks.visiblePanelIds = null;
  mocks.chatProps = [];
});

describe("visible Dockview session demand", () => {
  it("separates read visibility from rich detail demand and activates both with the visible tab", () => {
    const panel = () =>
      render(<>{renderPanel("session:session-1", "chat", { sessionId: "session-1" })}</>);
    const view = panel();

    expect(mocks.chatProps.at(-1)).toMatchObject({ isVisible: false, detailActive: false });

    mocks.isVisible = true;
    view.rerender(<>{renderPanel("session:session-1", "chat", { sessionId: "session-1" })}</>);
    expect(mocks.chatProps.at(-1)).toMatchObject({ isVisible: true, detailActive: true });
  });

  it("keeps both chats detail-active when two Dockview groups are visible", () => {
    mocks.visiblePanelIds = new Set(["session:split-a", "session:split-b"]);
    const renderSplit = () => (
      <>
        {renderPanel("session:split-a", "chat", { sessionId: "split-a" })}
        {renderPanel("session:split-b", "chat", { sessionId: "split-b" })}
      </>
    );
    const view = render(renderSplit());

    expect(mocks.chatProps).toHaveLength(2);
    expect(mocks.chatProps.every((props) => props.isVisible && props.detailActive)).toBe(true);

    mocks.chatProps = [];
    mocks.visiblePanelIds = new Set(["session:split-a"]);
    view.rerender(renderSplit());
    expect(mocks.chatProps).toHaveLength(2);
    expect(mocks.chatProps[0]).toMatchObject({ isVisible: true, detailActive: true });
    expect(mocks.chatProps[1]).toMatchObject({ isVisible: false, detailActive: false });
  });
});
