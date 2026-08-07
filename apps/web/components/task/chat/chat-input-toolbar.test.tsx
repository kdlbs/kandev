import { describe, it, expect, vi, afterEach, beforeEach } from "vitest";
import { render, screen, fireEvent, act, cleanup } from "@testing-library/react";
import { StateProvider } from "@/components/state-provider";
import type { TaskSession } from "@/lib/types/http";

const responsiveMock = vi.hoisted(() => ({
  breakpoint: "desktop" as "mobile" | "tablet" | "compactDesktop" | "desktop",
}));

afterEach(() => {
  cleanup();
});

beforeEach(() => {
  responsiveMock.breakpoint = "desktop";
});

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({
    breakpoint: responsiveMock.breakpoint,
    isMobile: responsiveMock.breakpoint === "mobile",
    isTablet: responsiveMock.breakpoint === "tablet",
    isDesktop:
      responsiveMock.breakpoint === "compactDesktop" || responsiveMock.breakpoint === "desktop",
    isCompactDesktop: responsiveMock.breakpoint === "compactDesktop",
    isFullDesktop: responsiveMock.breakpoint === "desktop",
    isFinePointer: true,
    usesDesktopWorkbench:
      responsiveMock.breakpoint === "compactDesktop" || responsiveMock.breakpoint === "desktop",
  }),
}));

vi.mock("@kandev/ui/tooltip", () => ({
  Tooltip: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  TooltipTrigger: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  TooltipContent: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  TooltipProvider: ({ children }: { children: React.ReactNode }) => <>{children}</>,
}));

vi.mock("@/components/keyboard-shortcut-tooltip", () => ({
  KeyboardShortcutTooltip: ({
    children,
    description,
  }: {
    children: React.ReactNode;
    description?: string;
  }) => (
    <>
      {children}
      {description ? <span>{description}</span> : null}
    </>
  ),
}));

vi.mock("@/components/task/model-selector", () => ({
  ModelSelector: ({ triggerClassName }: { triggerClassName?: string }) => (
    <button type="button" data-testid="mock-model-selector" className={triggerClassName}>
      model
    </button>
  ),
}));

vi.mock("@/components/task/mode-selector", () => ({
  ModeSelector: ({ triggerClassName }: { triggerClassName?: string }) => (
    <button type="button" data-testid="mock-mode-selector" className={triggerClassName}>
      mode
    </button>
  ),
}));

vi.mock("@/components/task/sessions-dropdown", () => ({
  SessionsDropdown: () => (
    <button type="button" data-testid="mock-sessions-dropdown">
      sessions
    </button>
  ),
}));

vi.mock("@/components/task/chat/token-usage-display", () => ({
  TokenUsageDisplay: () => <span data-testid="mock-token-usage" />,
}));

vi.mock("@/components/enhance-prompt-button", () => ({
  EnhancePromptButton: () => <button type="button">Enhance</button>,
}));

vi.mock("./context-popover", () => ({
  ContextPopover: ({ trigger }: { trigger: React.ReactNode }) => <>{trigger}</>,
}));

vi.mock("./implement-plan-button", () => ({
  ImplementPlanButton: () => <button type="button">Implement plan</button>,
}));

vi.mock("./reset-context-button", () => ({
  ResetContextButton: () => <button type="button">Reset context</button>,
}));

vi.mock("./chat-input-plugin-actions", () => ({
  ChatInputPluginActions: () => null,
}));

vi.mock("./voice-input-button", () => ({
  VoiceInputButton: () => <button type="button">Voice</button>,
}));

import { ChatInputToolbar } from "./chat-input-toolbar";
import type { ChatInputToolbarProps } from "./chat-input-toolbar";

const MOBILE_TOOLBAR_TEST_ID = "mobile-chat-input-toolbar";
const CANCEL_AGENT_BUTTON_TEST_ID = "cancel-agent-button";
const SESSION_TIMESTAMP = "2026-01-01T00:00:00Z";

function deferred<T>() {
  let resolve!: (v: T) => void;
  const promise = new Promise<T>((res) => {
    resolve = res;
  });
  return { promise, resolve };
}

function renderToolbar(onCancel: () => void | Promise<void>) {
  return render(
    <StateProvider>
      <ChatInputToolbar
        planModeEnabled={false}
        onPlanModeChange={() => {}}
        sessionId="s1"
        taskId="t1"
        taskDescription=""
        isAgentBusy
        isDisabled={false}
        isSending={false}
        onCancel={onCancel}
        onSubmit={() => {}}
        minimalToolbar
      />
    </StateProvider>,
  );
}

function renderFullToolbar(overrides: Partial<ChatInputToolbarProps> = {}) {
  return render(
    <StateProvider>
      <ChatInputToolbar
        planModeEnabled={false}
        onPlanModeChange={() => {}}
        sessionId="s1"
        taskId="t1"
        taskDescription=""
        isAgentBusy={false}
        hasContent
        isDisabled={false}
        isSending={false}
        onCancel={() => {}}
        onSubmit={() => {}}
        hidePlanMode
        mcpServers={["filesystem"]}
        contextCount={2}
        onAttachFiles={() => {}}
        onEnhancePrompt={() => {}}
        isUtilityConfigured
        {...overrides}
      />
    </StateProvider>,
  );
}

function makeRunningSession(cancellationPending: boolean): TaskSession {
  return {
    id: "s1",
    task_id: "t1",
    state: "RUNNING",
    cancellation_pending: cancellationPending,
    started_at: SESSION_TIMESTAMP,
    updated_at: SESSION_TIMESTAMP,
  } as TaskSession;
}

describe("ChatInputToolbar backend cancellation state", () => {
  it("renders backend-owned pending state after store hydration", () => {
    render(
      <StateProvider initialState={{ taskSessions: { items: { s1: makeRunningSession(true) } } }}>
        <ChatInputToolbar
          planModeEnabled={false}
          onPlanModeChange={() => {}}
          sessionId="s1"
          taskId="t1"
          taskDescription=""
          isAgentBusy
          isDisabled={false}
          isSending={false}
          onCancel={() => {}}
          onSubmit={() => {}}
          hasContent={false}
        />
      </StateProvider>,
    );

    const button = screen.getByTestId(CANCEL_AGENT_BUTTON_TEST_ID) as HTMLButtonElement;
    expect(button.disabled).toBe(true);
    expect(button.querySelector('[role="status"]')).toBeTruthy();
  });

  it("uses backend pending state as the duplicate-click guard", () => {
    const onCancel = vi.fn();

    render(
      <StateProvider initialState={{ taskSessions: { items: { s1: makeRunningSession(true) } } }}>
        <ChatInputToolbar
          planModeEnabled={false}
          onPlanModeChange={() => {}}
          sessionId="s1"
          taskId="t1"
          taskDescription=""
          isAgentBusy
          isDisabled={false}
          isSending={false}
          onCancel={onCancel}
          onSubmit={() => {}}
          hasContent={false}
        />
      </StateProvider>,
    );

    const button = screen.getByTestId(CANCEL_AGENT_BUTTON_TEST_ID) as HTMLButtonElement;
    fireEvent.click(button);
    expect(onCancel).not.toHaveBeenCalled();
    expect(button.disabled).toBe(true);
    expect(button.querySelector('[role="status"]')).toBeTruthy();
  });
});

// The cancel button must disable itself while a cancel request is in flight.
// Without this guard, an impatient user clicking it repeatedly while the agent
// tears down a long-running tool (Claude Monitor, etc.) sends N cancel requests
// to the backend, each producing a duplicate "Turn cancelled by user" message.
describe("ChatInputToolbar cancel button", () => {
  it.each(["desktop", "mobile"] as const)(
    "keeps cancellation progress after a %s toolbar remount",
    async (breakpoint) => {
      responsiveMock.breakpoint = breakpoint;
      const { promise, resolve } = deferred<void>();
      const onCancel = vi.fn(() => promise);
      let mounted = true;

      const renderToolbarTree = () => (
        <StateProvider>
          {mounted ? (
            <ChatInputToolbar
              planModeEnabled={false}
              onPlanModeChange={() => {}}
              sessionId="s1"
              taskId="t1"
              taskDescription=""
              isAgentBusy
              isDisabled={false}
              isSending={false}
              onCancel={onCancel}
              onSubmit={() => {}}
              hasContent={false}
            />
          ) : null}
        </StateProvider>
      );

      const view = render(renderToolbarTree());
      fireEvent.click(screen.getByTestId(CANCEL_AGENT_BUTTON_TEST_ID));
      await act(async () => {});

      mounted = false;
      view.rerender(renderToolbarTree());
      mounted = true;
      view.rerender(renderToolbarTree());

      const remountedButton = screen.getByTestId(CANCEL_AGENT_BUTTON_TEST_ID) as HTMLButtonElement;
      expect(remountedButton.disabled).toBe(true);
      expect(remountedButton.querySelector('[role="status"]')).toBeTruthy();
      expect(onCancel).toHaveBeenCalledTimes(1);

      await act(async () => {
        resolve();
        await promise;
      });
      expect(remountedButton.disabled).toBe(false);
    },
  );

  it("keeps the queue affordance without a cancel control after clarification detaches", () => {
    renderFullToolbar({ isAgentBusy: true, canCancelAgent: false });

    expect(screen.queryByTestId(CANCEL_AGENT_BUTTON_TEST_ID)).toBeNull();
    expect(screen.getByTestId("submit-message-button")).toBeTruthy();
    expect(screen.getByText("Queue message")).toBeTruthy();
  });

  it("disables itself and blocks duplicate clicks while cancel is in flight", async () => {
    const { promise, resolve } = deferred<void>();
    const onCancel = vi.fn(() => promise);

    renderToolbar(onCancel);

    const button = screen.getByTestId(CANCEL_AGENT_BUTTON_TEST_ID) as HTMLButtonElement;
    expect(button.disabled).toBe(false);

    fireEvent.click(button);
    // Click is processed synchronously; React then flushes the setState that
    // marks the button disabled. We assert post-flush.
    await act(async () => {});
    expect(button.disabled).toBe(true);
    expect(onCancel).toHaveBeenCalledTimes(1);

    // Subsequent clicks while the promise is pending must not call onCancel.
    fireEvent.click(button);
    fireEvent.click(button);
    fireEvent.click(button);
    expect(onCancel).toHaveBeenCalledTimes(1);

    // Once the in-flight cancel resolves, the button re-enables for retry
    // (rare, but possible if the first cancel returned an error).
    await act(async () => {
      resolve();
      await promise;
    });
    expect(button.disabled).toBe(false);
  });

  it("re-enables the button if onCancel rejects", async () => {
    const { promise, resolve } = deferred<void>();
    const onCancel = vi.fn(() => promise.then(() => Promise.reject(new Error("network"))));

    renderToolbar(onCancel);
    const button = screen.getByTestId(CANCEL_AGENT_BUTTON_TEST_ID) as HTMLButtonElement;

    fireEvent.click(button);
    await act(async () => {});
    expect(button.disabled).toBe(true);

    await act(async () => {
      resolve();
      // Allow the rejected promise to settle inside the click handler.
      await new Promise((r) => setTimeout(r, 0));
    });
    expect(button.disabled).toBe(false);
  });
});

describe("ChatInputToolbar submit button", () => {
  it("shows the setup-disabled reason while keeping the submit button disabled", () => {
    render(
      <StateProvider>
        <ChatInputToolbar
          planModeEnabled={false}
          onPlanModeChange={() => {}}
          sessionId="s1"
          taskId="t1"
          taskDescription=""
          isAgentBusy={false}
          hasContent
          isDisabled
          submitDisabledReason="The agent is still being set up."
          isSending={false}
          onCancel={() => {}}
          onSubmit={() => {}}
          minimalToolbar
        />
      </StateProvider>,
    );

    expect((screen.getByTestId("submit-message-button") as HTMLButtonElement).disabled).toBe(true);
    expect(screen.getByText("The agent is still being set up.")).toBeTruthy();
  });
});

describe("ChatInputToolbar responsive wrapper", () => {
  it("routes mobile breakpoints to the compact toolbar without a duplicate sessions control", () => {
    responsiveMock.breakpoint = "mobile";
    renderFullToolbar();

    const mobileToolbar = screen.getByTestId(MOBILE_TOOLBAR_TEST_ID);
    expect(mobileToolbar).toBeTruthy();
    expect(mobileToolbar.getAttribute("data-legacy-testid")).toBe("chat-input-toolbar");
    expect(screen.getByTestId("mobile-chat-toolbar-left-actions")).toBeTruthy();
    expect(screen.getByTestId("mobile-chat-toolbar-left-actions").className).toContain("pr-8");
    expect(screen.getByTestId("mobile-chat-toolbar-scroll-fade").className).toContain(
      "bg-gradient-to-l",
    );
    expect(screen.getByTestId("toolbar-item-mcp")).toBeTruthy();
    expect(screen.getByTestId("toolbar-item-mode")).toBeTruthy();
    expect(screen.getByTestId("toolbar-item-model")).toBeTruthy();
    expect(screen.queryByTestId("toolbar-item-sessions")).toBeNull();
    expect(screen.queryByTestId("mock-sessions-dropdown")).toBeNull();
    expect(screen.getByTestId("toolbar-item-context")).toBeTruthy();
    expect(screen.getByTestId("toolbar-item-reset-context")).toBeTruthy();
    expect(screen.getByTestId("toolbar-item-enhance")).toBeTruthy();
    expect(screen.getByTestId("mock-mode-selector").className).toContain("max-w-[46vw]");
    expect(screen.getByTestId("mock-model-selector").className).toContain("max-w-[56vw]");
    expect(screen.getByTestId("mock-model-selector").className).toContain("min-w-0");
    expect(screen.getByTestId("mock-model-selector").className).toContain("overflow-hidden");
  });

  it("keeps the compact sessions control on tablet layouts", () => {
    responsiveMock.breakpoint = "tablet";
    renderFullToolbar();

    expect(screen.getByTestId(MOBILE_TOOLBAR_TEST_ID)).toBeTruthy();
    expect(screen.getByTestId("toolbar-item-sessions")).toBeTruthy();
    expect(screen.getByTestId("mock-sessions-dropdown")).toBeTruthy();
  });

  it("hides the compact sessions control when requested", () => {
    responsiveMock.breakpoint = "tablet";
    renderFullToolbar({ hideSessionsDropdown: true });

    expect(screen.getByTestId(MOBILE_TOOLBAR_TEST_ID)).toBeTruthy();
    expect(screen.queryByTestId("toolbar-item-sessions")).toBeNull();
    expect(screen.queryByTestId("mock-sessions-dropdown")).toBeNull();
  });

  it.each(["compactDesktop", "desktop"] as const)(
    "routes %s breakpoints to the desktop toolbar",
    (breakpoint) => {
      responsiveMock.breakpoint = breakpoint;
      renderFullToolbar();

      expect(screen.getByTestId("chat-input-toolbar")).toBeTruthy();
      expect(screen.queryByTestId(MOBILE_TOOLBAR_TEST_ID)).toBeNull();
    },
  );
});
