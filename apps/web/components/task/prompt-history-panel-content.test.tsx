import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import i18n from "i18next";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Message, Turn } from "@/lib/types/http";

const { state, messagesBySession, turnsBySession, turnsHydratedBySession } = vi.hoisted(() => ({
  state: {
    tasks: { activeSessionId: "session-a" as string | null },
    taskSessions: {
      items: {} as Record<string, { name?: string; is_passthrough?: boolean }>,
    },
  },
  messagesBySession: {} as Record<string, Message[]>,
  turnsBySession: {} as Record<string, Turn[]>,
  turnsHydratedBySession: {} as Record<string, boolean>,
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (value: typeof state) => unknown) => selector(state),
}));

vi.mock("@/hooks/domains/session/use-session-messages", () => ({
  useSessionMessages: (sessionId: string | null) => ({
    messages: sessionId ? (messagesBySession[sessionId] ?? []) : [],
  }),
}));

vi.mock("@/hooks/domains/session/use-session-turns", () => ({
  useSessionTurns: (sessionId: string | null) =>
    sessionId ? (turnsBySession[sessionId] ?? []) : [],
  useSessionTurnsState: (sessionId: string | null) => ({
    turns: sessionId ? (turnsBySession[sessionId] ?? []) : [],
    isHydrated: sessionId ? (turnsHydratedBySession[sessionId] ?? true) : true,
  }),
}));

import { PromptHistoryPanelContent } from "./prompt-history-panel-content";
import { useMessageFavoritesStore } from "@/lib/state/slices/message-favorites";
import { formatDateTime } from "@/lib/i18n/formats";

const SESSION_A = "session-a";
const SESSION_B = "session-b";
const PASSTHROUGH = "session-passthrough";
const BASE_TIME = "2026-01-01T12:00:00.000Z";
const COMPLETED_5S = "2026-01-01T12:00:05.000Z";
const LATER_TIME = "2026-01-01T12:05:00.000Z";
const NEWER_PROMPT = "newer prompt";
const OLDER_PROMPT = "older prompt";
const OLD_PROMPT = "old prompt";
const MID_PROMPT = "mid prompt";
const NEW_PROMPT = "new prompt";
const TURN_ID = "turn-1";
const OVERFLOW_SCROLL_WIDTH = 600;
const EXPAND_TEST_ID = "prompt-history-expand-0";
const PANEL_TEST_ID = "prompt-history-panel";
const COLLAPSED_WIDTH = 100;
const SPAN_NOT_RENDERED = "text span did not render";

type ObserverEntry = { element: Element; callback: ResizeObserverCallback };
const observerEntries: ObserverEntry[] = [];

class CapturingResizeObserver {
  private readonly callback: ResizeObserverCallback;

  /** Stores the ResizeObserver callback for later manual invocation. */
  constructor(callback: ResizeObserverCallback) {
    this.callback = callback;
  }

  /** Records the observed element and its callback for later manual resize firing. */
  observe(element: Element) {
    observerEntries.push({ element, callback: this.callback });
  }

  /** No-op: recorded observations are retained so tests can fire them manually. */
  disconnect() {}
  /** No-op: recorded observations are retained so tests can fire them manually. */
  unobserve() {}
}

/** Builds a user Message for the active session with default fields, merged with the given overrides. */
function message(overrides: Partial<Message> = {}): Message {
  return {
    id: "message-1",
    session_id: SESSION_A as Message["session_id"],
    task_id: "task-1" as Message["task_id"],
    author_type: "user",
    content: "A prompt that is rendered in history",
    type: "message",
    created_at: BASE_TIME,
    ...overrides,
  };
}

/** Builds a Turn for the active session with default fields, merged with the given overrides. */
function turn(overrides: Partial<Turn> = {}): Turn {
  return {
    id: "turn-1",
    session_id: SESSION_A as Turn["session_id"],
    task_id: "task-1" as Turn["task_id"],
    started_at: BASE_TIME,
    created_at: BASE_TIME,
    updated_at: BASE_TIME,
    ...overrides,
  };
}

/** Stubs the given scrollWidth/clientWidth/clientHeight values onto an element. */
function setGeometry(
  element: Element,
  overrides: { scrollWidth?: number; clientWidth?: number; clientHeight?: number },
) {
  if (overrides.scrollWidth !== undefined) {
    Object.defineProperty(element, "scrollWidth", {
      configurable: true,
      value: overrides.scrollWidth,
    });
  }
  if (overrides.clientWidth !== undefined) {
    Object.defineProperty(element, "clientWidth", {
      configurable: true,
      value: overrides.clientWidth,
    });
  }
  if (overrides.clientHeight !== undefined) {
    Object.defineProperty(element, "clientHeight", {
      configurable: true,
      value: overrides.clientHeight,
    });
  }
}

/** Fires the captured ResizeObserver callback recorded for the given element. */
function fireResize(element: Element) {
  for (const entry of observerEntries) {
    if (entry.element === element) {
      // The component callbacks ignore both arguments; pass a stub observer
      // so the ResizeObserverCallback typing is satisfied.
      act(() => entry.callback([], {} as ResizeObserver));
    }
  }
}

/** Returns the prompt-history row element for the given row index. */
function row(index: number): HTMLElement {
  return screen.getByTestId(`prompt-history-row-${index}`);
}

/** Returns the expanded-content box element for the given row index. */
function expandedBox(index: number): HTMLElement {
  return screen.getByTestId(`prompt-history-expanded-box-${index}`);
}

/** Returns the expand/collapse chevron button element for the given row index. */
function expandButton(index: number): HTMLElement {
  return screen.getByTestId(`prompt-history-expand-${index}`);
}

beforeEach(() => {
  vi.clearAllMocks();
  observerEntries.length = 0;
  vi.stubGlobal("ResizeObserver", CapturingResizeObserver);
  state.tasks.activeSessionId = SESSION_A;
  state.taskSessions.items = {
    [SESSION_A]: { name: "Agent A", is_passthrough: false },
    [SESSION_B]: { name: "Agent B", is_passthrough: false },
    [PASSTHROUGH]: { name: "PT", is_passthrough: true },
  };
  messagesBySession[SESSION_A] = [];
  messagesBySession[SESSION_B] = [];
  turnsBySession[SESSION_A] = [];
  turnsBySession[SESSION_B] = [];
  turnsHydratedBySession[SESSION_A] = true;
  turnsHydratedBySession[SESSION_B] = true;
});

afterEach(async () => {
  cleanup();
  vi.unstubAllGlobals();
  useMessageFavoritesStore.setState({ bySession: {} });
  sessionStorage.clear();
  await i18n.changeLanguage("en");
});

describe("PromptHistoryPanelContent — rows and test IDs", () => {
  it("renders newest-first rows with the stable test IDs", () => {
    messagesBySession[SESSION_A] = [
      message({ id: "older", content: OLDER_PROMPT }),
      message({ id: "newer", content: NEWER_PROMPT, created_at: LATER_TIME }),
    ];

    render(<PromptHistoryPanelContent />);

    expect(screen.getByTestId(PANEL_TEST_ID)).toBeTruthy();
    expect(row(0).textContent).toContain(NEWER_PROMPT);
    expect(row(1).textContent).toContain(OLDER_PROMPT);
    expect(row(0).querySelector(".cursor-pointer")).toBeTruthy();
    expect(row(1).querySelector(".cursor-pointer")).toBeTruthy();
  });

  it("styles the prompt like the transcript user bubble with lighter padding", () => {
    messagesBySession[SESSION_A] = [message({ content: "bubbled prompt" })];

    render(<PromptHistoryPanelContent />);

    const bubble = row(0).querySelector(".rounded-2xl");
    expect(bubble).toBeTruthy();
    expect(bubble?.classList.contains("bg-primary/30")).toBe(true);
    expect(bubble?.classList.contains("px-3")).toBe(true);
    expect(bubble?.classList.contains("py-1.5")).toBe(true);
    // Same font as the transcript user message (markdown-body 13px).
    expect(bubble?.classList.contains("markdown-body")).toBe(true);
    expect(bubble?.classList.contains("markdown-body-user")).toBe(true);
    expect(bubble?.textContent).toContain("bubbled prompt");
  });
  it("starts agent-owned prompts with the robot icon used by the transcript header", () => {
    messagesBySession[SESSION_A] = [
      message({
        metadata: {
          sender_task_id: "sender-task",
          sender_task_title: "Sending task",
          sender_session_id: "sender-session",
        },
      }),
    ];

    render(<PromptHistoryPanelContent />);

    const bubble = row(0).querySelector(".rounded-2xl");
    const robot = bubble?.querySelector("svg.tabler-icon-robot");
    expect(robot).toBeTruthy();
    expect(bubble?.firstElementChild).toBe(robot);
  });

  it("turns the bubble yellow when the transcript message is starred", () => {
    useMessageFavoritesStore.getState().toggleFavorite(SESSION_A, "message-1");
    messagesBySession[SESSION_A] = [message({ id: "message-1", content: "starred prompt" })];

    render(<PromptHistoryPanelContent />);

    const bubble = row(0).querySelector(".rounded-2xl");
    expect(bubble?.classList.contains("bg-yellow-200/50")).toBe(true);
    expect(bubble?.classList.contains("bg-primary/30")).toBe(false);
  });

  it("renders the empty state when the session has no user prompts", () => {
    render(<PromptHistoryPanelContent />);

    expect(screen.getByTestId(PANEL_TEST_ID).textContent).toBe("No prompts yet.");
    expect(screen.queryByTestId(/^prompt-history-row-/)).toBeNull();
  });

  it("renders the empty state for a passthrough active session with zero arrows", () => {
    messagesBySession[PASSTHROUGH] = [
      message({ session_id: PASSTHROUGH as Message["session_id"] }),
    ];
    state.tasks.activeSessionId = PASSTHROUGH;

    render(<PromptHistoryPanelContent />);

    expect(screen.getByTestId(PANEL_TEST_ID).textContent).toBe("No prompts yet.");
    expect(screen.queryByTestId(/^prompt-history-row-/)).toBeNull();
  });
});

describe("PromptHistoryPanelContent — navigation seam", () => {
  it("invokes the injected callback when the prompt row is clicked", () => {
    messagesBySession[SESSION_A] = [message({ id: "prompt-1" })];
    const onNavigateToPrompt = vi.fn();

    render(<PromptHistoryPanelContent onNavigateToPrompt={onNavigateToPrompt} />);
    const prompt = row(0).querySelector<HTMLElement>(".cursor-pointer");
    expect(prompt).toBeTruthy();
    fireEvent.click(prompt!);

    expect(onNavigateToPrompt).toHaveBeenCalledWith("prompt-1");
  });

  it("does nothing when the callback is absent", () => {
    messagesBySession[SESSION_A] = [message()];

    render(<PromptHistoryPanelContent />);
    fireEvent.click(screen.getByTestId("prompt-history-row-0"));
  });
});
describe("PromptHistoryPanelContent — active-session switch", () => {
  it("re-derives rows for the new active session and shows the empty state for null", () => {
    messagesBySession[SESSION_A] = [message({ id: "a-prompt", content: "A session prompt" })];
    messagesBySession[SESSION_B] = [
      message({
        id: "b-prompt",
        content: "B session prompt",
        session_id: SESSION_B as Message["session_id"],
      }),
    ];

    const { rerender } = render(<PromptHistoryPanelContent />);
    expect(screen.getByTestId("prompt-history-row-0").textContent).toContain("A session prompt");
    expect(screen.queryByText("B session prompt")).toBeNull();

    state.tasks.activeSessionId = SESSION_B;
    rerender(<PromptHistoryPanelContent />);

    expect(screen.getByTestId("prompt-history-row-0").textContent).toContain("B session prompt");
    expect(screen.queryByText("A session prompt")).toBeNull();

    state.tasks.activeSessionId = null;
    rerender(<PromptHistoryPanelContent />);

    expect(screen.getByTestId(PANEL_TEST_ID).textContent).toBe("No prompts yet.");
  });
});

describe("PromptHistoryPanelContent — duration rendering", () => {
  it("hides provisional durations while turn hydration is in flight", () => {
    messagesBySession[SESSION_A] = [
      message({ id: "older", turn_id: TURN_ID }),
      message({ id: "newer", created_at: LATER_TIME }),
    ];
    turnsBySession[SESSION_A] = [];
    turnsHydratedBySession[SESSION_A] = false;

    const { rerender } = render(<PromptHistoryPanelContent />);

    expect(screen.queryByTestId(/^prompt-history-duration-/)).toBeNull();

    turnsHydratedBySession[SESSION_A] = true;
    turnsBySession[SESSION_A] = [turn({ completed_at: COMPLETED_5S })];
    rerender(<PromptHistoryPanelContent />);

    expect(screen.getByTestId("prompt-history-duration-1").textContent).toBe("5s");
  });

  it("composes the exact formatPromptDuration output from the localized units", () => {
    messagesBySession[SESSION_A] = [message({ turn_id: TURN_ID })];
    turnsBySession[SESSION_A] = [turn({ completed_at: COMPLETED_5S })];

    render(<PromptHistoryPanelContent />);

    expect(screen.getByTestId("prompt-history-duration-0").textContent).toBe("5s");
  });

  it("renders a 0s duration for a sub-second completed turn instead of hiding it", () => {
    messagesBySession[SESSION_A] = [message({ turn_id: TURN_ID })];
    turnsBySession[SESSION_A] = [turn({ completed_at: BASE_TIME })];

    render(<PromptHistoryPanelContent />);

    expect(screen.getByTestId("prompt-history-duration-0").textContent).toBe("0s");
  });

  it("omits the duration element for a running prompt", () => {
    messagesBySession[SESSION_A] = [message({ turn_id: TURN_ID })];
    turnsBySession[SESSION_A] = [turn({})];

    render(<PromptHistoryPanelContent />);

    expect(screen.queryByTestId(/^prompt-history-duration-/)).toBeNull();
  });

  it("composes durations from pseudo-locale unit labels, proving the catalog path", async () => {
    messagesBySession[SESSION_A] = [message({ turn_id: TURN_ID })];
    turnsBySession[SESSION_A] = [turn({ completed_at: COMPLETED_5S })];

    await i18n.changeLanguage("pseudo");
    render(<PromptHistoryPanelContent />);

    const text = screen.getByTestId("prompt-history-duration-0").textContent ?? "";
    expect(text).toMatch(/[^\x20-\x7E]/);
    expect(text.startsWith("5")).toBe(true);
  });
});

describe("PromptHistoryPanelContent — time element", () => {
  it.each([
    { name: "just now", now: "2026-01-01T12:00:30.000Z", expected: "just now" },
    { name: "5m", now: "2026-01-01T12:05:00.000Z", expected: "5m" },
    { name: "3h", now: "2026-01-01T15:00:00.000Z", expected: "3h" },
    { name: "2d", now: "2026-01-03T12:00:00.000Z", expected: "2d" },
  ])("renders the $name bucket of the compact formatRelative ladder", ({ now, expected }) => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date(now));
    try {
      messagesBySession[SESSION_A] = [message()];

      render(<PromptHistoryPanelContent />);

      const time = row(0).querySelector("time");
      expect(time?.textContent).toBe(expected);
    } finally {
      vi.useRealTimers();
    }
  });

  it("renders dateTime and an absolute title on the time element", () => {
    messagesBySession[SESSION_A] = [message()];

    render(<PromptHistoryPanelContent />);

    const time = row(0).querySelector("time");
    expect(time?.getAttribute("dateTime")).toBe(BASE_TIME);
    expect(time?.getAttribute("title")).toBe(formatDateTime(BASE_TIME));
  });
});

describe("PromptHistoryPanelContent — expand/collapse behavior", () => {
  /** Renders a row whose collapsed text overflows, firing the resize that reveals the chevron. */
  function renderOverflowingRow() {
    messagesBySession[SESSION_A] = [message({ id: "long", content: "long prompt text" })];
    render(<PromptHistoryPanelContent />);
    const text = row(0).querySelector("span");
    if (!text) throw new Error(SPAN_NOT_RENDERED);
    setGeometry(text, { scrollWidth: OVERFLOW_SCROLL_WIDTH, clientWidth: COLLAPSED_WIDTH });
    fireResize(text);
    return text;
  }

  it("shows the chevron on hover when the collapsed text overflows", () => {
    renderOverflowingRow();
    expect(expandButton(0).className).toContain("group-hover:opacity-100");
    const text = row(0).querySelector("span");
    if (!text) throw new Error(SPAN_NOT_RENDERED);
    setGeometry(text, { scrollWidth: 60 });
    fireResize(text);

    expect(screen.queryByTestId(EXPAND_TEST_ID)).toBeNull();
  });

  it("keeps the chevron visible while expanded and collapses on click", () => {
    renderOverflowingRow();

    expect(expandButton(0).getAttribute("aria-label")).toBe("Expand prompt");
    expect(expandButton(0).getAttribute("aria-expanded")).toBe("false");

    fireEvent.click(expandButton(0));
    expect(expandButton(0).getAttribute("aria-expanded")).toBe("true");
    expect(expandButton(0).getAttribute("aria-label")).toBe("Collapse prompt");

    // Expanded text wraps and no longer overflows horizontally, but the
    // chevron must stay visible (a literal "only when overflow" gate fails).
    const text = row(0).querySelector("span");
    if (!text) throw new Error(SPAN_NOT_RENDERED);
    setGeometry(text, { scrollWidth: 60 });
    fireResize(text);
    expect(expandButton(0)).toBeTruthy();

    fireEvent.click(expandButton(0));
    // Collapsed with no overflow: both the chevron and the expanded box go.
    expect(screen.queryByTestId(EXPAND_TEST_ID)).toBeNull();
    expect(screen.queryByTestId("prompt-history-expanded-box-0")).toBeNull();
  });

  it("renders the expanded content once inside the capped box with wrapping text", () => {
    renderOverflowingRow();

    fireEvent.click(expandButton(0));

    // The collapsed span is hidden while expanded; the full content renders
    // exactly once — inside the expanded box, not duplicated beside it.
    expect(row(0).querySelector("span")?.className).toContain("hidden");
    const box = expandedBox(0);
    expect(box.className).toContain("overflow-y-auto");
    expect(box.className).toContain("whitespace-normal");
    expect(box.textContent).toBe("long prompt text");
    // The only other copy is the collapsed span, which is hidden while
    // expanded — jsdom does not apply stylesheet classes, so filter by the
    // component's own `hidden` class instead of computed layout.
    const nonHidden = screen
      .getAllByText("long prompt text")
      .filter((el) => !(el as HTMLElement).className.includes("hidden"));
    expect(nonHidden).toHaveLength(1);
    expect(nonHidden[0]).toBe(box);
  });

  it("caps the expanded box at 40% of the panel root clientHeight and remeasures", () => {
    const { rerender } = render(<PromptHistoryPanelContent />);
    messagesBySession[SESSION_A] = [message({ id: "long" })];
    rerender(<PromptHistoryPanelContent />);

    const root = screen.getByTestId(PANEL_TEST_ID);
    setGeometry(root, { clientHeight: 1000 });
    fireResize(root);

    // Force the chevron via overflow so the row can be expanded.
    const text = row(0).querySelector("span");
    if (!text) throw new Error(SPAN_NOT_RENDERED);
    setGeometry(text, { scrollWidth: OVERFLOW_SCROLL_WIDTH, clientWidth: COLLAPSED_WIDTH });
    fireResize(text);
    fireEvent.click(expandButton(0));

    expect(expandedBox(0).style.maxHeight).toBe("400px");

    // Root remeasure: a second measurement must replace the first.
    setGeometry(root, { clientHeight: 2000 });
    fireResize(root);
    expect(expandedBox(0).style.maxHeight).toBe("800px");
  });

  it("falls back to 40vh when the root has no measurable height", () => {
    messagesBySession[SESSION_A] = [message({ id: "long" })];
    render(<PromptHistoryPanelContent />);

    const text = row(0).querySelector("span");
    if (!text) throw new Error(SPAN_NOT_RENDERED);
    setGeometry(text, { scrollWidth: OVERFLOW_SCROLL_WIDTH, clientWidth: COLLAPSED_WIDTH });
    fireResize(text);
    fireEvent.click(expandButton(0));

    expect(expandedBox(0).style.maxHeight).toBe("40vh");
  });

  it("keys expansion by messageId, not row index", () => {
    messagesBySession[SESSION_A] = [
      message({ id: "old", content: OLD_PROMPT }),
      message({ id: "mid", content: MID_PROMPT, created_at: LATER_TIME }),
    ];
    const { rerender } = render(<PromptHistoryPanelContent />);

    // Expand "mid" (newest-first index 0).
    const midText = row(0).querySelector("span");
    if (!midText) throw new Error(SPAN_NOT_RENDERED);
    setGeometry(midText, { scrollWidth: OVERFLOW_SCROLL_WIDTH, clientWidth: COLLAPSED_WIDTH });
    fireResize(midText);
    fireEvent.click(expandButton(0));
    expect(expandedBox(0).textContent).toBe(MID_PROMPT);

    // Prepend a newer prompt: "mid" moves to index 1 and stays expanded.
    messagesBySession[SESSION_A] = [
      message({ id: "new", content: NEW_PROMPT, created_at: "2026-01-01T12:10:00.000Z" }),
      message({ id: "mid", content: MID_PROMPT, created_at: LATER_TIME }),
      message({ id: "old", content: OLD_PROMPT }),
    ];
    rerender(<PromptHistoryPanelContent />);

    expect(screen.queryByTestId("prompt-history-expanded-box-0")).toBeNull();
    expect(expandedBox(1).textContent).toBe(MID_PROMPT);
    expect(expandButton(1).getAttribute("aria-expanded")).toBe("true");
  });
});
