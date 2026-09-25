import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const runtimeMocks = vi.hoisted(() => ({
  queue: {
    snapshot: null,
    loading: true,
    loadFailed: false,
    saveFailed: false,
    invalidReason: undefined as string | undefined,
    isDirty: false,
  },
  session: {
    snapshot: null,
    loading: true,
    loadFailed: false,
    saveFailed: false,
    invalidReason: undefined as string | undefined,
    isDirty: false,
  },
}));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));
vi.mock("./agent-generated-task-title-settings", () => ({
  AgentGeneratedTaskTitleSettings: () => null,
}));
vi.mock("./anchored-prompt-bar-settings", () => ({ AnchoredPromptBarSettings: () => null }));
vi.mock("./archive-confirmation-settings", () => ({ ArchiveConfirmationSettings: () => null }));
vi.mock("./creation-auto-focus-settings", () => ({ CreationAutoFocusSettings: () => null }));
vi.mock("./mcp-task-agent-profile-default-settings", () => ({
  MCPTaskAgentProfileDefaultSettings: () => null,
}));
vi.mock("./prevent-auto-start-agent-settings", () => ({
  PreventAutoStartAgentSettings: () => null,
}));
vi.mock("./sleep-inhibition-settings", () => ({
  SleepInhibitionSettings: ({
    onAttentionChange,
  }: {
    onAttentionChange?: (needsAttention: boolean) => void;
  }) => (
    <button type="button" data-testid="sleep-attention" onClick={() => onAttentionChange?.(true)} />
  ),
}));
vi.mock("./todo-list-panel-settings", () => ({ TodoListPanelSettings: () => null }));
vi.mock("./unread-divider-settings", () => ({ UnreadDividerSettings: () => null }));
vi.mock("./system/message-queue-settings", () => ({
  MessageQueueSettingsContent: () => <div data-testid="message-queue" />,
  useMessageQueueSettingsDraft: () => runtimeMocks.queue,
}));
vi.mock("./system/session-capacity-settings", () => ({
  SessionCapacitySettingsContent: () => <div data-testid="session-capacity" />,
}));
vi.mock("./system/use-session-capacity-settings", () => ({
  useSessionCapacitySettings: () => runtimeMocks.session,
}));

import { TaskBehaviorSettings } from "./task-behavior-settings";

beforeEach(() => {
  runtimeMocks.queue.loadFailed = false;
  runtimeMocks.queue.saveFailed = false;
  runtimeMocks.queue.invalidReason = undefined;
  runtimeMocks.queue.isDirty = false;
  runtimeMocks.session.loadFailed = false;
  runtimeMocks.session.saveFailed = false;
  runtimeMocks.session.invalidReason = undefined;
  runtimeMocks.session.isDirty = false;
});

afterEach(cleanup);

vi.mock("@/hooks/domains/settings/use-settings-tab", async () => {
  const { useState } = await import("react");
  return {
    useSettingsTab: () => {
      const [value, selectTab] = useState("tasks");
      return { value, selectTab };
    },
  };
});
vi.mock("./settings-save-provider", async (importOriginal) => ({
  ...(await importOriginal<typeof import("./settings-save-provider")>()),
  useSettingsSaveCoordinator: () => ({ contributorStates: [] }),
}));
describe("TaskBehaviorSettings tabs", () => {
  it("starts in Tasks and reveals expanded runtime through a visible tab", () => {
    render(<TaskBehaviorSettings />);
    expect(
      screen
        .getByRole("tab", { name: "settings:taskBehaviorTabTasks" })
        .getAttribute("aria-selected"),
    ).toBe("true");
    fireEvent.keyDown(screen.getByRole("tab", { name: "settings:taskBehaviorTabRuntime" }), {
      key: "Enter",
    });
    expect(screen.getByTestId("task-behavior-runtime").querySelector("details")).toBeNull();
    expect(screen.getByTestId("task-behavior-runtime").getAttribute("aria-hidden")).not.toBe(
      "true",
    );
  });
  it("does not steal the active tab when runtime loading fails", () => {
    runtimeMocks.queue.loadFailed = true;
    render(<TaskBehaviorSettings />);
    expect(
      screen
        .getByRole("tab", { name: "settings:taskBehaviorTabTasks" })
        .getAttribute("aria-selected"),
    ).toBe("true");
  });
});
