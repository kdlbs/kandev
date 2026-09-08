import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { TaskFormInputsHandle } from "@/components/task-create-dialog-types";
import type { ExecutorProfile } from "@/lib/types/http";
import type { AgentProfileOption } from "@/lib/state/slices";

const mockToast = vi.fn();
const mockSummarize = vi.fn();
const mockBuildStartRequest = vi.fn();
const mockLaunchSession = vi.fn();
const mockSetActiveSession = vi.fn();
const mockApplyAgentProfileRecentUse = vi.fn();
const mockRecordRecentUse = vi.fn();
let mockAgentSelectorValue: string | undefined;
let mockAgentSelectorOnChange: ((value: string) => void) | undefined;
let mockExecutorProfile: ExecutorProfile | null = null;
const PLUGIN_COMPOSER_LABEL = "Plugin composer action";

const BASE_PROFILE: AgentProfileOption = {
  id: "profile-1",
  label: "Profile 1",
  agent_name: "agent-1",
  agent_id: "agent-id-1",
  cli_passthrough: false,
  enabled: true,
};

const mockState = {
  features: { dynamicAgentRouting: true },
  kanban: {
    workflowId: null,
    tasks: [{ id: "task-1", title: "Task title" }],
  },
  kanbanMulti: { snapshots: {} },
  quickChat: { sessions: [] },
  workflows: { items: [] },
  agentProfiles: {
    items: [BASE_PROFILE],
  },
  agentProfileRecentUse: {
    loaded: true,
    records: {},
  },
  tasks: {
    activeSessionId: "session-1",
  },
  setActiveSession: mockSetActiveSession,
  applyAgentProfileRecentUse: mockApplyAgentProfileRecentUse,
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

vi.mock("@/lib/agent-profile-recent-use", () => ({
  recordAgentProfileRecentUseBestEffort: (...args: unknown[]) => mockRecordRecentUse(...args),
}));

vi.mock("@/components/task-create-dialog-selectors", async () => {
  const React = await vi.importActual<typeof import("react")>("react");
  function TaskFormInputs({
    descriptionValueRef,
    initialDescription,
    onDescriptionChange,
    onComposerSubmit,
  }: {
    descriptionValueRef: React.RefObject<TaskFormInputsHandle | null>;
    initialDescription: string;
    onDescriptionChange: (hasContent: boolean) => void;
    onComposerSubmit?: () => void;
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
          // Stands in for a plugin composer action: insert text, then ask the
          // dialog to submit the native way.
          "aria-label": PLUGIN_COMPOSER_LABEL,
          onClick: () => {
            updateValue("dictated prompt");
            onComposerSubmit?.();
          },
        },
        "Plugin action",
      ),
    );
  }
  return {
    AgentSelector: (props: { value?: string; onValueChange?: (value: string) => void }) => {
      mockAgentSelectorValue = props.value;
      mockAgentSelectorOnChange = props.onValueChange;
      return null;
    },
    TaskFormInputs,
  };
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
  useTaskExecutorProfile: () => mockExecutorProfile,
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

// eslint-disable-next-line max-lines-per-function
describe("NewSessionDialog", () => {
  afterEach(() => {
    cleanup();
    mockState.agentProfiles.items = [BASE_PROFILE];
    mockState.agentProfileRecentUse = { loaded: true, records: {} };
    mockExecutorProfile = null;
    mockAgentSelectorValue = undefined;
    mockAgentSelectorOnChange = undefined;
  });

  beforeEach(() => {
    vi.clearAllMocks();
    mockSummarize.mockResolvedValue({ summary: "summary text" });
    mockBuildStartRequest.mockReturnValue({ request: { task_id: "task-1" } });
    mockLaunchSession.mockResolvedValue({ session_id: "session-2" });
    mockRecordRecentUse.mockReset();
    mockApplyAgentProfileRecentUse.mockReset();
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

  it("submits text a plugin composer action inserted into a blank composer", async () => {
    render(<NewSessionDialog open={true} onOpenChange={vi.fn()} taskId="task-1" />);

    fireEvent.click(screen.getByRole("button", { name: PLUGIN_COMPOSER_LABEL }));

    await waitFor(() => expect(mockLaunchSession).toHaveBeenCalledTimes(1));
    expect(mockBuildStartRequest).toHaveBeenCalledWith(
      "task-1",
      "profile-1",
      expect.objectContaining({ prompt: "dictated prompt" }),
    );
  });

  it("does not mark the initialized profile explicit until the picker changes", async () => {
    render(<NewSessionDialog open={true} onOpenChange={vi.fn()} taskId="task-1" />);

    fireEvent.click(screen.getByRole("button", { name: PLUGIN_COMPOSER_LABEL }));

    await waitFor(() =>
      expect(mockBuildStartRequest).toHaveBeenCalledWith(
        "task-1",
        "profile-1",
        expect.objectContaining({ profileExplicit: false }),
      ),
    );
  });

  it("uses the recent task-session profile as an explicit default", async () => {
    const recentProfile = { ...BASE_PROFILE, id: "profile-2", label: "Profile 2" };
    mockState.agentProfiles.items = [BASE_PROFILE, recentProfile];
    mockState.agentProfileRecentUse = {
      loaded: true,
      records: {
        task_session: {
          profileIds: [recentProfile.id],
          revision: 1,
          updatedAt: "2026-08-30T12:00:00Z",
        },
      },
    };

    render(<NewSessionDialog open={true} onOpenChange={vi.fn()} taskId="task-1" />);

    await waitFor(() => expect(mockAgentSelectorValue).toBe(recentProfile.id));
    fireEvent.click(screen.getByRole("button", { name: PLUGIN_COMPOSER_LABEL }));

    await waitFor(() =>
      expect(mockBuildStartRequest).toHaveBeenCalledWith(
        "task-1",
        recentProfile.id,
        expect.objectContaining({ profileExplicit: true }),
      ),
    );
  });

  it("does not replace a manual profile choice when recent use changes", async () => {
    const recentProfile = { ...BASE_PROFILE, id: "profile-2", label: "Profile 2" };
    const laterRecentProfile = { ...BASE_PROFILE, id: "profile-3", label: "Profile 3" };
    mockState.agentProfiles.items = [BASE_PROFILE, recentProfile, laterRecentProfile];
    mockState.agentProfileRecentUse = {
      loaded: true,
      records: {
        task_session: {
          profileIds: [recentProfile.id],
          revision: 1,
          updatedAt: "2026-08-30T12:00:00Z",
        },
      },
    };
    const { rerender } = render(
      <NewSessionDialog open={true} onOpenChange={vi.fn()} taskId="task-1" />,
    );

    await waitFor(() => expect(mockAgentSelectorValue).toBe(recentProfile.id));
    await act(async () => {
      mockAgentSelectorOnChange?.(BASE_PROFILE.id);
    });

    mockState.agentProfileRecentUse = {
      loaded: true,
      records: {
        task_session: {
          profileIds: [laterRecentProfile.id],
          revision: 2,
          updatedAt: "2026-08-30T12:01:00Z",
        },
      },
    };
    rerender(<NewSessionDialog open={true} onOpenChange={vi.fn()} taskId="task-1" />);

    await waitFor(() => expect(mockAgentSelectorValue).toBe(BASE_PROFILE.id));
  });

  it("marks a changed picker profile explicit", async () => {
    mockState.agentProfiles.items = [
      BASE_PROFILE,
      { ...BASE_PROFILE, id: "profile-2", label: "Profile 2" },
    ];
    render(<NewSessionDialog open={true} onOpenChange={vi.fn()} taskId="task-1" />);

    await act(async () => {
      mockAgentSelectorOnChange?.("profile-2");
    });
    fireEvent.click(screen.getByRole("button", { name: PLUGIN_COMPOSER_LABEL }));

    await waitFor(() =>
      expect(mockBuildStartRequest).toHaveBeenCalledWith(
        "task-1",
        "profile-2",
        expect.objectContaining({ profileExplicit: true }),
      ),
    );
  });

  it("defaults to the first enabled profile when the session profile is disabled", () => {
    mockState.agentProfiles.items = [
      { ...BASE_PROFILE, enabled: false },
      { ...BASE_PROFILE, id: "profile-2", label: "Profile 2" },
    ];
    render(<NewSessionDialog open={true} onOpenChange={vi.fn()} taskId="task-1" />);
    expect(mockAgentSelectorValue).toBe("profile-2");
  });

  it("reconciles the selection when the active profile becomes disabled", async () => {
    mockState.agentProfiles.items = [
      BASE_PROFILE,
      { ...BASE_PROFILE, id: "profile-2", label: "Profile 2" },
    ];
    const { rerender } = render(
      <NewSessionDialog open={true} onOpenChange={vi.fn()} taskId="task-1" />,
    );
    expect(mockAgentSelectorValue).toBe("profile-1");

    mockState.agentProfiles.items = [
      { ...BASE_PROFILE, enabled: false },
      { ...BASE_PROFILE, id: "profile-2", label: "Profile 2" },
    ];
    rerender(<NewSessionDialog open={true} onOpenChange={vi.fn()} taskId="task-1" />);

    await waitFor(() => expect(mockAgentSelectorValue).toBe("profile-2"));
  });

  it("resets a manual profile choice when it becomes incompatible", async () => {
    const manualProfile = { ...BASE_PROFILE, id: "profile-2", label: "Profile 2" };
    const fallbackProfile = { ...BASE_PROFILE, id: "profile-3", label: "Profile 3" };
    mockState.agentProfiles.items = [BASE_PROFILE, manualProfile, fallbackProfile];
    const { rerender } = render(
      <NewSessionDialog open={true} onOpenChange={vi.fn()} taskId="task-1" />,
    );

    await act(async () => {
      mockAgentSelectorOnChange?.(manualProfile.id);
    });
    await waitFor(() => expect(mockAgentSelectorValue).toBe(manualProfile.id));

    mockState.agentProfiles.items = [
      BASE_PROFILE,
      { ...manualProfile, enabled: false },
      fallbackProfile,
    ];
    rerender(<NewSessionDialog open={true} onOpenChange={vi.fn()} taskId="task-1" />);

    await waitFor(() => expect(mockAgentSelectorValue).toBe(BASE_PROFILE.id));
  });

  it("uses an implicit fallback when a local handoff target is incompatible", async () => {
    mockExecutorProfile = {
      id: "executor-profile-1",
      name: "Local",
      executor_id: "executor-1",
      executor_type: "local_pc",
      prepare_script: "",
      cleanup_script: "",
      created_at: "2026-01-01T00:00:00Z",
      updated_at: "2026-01-01T00:00:00Z",
    };
    mockState.agentProfiles.items = [
      BASE_PROFILE,
      { ...BASE_PROFILE, id: "profile-2", label: "Profile 2", capability_status: "not_installed" },
    ];

    render(
      <NewSessionDialog
        open={true}
        onOpenChange={vi.fn()}
        taskId="task-1"
        handoff={{ sourceSessionId: "session-9", targetProfileId: "profile-2" }}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: PLUGIN_COMPOSER_LABEL }));

    await waitFor(() =>
      expect(mockBuildStartRequest).toHaveBeenCalledWith(
        "task-1",
        "profile-1",
        expect.objectContaining({ profileExplicit: false }),
      ),
    );
  });
});
