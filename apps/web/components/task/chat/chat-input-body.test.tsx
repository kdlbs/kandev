import { createRef } from "react";
import { cleanup, render, screen } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ChatInputBody, type ChatInputBodyProps } from "./chat-input-body";
import { shouldShowCancelAgent } from "./chat-input-container";

const tipTapPropsMock = vi.hoisted(() => vi.fn());

vi.mock("./tiptap-input", () => ({
  TipTapInput: (props: unknown) => {
    tipTapPropsMock(props);
    return <div data-testid="mock-tiptap-input" />;
  },
}));

vi.mock("./chat-input-toolbar", () => ({
  ChatInputToolbar: () => <div data-testid="mock-chat-input-toolbar" />,
}));

vi.mock("./context-items/context-zone", () => ({
  ContextZone: () => <div data-testid="mock-context-zone" />,
}));

afterEach(() => {
  cleanup();
  tipTapPropsMock.mockClear();
});

function props(overrides: Partial<ChatInputBodyProps> = {}): ChatInputBodyProps {
  return {
    containerRef: createRef<HTMLDivElement>(),
    height: 120,
    resizeHandleProps: { onMouseDown: vi.fn(), onDoubleClick: vi.fn() },
    isStarting: false,
    isAgentBusy: false,
    hasClarification: false,
    showRequestChangesTooltip: false,
    hasPendingComments: false,
    planModeEnabled: false,
    showFocusHint: false,
    needsRecovery: false,
    addFiles: vi.fn().mockResolvedValue(undefined),
    contextAreaProps: {
      hasContextZone: false,
      allItems: [],
      sessionId: "session-1",
    },
    editorAreaProps: {
      inputRef: createRef(),
      value: "",
      handleChange: vi.fn(),
      handleSubmitWithReset: vi.fn(),
      inputPlaceholder: "Ask to make changes",
      isDisabled: false,
      submitDisabled: false,
      planModeEnabled: false,
      planModeAvailable: true,
      mcpServers: [],
      submitKey: "cmd_enter",
      setIsInputFocused: vi.fn(),
      sessionId: "session-1",
      taskId: "task-1",
      planContextEnabled: false,
      addFiles: vi.fn().mockResolvedValue(undefined),
      fileInputRef: createRef(),
      showRequestChangesTooltip: false,
      isAgentBusy: false,
      onPlanModeChange: vi.fn(),
      taskDescription: "",
      isSending: false,
      onCancel: vi.fn(),
      contextCount: 0,
      contextPopoverOpen: false,
      setContextPopoverOpen: vi.fn(),
      contextFiles: [],
    },
    ...overrides,
  };
}

describe("ChatInputBody", () => {
  it("keeps the regular editor enabled while a structured clarification is pending", () => {
    render(
      <TooltipProvider>
        <ChatInputBody
          {...props({
            hasClarification: true,
          })}
        />
      </TooltipProvider>,
    );

    expect(tipTapPropsMock).toHaveBeenCalledWith(expect.objectContaining({ disabled: false }));
  });

  it("keeps entity references explicitly disabled unless the chat surface enables them", () => {
    render(
      <TooltipProvider>
        <ChatInputBody {...props()} />
      </TooltipProvider>,
    );

    expect(tipTapPropsMock).toHaveBeenCalledWith(
      expect.objectContaining({ entityReferencesEnabled: false }),
    );
  });

  it("reserves right-side editable space while the focus hint is visible", () => {
    render(
      <TooltipProvider>
        <ChatInputBody {...props({ showFocusHint: true })} />
      </TooltipProvider>,
    );

    expect(screen.getByText("to focus")).toBeTruthy();
    expect(screen.getByTestId("chat-input-editor-shell").className).not.toContain("pr-28");
    expect(screen.getByTestId("mock-tiptap-input").parentElement?.className).toContain("pr-28");
  });

  it("does not reserve focus-hint space when the hint is hidden", () => {
    render(
      <TooltipProvider>
        <ChatInputBody {...props({ showFocusHint: false })} />
      </TooltipProvider>,
    );

    expect(screen.getByTestId("chat-input-editor-shell").className).not.toContain("pr-28");
    expect(screen.getByTestId("mock-tiptap-input").parentElement?.className).not.toContain("pr-28");
  });
});

describe("shouldShowCancelAgent", () => {
  const detachedClarification = { metadata: { agent_disconnected: true } } as never;
  const attachedClarification = {} as never;

  it.each([
    [
      "suppresses cancel for detached clarification while session is RUNNING",
      true,
      detachedClarification,
      false,
    ],
    [
      "suppresses cancel for detached clarification while session is WAITING",
      false,
      detachedClarification,
      false,
    ],
    [
      "keeps cancel for clarification without detached metadata",
      false,
      attachedClarification,
      true,
    ],
    ["keeps cancel for a running session without clarification", true, null, true],
  ])("%s", (_name, isAgentBusy, pendingClarification, expected) => {
    expect(shouldShowCancelAgent(isAgentBusy, pendingClarification)).toBe(expected);
  });
});
