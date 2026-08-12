/**
 * The composer capability has to survive the whole prop chain
 * `ChatInputToolbar -> Desktop/MobileChatInputToolbar -> DesktopRightSection
 * -> ChatInputPluginActions`. Every hop in that chain forwards an explicit
 * prop list, and two of them silently dropped `composerCapability` — the slot
 * then fell back to the always-`unavailable` stub, so a plugin action
 * rendered fine and did nothing.
 *
 * These tests render the real top-level `ChatInputToolbar` at both
 * presentations and assert on what the slot actually received, which is the
 * only level at which a dropped hop is visible. `chat-input-toolbar.test.tsx`
 * mocks the slot away, and the sub-toolbar tests render past the missing hop.
 */
import { describe, it, expect, vi, afterEach, beforeEach } from "vitest";
import { render, cleanup } from "@testing-library/react";
import { StateProvider } from "@/components/state-provider";
import type { PluginComposerCapability } from "@/lib/plugins/types";
import type { ChatInputActionsSlotProps } from "./chat-input-plugin-actions";

const responsiveMock = vi.hoisted(() => ({
  breakpoint: "desktop" as "mobile" | "tablet" | "compactDesktop" | "desktop",
}));

const slotSpy = vi.hoisted(() => ({
  received: [] as ChatInputActionsSlotProps[],
}));

afterEach(() => {
  cleanup();
});

beforeEach(() => {
  responsiveMock.breakpoint = "desktop";
  slotSpy.received = [];
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
  KeyboardShortcutTooltip: ({ children }: { children: React.ReactNode }) => <>{children}</>,
}));

vi.mock("@/components/task/model-selector", () => ({
  ModelSelector: () => <button type="button">model</button>,
}));

vi.mock("@/components/task/mode-selector", () => ({
  ModeSelector: () => <button type="button">mode</button>,
}));

vi.mock("@/components/task/sessions-dropdown", () => ({
  SessionsDropdown: () => <button type="button">sessions</button>,
}));

vi.mock("@/components/task/chat/token-usage-display", () => ({
  TokenUsageDisplay: () => <span />,
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

vi.mock("./voice-input-button", () => ({
  VoiceInputButton: () => <button type="button">Voice</button>,
}));

// Records what the slot was handed instead of rendering plugin components.
vi.mock("./chat-input-plugin-actions", () => ({
  ChatInputPluginActions: (props: Record<string, unknown>) => {
    slotSpy.received.push(props as unknown as ChatInputActionsSlotProps);
    return null;
  },
}));

import { ChatInputToolbar } from "./chat-input-toolbar";
import type { ChatInputToolbarProps } from "./chat-input-toolbar";

function makeCapability(): PluginComposerCapability {
  return {
    insertText: () => ({ status: "inserted" }),
    focus: () => ({ status: "focused" }),
    submit: async () => ({ status: "submitted" }),
  };
}

function renderToolbar(overrides: Partial<ChatInputToolbarProps> = {}) {
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
        onAttachFiles={() => {}}
        {...overrides}
      />
    </StateProvider>,
  );
}

function lastSlotProps(): ChatInputActionsSlotProps {
  const last = slotSpy.received.at(-1);
  expect(last, "the plugin slot should have rendered").toBeDefined();
  return last!;
}

describe("ChatInputToolbar forwards the composer capability to the plugin slot", () => {
  it("reaches the slot on desktop", () => {
    const composer = makeCapability();
    renderToolbar({ composerCapability: composer });

    expect(lastSlotProps().composer).toBe(composer);
  });

  it("reaches the slot on mobile", () => {
    responsiveMock.breakpoint = "mobile";
    const composer = makeCapability();
    renderToolbar({ composerCapability: composer });

    expect(lastSlotProps().composer).toBe(composer);
  });

  it("reaches the slot on a tablet, which uses the compact toolbar", () => {
    responsiveMock.breakpoint = "tablet";
    const composer = makeCapability();
    renderToolbar({ composerCapability: composer });

    expect(lastSlotProps().composer).toBe(composer);
  });

  it("carries the surface through so Quick Chat is distinguishable from task chat", () => {
    renderToolbar({ composerCapability: makeCapability(), composerSurface: "quick-chat" });

    expect(lastSlotProps().surface).toBe("quick-chat");
  });

  it("reports the live disabled and submittable gates rather than a snapshot", () => {
    const composer = makeCapability();
    const { rerender } = renderToolbar({ composerCapability: composer, hasContent: false });
    expect(lastSlotProps().submittable).toBe(false);

    rerender(
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
          onAttachFiles={() => {}}
          composerCapability={composer}
        />
      </StateProvider>,
    );

    expect(lastSlotProps().submittable).toBe(true);
  });
});
