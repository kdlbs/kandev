import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type ComponentProps,
  type ReactNode,
} from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { FileAttachment } from "@/components/task/chat/file-attachment";
import type { TaskFormInputsHandle } from "@/components/task-create-dialog-types";
import type { QuickChatSetupDraft } from "./use-quick-chat-setup-draft";
import { QuickChatSetup } from "./quick-chat-setup";

const AGENT_A_ID = "agent-a";
const AGENT_B_ID = "agent-b";
const SEND_TEST_ID = "quick-chat-send";
const DESCRIPTION_TEST_ID = "task-description-input";
const PROMPT_MESSAGE = "Review this code";
let defaultAgentId = AGENT_A_ID;
let defaultConfigAgentId = AGENT_B_ID;
let chatSubmitKey: "enter" | "cmd_enter" = "enter";
let agentProfiles: Array<{ id: string; enabled?: boolean }> = [
  { id: AGENT_A_ID },
  { id: AGENT_B_ID },
];
const AGENT_SELECTOR_TEST_ID = "agent-profile-selector";

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: unknown) => unknown) =>
    selector({
      features: { dynamicAgentRouting: true },
      userSettings: { chatSubmitKey },
      agentProfiles: { items: agentProfiles },
      workspaces: {
        items: [
          {
            id: "workspace-1",
            default_agent_profile_id: defaultAgentId,
            default_config_agent_profile_id: defaultConfigAgentId,
          },
        ],
      },
    }),
}));

vi.mock("@/components/task-create-dialog-options", () => ({
  useAgentProfileOptions: () =>
    agentProfiles.map((profile) => ({
      value: profile.id,
      label: profile.id,
      renderLabel: () => profile.id,
    })),
}));

vi.mock("@/components/task-create-dialog-selectors", () => ({
  TaskFormInputs: ({
    initialDescription,
    initialAttachments,
    descriptionValueRef,
    onDescriptionValueChange,
    onAttachmentsChange,
    onKeyDown,
    toolbarActions,
  }: {
    initialDescription: string;
    initialAttachments?: FileAttachment[];
    descriptionValueRef: React.RefObject<TaskFormInputsHandle | null>;
    onDescriptionValueChange?: (message: string) => void;
    onAttachmentsChange?: (attachments: FileAttachment[]) => void;
    onKeyDown?: (event: React.KeyboardEvent<HTMLTextAreaElement>) => void;
    toolbarActions?: ReactNode;
  }) => {
    const [attachments] = useState(initialAttachments ?? []);
    const inputRef = useRef<HTMLTextAreaElement>(null);
    useEffect(() => onAttachmentsChange?.(attachments), [attachments, onAttachmentsChange]);
    descriptionValueRef.current = {
      getValue: () => inputRef.current?.value ?? initialDescription,
      setValue: (value) => {
        if (inputRef.current) inputRef.current.value = value;
        onDescriptionValueChange?.(value);
      },
      getAttachments: () => attachments,
    };
    return (
      <div data-testid="mock-opening-composer">
        <textarea
          ref={inputRef}
          data-testid={DESCRIPTION_TEST_ID}
          defaultValue={initialDescription}
          onChange={(event) => onDescriptionValueChange?.(event.target.value)}
          onKeyDown={onKeyDown}
        />
        {toolbarActions}
      </div>
    );
  },
}));

vi.mock("./quick-chat-agent-picker", () => ({
  QuickChatAgentPicker: ({
    value,
    onValueChange,
    placeholder,
  }: {
    value: string;
    onValueChange: (value: string) => void;
    placeholder: string;
  }) => (
    <button
      type="button"
      data-testid={AGENT_SELECTOR_TEST_ID}
      className="border-input"
      onClick={() => onValueChange(AGENT_B_ID)}
    >
      {value || placeholder}
    </button>
  ),
}));

vi.mock("@/components/task-create-dialog-workspace-repo-chips", () => ({
  WorkspaceRepoChips: () => null,
}));

vi.mock("@/hooks/domains/workspace/use-repositories", () => ({
  useRepositories: () => ({ repositories: [], isLoading: false }),
}));

vi.mock("@/hooks/domains/features/use-feature", () => ({ useFeature: () => true }));
vi.mock("@/hooks/use-compact-task-chrome", () => ({ useTouchDrawer: () => false }));

vi.mock("@kandev/ui/tooltip", () => ({
  Tooltip: ({ children }: { children: ReactNode }) => <>{children}</>,
  TooltipContent: ({ children }: { children: ReactNode }) => <>{children}</>,
  TooltipTrigger: ({ children }: { children: ReactNode }) => <>{children}</>,
}));

vi.mock("./configuration-chat-toggle", () => ({
  ConfigurationChatToggle: ({
    checked,
    onCheckedChange,
  }: {
    checked: boolean;
    onCheckedChange: (checked: boolean) => void;
  }) => (
    <button
      role="switch"
      aria-checked={checked}
      aria-label="Configuration chat"
      onClick={() => onCheckedChange(!checked)}
    >
      Configuration chat
    </button>
  ),
}));

const initialDraft: QuickChatSetupDraft = {
  message: "",
  attachments: [],
  agentProfileId: AGENT_A_ID,
  agentProfileExplicit: false,
  repositories: [],
};

const props = {
  workspaceId: "workspace-1",
  kind: "chat" as const,
  canCreateConfigurationChat: true,
  defaultConfigProfileId: AGENT_B_ID,
  pendingAgentId: null,
  configurationStarting: false,
  configurationError: null,
  quickChatError: null,
  draft: initialDraft,
  onDraftChange: vi.fn(),
  onStartQuickChat: vi.fn().mockResolvedValue(true),
  onStartConfigChat: vi.fn().mockResolvedValue(true),
  onKindChange: vi.fn(),
  onDiscardDraft: vi.fn(),
  onRegisterDiscard: vi.fn(() => () => {}),
};

function QuickChatSetupHarness({
  overrides = {},
}: {
  overrides?: Partial<ComponentProps<typeof QuickChatSetup>>;
}) {
  const [draft, setDraft] = useState(overrides.draft ?? initialDraft);
  const handleDraftChange = useCallback((patch: Partial<QuickChatSetupDraft>) => {
    props.onDraftChange(patch);
    setDraft((current) => ({ ...current, ...patch }));
  }, []);
  return (
    <QuickChatSetup {...props} {...overrides} draft={draft} onDraftChange={handleDraftChange} />
  );
}

beforeEach(() => {
  defaultAgentId = AGENT_A_ID;
  defaultConfigAgentId = AGENT_B_ID;
  chatSubmitKey = "enter";
  agentProfiles = [{ id: AGENT_A_ID }, { id: AGENT_B_ID }];
  vi.clearAllMocks();
});

afterEach(cleanup);

describe("QuickChatSetup", () => {
  it("publishes the initial attachment list once while retaining draft updates", async () => {
    render(<QuickChatSetupHarness />);

    await waitFor(() => expect(props.onDraftChange).toHaveBeenCalledWith({ attachments: [] }));
    expect(props.onDraftChange).toHaveBeenCalledTimes(1);
  });

  it("shows the opening prompt and configuration choice", () => {
    render(<QuickChatSetupHarness />);

    expect(screen.getByText(/idea, question, or codebase/i)).toBeTruthy();
    fireEvent.click(screen.getByRole("switch", { name: "Configuration chat" }));
    expect(props.onKindChange).toHaveBeenCalledWith("config");
  });

  it("hides configuration mode when a configuration session already exists", () => {
    render(<QuickChatSetupHarness overrides={{ canCreateConfigurationChat: false }} />);
    expect(screen.queryByRole("switch", { name: "Configuration chat" })).toBeNull();
  });

  it("shows Send in the composer and waits for a prompt before starting", () => {
    render(<QuickChatSetupHarness />);

    const send = screen.getByTestId(SEND_TEST_ID) as HTMLButtonElement;
    expect(send.disabled).toBe(true);
    fireEvent.change(screen.getByTestId(DESCRIPTION_TEST_ID), {
      target: { value: PROMPT_MESSAGE },
    });
    expect(send.disabled).toBe(false);
    fireEvent.click(send);

    expect(props.onStartQuickChat).toHaveBeenCalledWith(
      AGENT_A_ID,
      [],
      expect.objectContaining({ message: PROMPT_MESSAGE, clientMessageId: expect.any(String) }),
    );
  });

  it("uses the configured chat submit key and keeps modified or composing Enter in the draft", async () => {
    chatSubmitKey = "cmd_enter";
    render(<QuickChatSetupHarness />);
    const input = screen.getByTestId(DESCRIPTION_TEST_ID);
    fireEvent.change(input, { target: { value: PROMPT_MESSAGE } });

    fireEvent.keyDown(input, { key: "Enter" });
    fireEvent.keyDown(input, { key: "Enter", ctrlKey: true, shiftKey: true });
    fireEvent.keyDown(input, { key: "Enter", ctrlKey: true, altKey: true });
    fireEvent.keyDown(input, { key: "Enter", ctrlKey: true, repeat: true });
    fireEvent.keyDown(input, { key: "Enter", ctrlKey: true, isComposing: true, keyCode: 229 });

    expect(props.onStartQuickChat).not.toHaveBeenCalled();

    fireEvent.keyDown(input, { key: "Enter", ctrlKey: true });
    await waitFor(() => expect(props.onStartQuickChat).toHaveBeenCalledOnce());
    expect(props.onStartQuickChat).toHaveBeenCalledWith(
      AGENT_A_ID,
      [],
      expect.objectContaining({ message: PROMPT_MESSAGE }),
    );
  });

  it("delivers staged attachments with the opening message", () => {
    const attachment: FileAttachment = {
      id: "attachment-row",
      attachmentId: "staged-attachment",
      mimeType: "image/png",
      fileName: "diagram.png",
      size: 8,
      isImage: true,
      deliveryMode: "prompt",
      uploadStatus: "ready",
    };
    render(
      <QuickChatSetupHarness
        overrides={{ draft: { ...initialDraft, attachments: [attachment] } }}
      />,
    );
    fireEvent.change(screen.getByTestId(DESCRIPTION_TEST_ID), {
      target: { value: "Explain this image" },
    });
    fireEvent.click(screen.getByTestId(SEND_TEST_ID));

    expect(props.onStartQuickChat).toHaveBeenCalledWith(
      AGENT_A_ID,
      [],
      expect.objectContaining({
        message: "Explain this image",
        attachments: [
          expect.objectContaining({ attachment_id: "staged-attachment", name: "diagram.png" }),
        ],
      }),
    );
  });

  it("keeps the opening draft available when creation fails so the user can retry", async () => {
    props.onStartQuickChat.mockResolvedValueOnce(false).mockResolvedValueOnce(true);
    render(<QuickChatSetupHarness />);
    const editor = screen.getByTestId(DESCRIPTION_TEST_ID);
    const send = screen.getByTestId(SEND_TEST_ID);
    fireEvent.change(editor, { target: { value: "Retry this opening prompt" } });

    fireEvent.click(send);
    await waitFor(() => expect(props.onStartQuickChat).toHaveBeenCalledTimes(1));
    expect((editor as HTMLTextAreaElement).value).toBe("Retry this opening prompt");
    expect((send as HTMLButtonElement).disabled).toBe(false);

    fireEvent.click(send);
    await waitFor(() => expect(props.onStartQuickChat).toHaveBeenCalledTimes(2));
    expect(props.onStartQuickChat.mock.calls[1]?.[2]).toMatchObject({
      message: "Retry this opening prompt",
    });
  });
});

describe("QuickChatSetup profile validation", () => {
  it("uses the configuration default until the user selects a profile", () => {
    render(
      <QuickChatSetupHarness
        overrides={{
          kind: "config",
          draft: { ...initialDraft, agentProfileId: "", agentProfileExplicit: false },
        }}
      />,
    );

    expect(props.onDraftChange).toHaveBeenCalledWith({ agentProfileId: AGENT_B_ID });
  });

  it("falls back to the workspace profile when the configuration default is unavailable", () => {
    agentProfiles = [{ id: AGENT_A_ID }, { id: AGENT_B_ID, enabled: false }];
    render(
      <QuickChatSetupHarness
        overrides={{
          kind: "config",
          draft: { ...initialDraft, agentProfileId: "", agentProfileExplicit: false },
        }}
      />,
    );

    expect(props.onDraftChange).toHaveBeenCalledWith({ agentProfileId: AGENT_A_ID });
  });

  it("preserves an explicit profile and blocks Send when that profile is disabled", () => {
    agentProfiles = [{ id: AGENT_A_ID }, { id: AGENT_B_ID, enabled: false }];
    render(
      <QuickChatSetupHarness
        overrides={{
          draft: { ...initialDraft, agentProfileId: AGENT_B_ID, agentProfileExplicit: true },
        }}
      />,
    );

    expect(screen.getByTestId(AGENT_SELECTOR_TEST_ID).textContent).toContain(AGENT_B_ID);
    fireEvent.change(screen.getByTestId(DESCRIPTION_TEST_ID), {
      target: { value: "Still editable" },
    });
    expect((screen.getByTestId(SEND_TEST_ID) as HTMLButtonElement).disabled).toBe(true);
    expect(props.onDraftChange).not.toHaveBeenCalledWith({ agentProfileId: "" });
  });
});

describe("QuickChatSetup launch errors", () => {
  it("shows configuration launch failures beside the shared prompt", () => {
    render(
      <QuickChatSetupHarness
        overrides={{
          kind: "config",
          configurationError: "Launch failed",
          draft: { ...initialDraft, agentProfileId: AGENT_B_ID },
        }}
      />,
    );
    expect(screen.getByRole("alert").textContent).toBe("Launch failed");
  });

  it("shows ordinary chat creation failures beside the retained prompt", () => {
    render(
      <QuickChatSetupHarness
        overrides={{
          quickChatError: "Quick Chat could not start",
          draft: { ...initialDraft, message: "Keep this request for retry" },
        }}
      />,
    );

    expect(screen.getByRole("alert").textContent).toBe("Quick Chat could not start");
    expect(screen.getByTestId(DESCRIPTION_TEST_ID)).toHaveProperty(
      "value",
      "Keep this request for retry",
    );
  });
});
