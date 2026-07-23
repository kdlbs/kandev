import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const mockToast = vi.fn();
const mockSummarize = vi.fn();

const mockState = {
  kanban: {
    tasks: [{ id: "task-1", title: "Task title" }],
  },
  agentProfiles: {
    items: [
      {
        id: "profile-1",
        label: "Profile 1",
        agent_name: "agent-1",
        agent_id: "agent-id-1",
        cli_passthrough: false,
      },
    ],
  },
  tasks: {
    activeSessionId: "session-1",
  },
  taskSessions: {
    items: {
      "session-1": {
        id: "session-1",
        agent_profile_id: "profile-1",
        executor_id: "executor-1",
      },
    },
  },
  sessionWorktreesBySessionId: {
    itemsBySessionId: {},
  },
  worktrees: {
    items: {},
  },
  messages: {
    bySession: {
      "session-1": [{ id: "message-1", author_type: "user", content: "seed prompt" }],
    },
  },
  executors: {
    items: [{ id: "executor-1", name: "Executor 1" }],
  },
};

vi.mock("@kandev/ui/dialog", () => ({
  Dialog: ({ open, children }: { open: boolean; children: React.ReactNode }) =>
    open ? <div>{children}</div> : null,
  DialogContent: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  DialogHeader: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  DialogTitle: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  DialogFooter: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: typeof mockState) => unknown) => selector(mockState),
}));

vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: mockToast }),
}));

vi.mock("@/lib/state/dockview-store", () => ({
  useDockviewStore: {
    getState: () => ({ api: null, centerGroupId: "center-group" }),
  },
}));

vi.mock("@/lib/state/dockview-panel-actions", () => ({
  addSessionPanel: vi.fn(),
}));

vi.mock("@/components/task-create-dialog-selectors", () => ({
  AgentSelector: () => null,
}));

vi.mock("@/components/task-create-dialog-options", () => ({
  useAgentProfileOptions: (profiles: Array<{ id: string; label: string }>) =>
    profiles.map((profile) => ({ value: profile.id, label: profile.label })),
}));

vi.mock("@/hooks/use-summarize-session", () => ({
  useSummarizeSession: () => ({
    summarize: mockSummarize,
    isSummarizing: false,
  }),
}));

vi.mock("@/hooks/use-task-sessions", () => ({
  useTaskSessions: () => ({
    sessions: [],
    loadSessions: vi.fn(),
  }),
}));

vi.mock("@/hooks/domains/settings/use-remote-auth-specs", () => ({
  useRemoteAuthSpecs: () => ({
    specs: [],
    loaded: true,
  }),
}));

vi.mock("@/hooks/domains/session/use-task-executor-profile", () => ({
  useTaskExecutorProfile: () => null,
}));

vi.mock("@/hooks/use-is-utility-configured", () => ({
  useIsUtilityConfigured: () => true,
}));

vi.mock("@/hooks/use-utility-agent-generator", () => ({
  useUtilityAgentGenerator: () => ({
    enhancePrompt: vi.fn(),
    isEnhancingPrompt: false,
  }),
}));

vi.mock("@/components/enhance-prompt-button", () => ({
  EnhancePromptButton: () => null,
}));

vi.mock("./session-dialog-shared", () => ({
  EnvironmentBadges: () => null,
  AttachButton: () => null,
  ContextSelect: ({ onValueChange }: { onValueChange: (value: string) => void }) => (
    <div>
      <button type="button" onClick={() => void onValueChange("copy_prompt")}>
        Copy initial prompt
      </button>
    </div>
  ),
  useDialogAttachments: () => ({
    attachments: [],
    isDragging: false,
    fileInputRef: { current: null },
    handleRemoveAttachment: vi.fn(),
    handlePaste: vi.fn(),
    handleDragOver: vi.fn(),
    handleDragLeave: vi.fn(),
    handleDrop: vi.fn(),
    handleAttachClick: vi.fn(),
    handleFileInputChange: vi.fn(),
  }),
  toContextItems: () => [],
}));

import { NewSessionDialog } from "./new-session-dialog";

describe("NewSessionDialog", () => {
  afterEach(() => {
    cleanup();
  });

  beforeEach(() => {
    vi.clearAllMocks();
    mockSummarize.mockResolvedValue({ summary: "summary text" });
  });

  it("copies the initial prompt on the first copy_prompt action after opening", async () => {
    render(<NewSessionDialog open={true} onOpenChange={vi.fn()} taskId="task-1" />);

    fireEvent.click(screen.getByRole("button", { name: "Copy initial prompt" }));

    await waitFor(() =>
      expect(
        (screen.getByPlaceholderText("What should the agent work on?") as HTMLTextAreaElement)
          .value,
      ).toBe("seed prompt"),
    );
  });

  it("writes the handoff summary into the fresh-open dialog prompt", async () => {
    render(
      <NewSessionDialog
        open={true}
        onOpenChange={vi.fn()}
        taskId="task-1"
        handoff={{ sourceSessionId: "session-9", targetProfileId: "profile-1" }}
      />,
    );

    await waitFor(() => expect(mockSummarize).toHaveBeenCalledWith("session-9"));
    await waitFor(() =>
      expect(
        (screen.getByPlaceholderText("What should the agent work on?") as HTMLTextAreaElement)
          .value,
      ).toBe("summary text"),
    );
  });
});
