import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { TaskFormInputsHandle } from "@/components/task-create-dialog-types";

const mockToast = vi.fn();
const mockSummarize = vi.fn();
const mockBuildStartRequest = vi.fn();
const mockLaunchSession = vi.fn();
const mockSetActiveSession = vi.fn();

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
  setActiveSession: mockSetActiveSession,
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

vi.mock("@/lib/services/session-launch-helpers", () => ({
  buildStartRequest: (...args: unknown[]) => mockBuildStartRequest(...args),
}));

vi.mock("@/lib/services/session-launch-service", () => ({
  launchSession: (...args: unknown[]) => mockLaunchSession(...args),
}));

vi.mock("@/components/task-create-dialog-selectors", async () => {
  const React = await vi.importActual<typeof import("react")>("react");
  function TaskFormInputs({
    descriptionValueRef,
    initialDescription,
    onDescriptionChange,
    onVoiceAutoSend,
  }: {
    descriptionValueRef: React.RefObject<TaskFormInputsHandle | null>;
    initialDescription: string;
    onDescriptionChange: (hasContent: boolean) => void;
    onVoiceAutoSend?: () => void;
  }) {
    const valueRef = React.useRef(initialDescription);
    const [value, setValue] = React.useState(initialDescription);
    const updateValue = React.useCallback(
      (next: string) => {
        valueRef.current = next;
        setValue(next);
        onDescriptionChange(next.trim().length > 0);
      },
      [onDescriptionChange],
    );
    React.useEffect(() => {
      descriptionValueRef.current = {
        getValue: () => valueRef.current,
        setValue: updateValue,
        getAttachments: () => [],
      };
    }, [descriptionValueRef, updateValue]);
    return React.createElement(
      React.Fragment,
      null,
      React.createElement("textarea", {
        "data-testid": "task-description-input",
        placeholder: "Describe what you want the agent to do... (@ to insert a saved prompt)",
        value,
        onChange: (event: React.ChangeEvent<HTMLTextAreaElement>) =>
          updateValue(event.target.value),
      }),
      React.createElement(
        "button",
        {
          type: "button",
          "aria-label": "Voice input",
          onClick: () => {
            updateValue("voice transcript");
            onVoiceAutoSend?.();
          },
        },
        "Voice",
      ),
    );
  }
  return { AgentSelector: () => null, TaskFormInputs };
});

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
    mockBuildStartRequest.mockReturnValue({ request: { task_id: "task-1" } });
    mockLaunchSession.mockResolvedValue({ session_id: "session-2" });
  });

  it("copies the initial prompt on the first copy_prompt action after opening", async () => {
    render(<NewSessionDialog open={true} onOpenChange={vi.fn()} taskId="task-1" />);

    fireEvent.click(screen.getByRole("button", { name: "Copy initial prompt" }));

    await waitFor(() =>
      expect((screen.getByTestId("task-description-input") as HTMLTextAreaElement).value).toBe(
        "seed prompt",
      ),
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
      expect((screen.getByTestId("task-description-input") as HTMLTextAreaElement).value).toBe(
        "summary text",
      ),
    );
  });

  it("auto-sends a transcript inserted into a blank composer", async () => {
    render(<NewSessionDialog open={true} onOpenChange={vi.fn()} taskId="task-1" />);

    fireEvent.click(screen.getByRole("button", { name: "Voice input" }));

    await waitFor(() => expect(mockLaunchSession).toHaveBeenCalledTimes(1));
    expect(mockBuildStartRequest).toHaveBeenCalledWith(
      "task-1",
      "profile-1",
      expect.objectContaining({ prompt: "voice transcript" }),
    );
  });
});
