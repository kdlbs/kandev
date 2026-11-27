import { cleanup, render } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { QuickChatTabs } from "./quick-chat-tab-strip";

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({ isFinePointer: true }),
}));
vi.mock("@/components/state-provider", () => ({ useAppStore: () => false }));

const terminals = [1, 2].map((sequence) => ({
  tabId: `terminal-${sequence}`,
  workspaceId: "workspace",
  sessionId: null,
  sequence,
  status: "running" as const,
}));
const sessions = [
  { sessionId: "chat-1", workspaceId: "workspace", kind: "chat" as const, name: "Chat one" },
  { sessionId: "quick-chat-setup:workspace:chat", workspaceId: "workspace", kind: "chat" as const },
];
const props = {
  sessions,
  terminalTabs: terminals,
  activeKind: "terminal" as const,
  activeSessionId: null,
  activeTerminalTabId: "terminal-1",
  onTabChange: vi.fn(),
  onTabClose: vi.fn(),
  onNewChat: vi.fn(),
  onNewTerminal: vi.fn(),
  onTerminalClose: vi.fn(),
  onTerminalActivate: vi.fn(),
  onRename: vi.fn(),
  onCloseModal: vi.fn(),
  tabOrderSyncError: null,
};

type Edges = { left: number; right: number };
let positions: Record<string, Edges>;
let activation: Edges | null;
const rectangle = (left: number, right: number) =>
  ({
    left,
    right,
    width: right - left,
    top: 0,
    bottom: 24,
    height: 24,
    x: left,
    y: 0,
    toJSON() {},
  }) as DOMRect;

beforeEach(() => {
  positions = {
    "terminal:terminal-1": { left: 0, right: 40 },
    "terminal:terminal-2": { left: 120, right: 160 },
    "conversation:chat-1": { left: 180, right: 220 },
    setup: { left: 240, right: 280 },
  };
  activation = null;
  vi.spyOn(HTMLElement.prototype, "clientWidth", "get").mockImplementation(function (
    this: HTMLElement,
  ) {
    return this.classList.contains("overflow-x-auto") ? 100 : 0;
  });
  vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockImplementation(function (
    this: HTMLElement,
  ) {
    if (this.classList.contains("overflow-x-auto")) return rectangle(0, 100);
    const wrapper = this.closest<HTMLElement>("[data-tab-reference], [data-quick-chat-active]");
    const key = wrapper?.getAttribute("data-tab-reference") ?? "setup";
    const edges = this.tagName === "BUTTON" && activation ? activation : positions[key];
    const scroll = wrapper?.parentElement?.scrollLeft ?? 0;
    return edges ? rectangle(edges.left - scroll, edges.right - scroll) : rectangle(0, 40);
  });
});
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

function mount() {
  const result = render(<QuickChatTabs {...props} />);
  const strip = result.container.querySelector<HTMLElement>(".overflow-x-auto")!;
  return { ...result, strip };
}

describe("active tab reveal", () => {
  it("moves by the minimum distance in both directions and leaves visible tabs alone", () => {
    const { strip, rerender } = mount();
    rerender(<QuickChatTabs {...props} activeTerminalTabId="terminal-2" />);
    expect(strip.scrollLeft).toBe(60);
    rerender(<QuickChatTabs {...props} activeTerminalTabId="terminal-1" />);
    expect(strip.scrollLeft).toBe(0);
    positions["terminal:terminal-2"] = { left: 50, right: 90 };
    rerender(<QuickChatTabs {...props} activeTerminalTabId="terminal-2" />);
    expect(strip.scrollLeft).toBe(0);
  });

  it("reveals conversation and setup selections without moving focus or vertical scroll", () => {
    const { strip, rerender, container } = mount();
    const focused = container.querySelector<HTMLButtonElement>(
      '[data-tab-reference="terminal:terminal-1"] button',
    )!;
    focused.focus();
    strip.scrollTop = 17;
    rerender(<QuickChatTabs {...props} activeKind="conversation" activeSessionId="chat-1" />);
    expect(strip.scrollLeft).toBe(120);
    rerender(
      <QuickChatTabs
        {...props}
        activeKind="conversation"
        activeSessionId="quick-chat-setup:workspace:chat"
      />,
    );
    expect(strip.scrollLeft).toBe(180);
    expect(document.activeElement).toBe(focused);
    expect(strip.scrollTop).toBe(17);
  });

  it("preserves manual browsing through unchanged selection, rename and order changes", () => {
    const { strip, rerender } = mount();
    rerender(<QuickChatTabs {...props} activeTerminalTabId="terminal-2" />);
    expect(strip.scrollLeft).toBe(60);
    strip.scrollLeft = 5;
    rerender(
      <QuickChatTabs
        {...props}
        activeTerminalTabId="terminal-2"
        sessions={[{ ...sessions[0], name: "Renamed" }, sessions[1]]}
        tabOrder={["terminal:terminal-2", "conversation:chat-1", "terminal:terminal-1"]}
      />,
    );
    expect(strip.scrollLeft).toBe(5);
  });

  it("reveals an oversized tab's activation control and aligns an oversized control at its leading edge", () => {
    positions["terminal:terminal-2"] = { left: 120, right: 300 };
    activation = { left: 120, right: 190 };
    const { strip, rerender } = mount();
    rerender(<QuickChatTabs {...props} activeTerminalTabId="terminal-2" />);
    expect(strip.scrollLeft).toBe(90);
    rerender(<QuickChatTabs {...props} activeTerminalTabId="terminal-1" />);
    activation = { left: 120, right: 280 };
    rerender(<QuickChatTabs {...props} activeTerminalTabId="terminal-2" />);
    expect(strip.scrollLeft).toBe(120);
  });
});
