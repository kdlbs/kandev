import { act, fireEvent, screen, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { ActiveThread } from "@/lib/threads/active-threads";
import { render, sessionMocks } from "./threads-board.test-helpers";
import { ThreadsBoard } from "./threads-board";

const COLUMN_A = "thread-column-a";
const BOARD_TEST_ID = "threads-board";
const FOCUSED_ATTR = "data-focused";
const COLUMN_B = "thread-column-b";
const CONVERSATION_A = "thread-conversation-session-a";
const TASK_A = "a";
const PRIMARY_SESSION_A = "session-a-primary";
const BUILDER_SESSION_A = "session-a-builder";
const PRIMARY_CONVERSATION_A = `thread-conversation-${PRIMARY_SESSION_A}`;
const BUILDER_CONVERSATION_A = `thread-conversation-${BUILDER_SESSION_A}`;

function thread(overrides: Partial<ActiveThread> & { taskId: string }): ActiveThread {
  return {
    title: `Task ${overrides.taskId}`,
    workflowId: "wf-1",
    workflowName: "Delivery",
    stepTitle: "Build",
    sessionId: `session-${overrides.taskId}`,
    sessionState: "RUNNING",
    pendingAction: null,
    activeSubagentCount: 0,
    queuedPromptCount: 0,
    lastActivityAt: "2026-08-27T10:00:00Z",
    ...overrides,
  };
}

function mockBoardResize() {
  let resize: ResizeObserverCallback = () => {};
  vi.stubGlobal(
    "ResizeObserver",
    class {
      constructor(callback: ResizeObserverCallback) {
        resize = callback;
      }
      observe() {}
      disconnect() {}
    },
  );
  return (width?: number) =>
    act(() =>
      resize(
        [{ contentRect: { height: 700, width } } as ResizeObserverEntry],
        {} as ResizeObserver,
      ),
    );
}

describe("ThreadsBoard — presentation reflow", () => {
  // @covers AC-UI-THREADS-DECK-004.6
  it("reasserts an unretired deep link after width-only reflow", () => {
    const measure = mockBoardResize();
    const scroll = vi.spyOn(Element.prototype, "scrollIntoView").mockImplementation(() => {});
    render(
      <ThreadsBoard
        threads={[thread({ taskId: "a" }), thread({ taskId: "b" })]}
        focusedTaskId="b"
        onOpenTask={() => {}}
      />,
    );
    measure(1100);
    scroll.mockClear();
    measure(800);
    expect(scroll.mock.contexts).toEqual([screen.getByTestId(COLUMN_B)]);

    fireEvent.wheel(screen.getByTestId(COLUMN_A));
    scroll.mockClear();
    measure(700);
    expect(scroll).not.toHaveBeenCalled();
  });
  // @covers AC-UI-THREADS-DECK-004.1, AC-UI-THREADS-DECK-004.2
  it("uses two column-major rows without replacing the task shells", () => {
    const measure = mockBoardResize();
    const threads = ["a", "b", "c"].map((taskId) => thread({ taskId }));
    const scroll = vi.spyOn(Element.prototype, "scrollIntoView").mockImplementation(() => {});
    const view = render(<ThreadsBoard threads={threads} focusedTaskId="b" onOpenTask={() => {}} />);
    const shells = [...screen.getByTestId(BOARD_TEST_ID).children];
    measure();
    view.rerender(
      <ThreadsBoard threads={threads} layout="grid" focusedTaskId="b" onOpenTask={() => {}} />,
    );
    const board = screen.getByTestId(BOARD_TEST_ID);
    expect(board.style.gridTemplateRows).toBe("repeat(2, minmax(0, 1fr))");
    expect([...board.children]).toEqual(shells);
    expect(scroll).toHaveBeenCalledTimes(2);
  });
});

describe("ThreadsBoard — reader recovery", () => {
  it("anchors the wheel-read tile across a layout reflow", () => {
    const measure = mockBoardResize();
    const threads = ["a", "b", "c", "d", "e", "f"].map((taskId) => thread({ taskId }));
    const view = render(<ThreadsBoard threads={threads} onOpenTask={() => {}} />);
    const board = screen.getByTestId(BOARD_TEST_ID);
    vi.spyOn(board, "getBoundingClientRect").mockReturnValue(new DOMRect(0, 0, 600, 700));
    let grid = false;
    threads.forEach(({ taskId }, index) => {
      vi.spyOn(
        screen.getByTestId(`thread-column-${taskId}`),
        "getBoundingClientRect",
      ).mockImplementation(
        () =>
          new DOMRect((grid ? Math.floor(index / 2) : index) * 372 - board.scrollLeft, 0, 360, 300),
      );
    });
    measure(600);
    board.scrollLeft = 1660;
    fireEvent.wheel(screen.getByTestId("thread-column-f"), { deltaY: 100 });
    grid = true;
    view.rerender(<ThreadsBoard threads={threads} layout="grid" onOpenTask={() => {}} />);
    expect(board.scrollLeft).toBe(544);
    expect(screen.getByTestId("thread-column-f").getBoundingClientRect().left).toBe(200);
  });

  it.each([true, false])("honors reduced motion (%s) when revealing a deep link", (reduced) => {
    const originalMatchMedia = window.matchMedia;
    vi.spyOn(window, "matchMedia").mockImplementation((query) =>
      query === "(prefers-reduced-motion: reduce)"
        ? { ...originalMatchMedia(query), matches: reduced }
        : originalMatchMedia(query),
    );
    const scroll = vi.spyOn(Element.prototype, "scrollIntoView").mockImplementation(() => {});
    render(
      <ThreadsBoard threads={[thread({ taskId: "a" })]} focusedTaskId="a" onOpenTask={() => {}} />,
    );
    expect(scroll).toHaveBeenCalledWith({
      inline: "center",
      block: "nearest",
      behavior: reduced ? "instant" : "smooth",
    });
  });
});

describe("ThreadsBoard — basic layout", () => {
  // @covers AC-TASKS-THREADS-ACTIONS-003.2, AC-TASKS-THREADS-ACTIONS-004.6
  it("focuses the successor when the focused column disappears", async () => {
    const view = render(
      <ThreadsBoard
        threads={[thread({ taskId: "a" }), thread({ taskId: "b" }), thread({ taskId: "c" })]}
        onOpenTask={() => {}}
      />,
    );
    const opener = screen
      .getByTestId(COLUMN_B)
      .querySelector<HTMLButtonElement>("[data-thread-task-menu] button")!;
    act(() => opener.focus());
    view.rerender(
      <ThreadsBoard
        threads={[thread({ taskId: "a" }), thread({ taskId: "c" })]}
        onOpenTask={() => {}}
      />,
    );
    await waitFor(() =>
      expect(document.activeElement).toBe(
        screen.getByTestId("thread-column-c").querySelector("[data-thread-task-menu] button"),
      ),
    );
  });
  it("renders one column per active thread, in the order it was given", () => {
    render(
      <ThreadsBoard
        threads={[thread({ taskId: "a" }), thread({ taskId: "b" })]}
        onOpenTask={() => {}}
      />,
    );

    const columns = screen.getAllByTestId(/^thread-column-/);
    expect(columns.map((column) => column.getAttribute("data-testid"))).toEqual([
      COLUMN_A,
      COLUMN_B,
    ]);
  });

  it("mounts the live conversation inside each column", () => {
    render(<ThreadsBoard threads={[thread({ taskId: "a" })]} onOpenTask={() => {}} />);

    expect(screen.getByTestId(CONVERSATION_A)).not.toBeNull();
  });

  it("keeps thirty task shells mounted without mounting thirty conversations", () => {
    const threads = Array.from({ length: 30 }, (_, index) =>
      thread({ taskId: `task-${String(index + 1).padStart(2, "0")}` }),
    );

    render(<ThreadsBoard threads={threads} onOpenTask={() => {}} />);

    expect(screen.getAllByTestId(/^thread-column-/)).toHaveLength(30);
    expect(screen.getAllByTestId(/^thread-conversation-/)).toHaveLength(1);
    const sessionListTaskIds = [
      ...new Set(sessionMocks.useTaskSessions.mock.calls.map(([taskId]) => taskId)),
    ];
    expect(sessionListTaskIds).toEqual(["task-01", "task-02"]);
  });

  it("keeps the snap pager through the mobile breakpoint", () => {
    render(
      <ThreadsBoard
        threads={[thread({ taskId: "a" }), thread({ taskId: "b" })]}
        onOpenTask={() => {}}
      />,
    );

    expect(screen.getByTestId(BOARD_TEST_ID).className).toContain("md:snap-none");
    expect(screen.getByTestId(COLUMN_A).className).toContain("md:w-auto");
    expect(screen.getByTestId(COLUMN_A).className).not.toContain("sm:w-auto");
  });

  it("labels a thread with its task title, workflow and step", () => {
    render(
      <ThreadsBoard
        threads={[thread({ taskId: "a", title: "Fix the flaky test" })]}
        onOpenTask={() => {}}
      />,
    );

    expect(screen.getByText("Fix the flaky test")).not.toBeNull();
    expect(screen.getByText("Delivery")).not.toBeNull();
    expect(screen.getByText("Build")).not.toBeNull();
  });
});

describe("ThreadsBoard — session list loading", () => {
  it("shows a local retry after the initial session list fails, then renders after success", async () => {
    const retry = vi.fn(async () => {
      sessionMocks.sessionErrors.set("a", null);
      sessionMocks.sessionLoaded.set("a", true);
    });
    sessionMocks.sessionErrors.set("a", "service unavailable");
    sessionMocks.sessionLoaded.set("a", false);
    sessionMocks.sessionLoaders.set("a", retry);

    const props = { threads: [thread({ taskId: "a" })], onOpenTask: () => {} };
    const { rerender } = render(<ThreadsBoard {...props} />);

    expect(screen.getByTestId("thread-session-list-error")).not.toBeNull();
    expect(screen.getByRole("button", { name: /retry/i })).not.toBeNull();
    expect(screen.queryByTestId(CONVERSATION_A)).toBeNull();

    fireEvent.click(screen.getByRole("button", { name: /retry/i }));
    await act(async () => {
      await retry.mock.results[0]?.value;
    });
    rerender(<ThreadsBoard {...props} />);

    await waitFor(() => {
      expect(screen.queryByTestId("thread-session-list-error")).toBeNull();
      expect(screen.getByTestId(CONVERSATION_A)).not.toBeNull();
    });
  });
});

describe("ThreadsBoard — session selection", () => {
  it("switches only the selected task column to another session", async () => {
    sessionMocks.sessionLists.set("a", [
      {
        id: PRIMARY_SESSION_A,
        task_id: TASK_A,
        name: "Planner",
        state: "RUNNING",
        is_primary: true,
        started_at: sessionMocks.startedAt,
        updated_at: sessionMocks.updatedAt,
      },
      {
        id: BUILDER_SESSION_A,
        task_id: TASK_A,
        name: "Builder",
        state: "COMPLETED",
        is_primary: false,
        started_at: "2026-08-27T11:00:00Z",
        updated_at: "2026-08-27T13:00:00Z",
      },
    ]);
    render(
      <ThreadsBoard
        threads={[thread({ taskId: TASK_A, sessionId: PRIMARY_SESSION_A })]}
        onOpenTask={() => {}}
      />,
    );

    await waitFor(() => {
      expect(screen.getByTestId(PRIMARY_CONVERSATION_A)).not.toBeNull();
    });
    fireEvent.mouseDown(screen.getByRole("tab", { name: /Builder/ }), { button: 0 });

    expect(screen.queryByTestId(PRIMARY_CONVERSATION_A)).toBeNull();
    expect(screen.getByTestId(BUILDER_CONVERSATION_A)).not.toBeNull();
  });

  it("applies a valid session deep link to the matching task column", async () => {
    sessionMocks.sessionLists.set("a", [
      {
        id: PRIMARY_SESSION_A,
        task_id: TASK_A,
        name: "Planner",
        state: "RUNNING",
        is_primary: true,
        started_at: sessionMocks.startedAt,
        updated_at: sessionMocks.updatedAt,
      },
      {
        id: BUILDER_SESSION_A,
        task_id: TASK_A,
        name: "Builder",
        state: "COMPLETED",
        is_primary: false,
        started_at: "2026-08-27T11:00:00Z",
        updated_at: "2026-08-27T13:00:00Z",
      },
    ]);
    render(
      <ThreadsBoard
        threads={[thread({ taskId: TASK_A, sessionId: PRIMARY_SESSION_A })]}
        focusedTaskId="a"
        focusedSessionId={BUILDER_SESSION_A}
        onOpenTask={() => {}}
      />,
    );

    await waitFor(() => {
      expect(screen.getByTestId(BUILDER_CONVERSATION_A)).not.toBeNull();
    });
    expect(screen.queryByTestId(PRIMARY_CONVERSATION_A)).toBeNull();
  });

  it("reports an invalid session deep link after membership loads and falls back locally", async () => {
    sessionMocks.sessionLists.set("a", [
      {
        id: PRIMARY_SESSION_A,
        task_id: TASK_A,
        state: "RUNNING",
        is_primary: true,
        started_at: sessionMocks.startedAt,
        updated_at: sessionMocks.updatedAt,
      },
    ]);
    const onInvalidRequestedSession = vi.fn();
    render(
      <ThreadsBoard
        threads={[thread({ taskId: TASK_A, sessionId: PRIMARY_SESSION_A })]}
        focusedTaskId="a"
        focusedSessionId="missing-session"
        onInvalidRequestedSession={onInvalidRequestedSession}
        onOpenTask={() => {}}
      />,
    );

    await waitFor(() => {
      expect(onInvalidRequestedSession).toHaveBeenCalledWith(TASK_A, "missing-session");
      expect(screen.getByTestId(PRIMARY_CONVERSATION_A)).not.toBeNull();
    });
  });
});

describe("ThreadsBoard — status, interaction and empty states", () => {
  it("shows plain waiting without presenting it as an agent question", () => {
    sessionMocks.sessionLists.set("a", [
      {
        id: "session-a",
        task_id: "a",
        state: "WAITING_FOR_INPUT",
        is_primary: true,
        started_at: sessionMocks.startedAt,
        updated_at: sessionMocks.updatedAt,
      },
    ]);
    render(
      <ThreadsBoard
        threads={[thread({ taskId: "a", sessionState: "WAITING_FOR_INPUT" })]}
        onOpenTask={() => {}}
      />,
    );

    expect(screen.getByText("Waiting")).not.toBeNull();
    expect(screen.queryByText("Working")).toBeNull();
    expect(screen.queryByTestId("thread-status-clarification")).toBeNull();
  });

  it("shows an explicit question while the session is parked", () => {
    render(
      <ThreadsBoard
        threads={[thread({ taskId: "a", sessionState: "IDLE", pendingAction: "clarification" })]}
        onOpenTask={() => {}}
      />,
    );

    expect(screen.getByText("Question from agent")).not.toBeNull();
  });

  it("counts subagents and queued prompts only when there are some", () => {
    render(
      <ThreadsBoard
        threads={[
          thread({ taskId: "a", activeSubagentCount: 2, queuedPromptCount: 1 }),
          thread({ taskId: "b" }),
        ]}
        onOpenTask={() => {}}
      />,
    );

    expect(screen.getByText("2 subagents")).not.toBeNull();
    expect(screen.getByText("1 queued prompt")).not.toBeNull();
    expect(screen.queryByText("0 subagents")).toBeNull();
  });

  it("opens the full task page from the column header", () => {
    const onOpenTask = vi.fn();
    render(<ThreadsBoard threads={[thread({ taskId: "a" })]} onOpenTask={onOpenTask} />);

    fireEvent.click(screen.getByRole("button", { name: "Open task" }));

    expect(onOpenTask).toHaveBeenCalledWith("a");
  });

  it("explains the empty board instead of showing a bare surface", () => {
    render(<ThreadsBoard threads={[]} onOpenTask={() => {}} />);

    expect(screen.getByTestId("threads-empty-state")).not.toBeNull();
    expect(screen.getByText("No agent is working right now")).not.toBeNull();
    expect(screen.queryByTestId(BOARD_TEST_ID)).toBeNull();
  });

  it("shows a loading state before the first snapshot lands, not the empty state", () => {
    render(<ThreadsBoard threads={[]} isLoading onOpenTask={() => {}} />);

    expect(screen.getByText("Loading threads...")).not.toBeNull();
    expect(screen.queryByTestId("threads-empty-state")).toBeNull();
  });

  it("keeps a landed board visible while a background refresh runs", () => {
    render(<ThreadsBoard threads={[thread({ taskId: "a" })]} isLoading onOpenTask={() => {}} />);

    expect(screen.getByTestId(COLUMN_A)).not.toBeNull();
    expect(screen.queryByText("Loading threads...")).toBeNull();
  });
});

describe("ThreadsBoard — focusing a column from a deep link", () => {
  function scrollSpy() {
    const calls: Element[] = [];
    Element.prototype.scrollIntoView = function scrollIntoViewStub(this: Element) {
      calls.push(this);
    } as Element["scrollIntoView"];
    return calls;
  }

  it("marks the requested column and leaves the others alone", () => {
    scrollSpy();
    render(
      <ThreadsBoard
        threads={[thread({ taskId: "a" }), thread({ taskId: "b" })]}
        focusedTaskId="b"
        onOpenTask={() => {}}
      />,
    );

    expect(screen.getByTestId(COLUMN_B).getAttribute(FOCUSED_ATTR)).toBe("true");
    expect(screen.getByTestId(COLUMN_A).getAttribute(FOCUSED_ATTR)).toBeNull();
  });

  it("scrolls the requested column into view", () => {
    const calls = scrollSpy();
    render(
      <ThreadsBoard
        threads={[thread({ taskId: "a" }), thread({ taskId: "b" })]}
        focusedTaskId="b"
        onOpenTask={() => {}}
      />,
    );

    expect(calls).toEqual([screen.getByTestId(COLUMN_B)]);
  });

  it("scrolls nothing when no column was requested", () => {
    const calls = scrollSpy();
    render(<ThreadsBoard threads={[thread({ taskId: "a" })]} onOpenTask={() => {}} />);

    expect(calls).toEqual([]);
  });

  it("still scrolls when the column only arrives with a later snapshot", () => {
    const calls = scrollSpy();
    const { rerender } = render(
      <ThreadsBoard threads={[thread({ taskId: "a" })]} focusedTaskId="b" onOpenTask={() => {}} />,
    );
    expect(calls).toEqual([]);

    rerender(
      <ThreadsBoard
        threads={[thread({ taskId: "a" }), thread({ taskId: "b" })]}
        focusedTaskId="b"
        onOpenTask={() => {}}
      />,
    );

    expect(calls).toEqual([screen.getByTestId(COLUMN_B)]);
  });

  it("does not re-scroll a column that stays focused across updates", () => {
    const calls = scrollSpy();
    const { rerender } = render(
      <ThreadsBoard threads={[thread({ taskId: "b" })]} focusedTaskId="b" onOpenTask={() => {}} />,
    );
    rerender(
      <ThreadsBoard
        threads={[thread({ taskId: "b", queuedPromptCount: 3 })]}
        focusedTaskId="b"
        onOpenTask={() => {}}
      />,
    );

    expect(calls).toHaveLength(1);
  });
});

// @covers AC-TASKS-THREADS-ACTIONS-003.4
describe("ThreadsBoard initial activation", () => {
  it.each(["pointerDown", "focusIn"] as const)(
    "keeps the requested conversation mounted when %s retires its mark before visibility is ready",
    (interaction) => {
      render(
        <ThreadsBoard
          threads={[thread({ taskId: "a" }), thread({ taskId: "b" })]}
          focusedTaskId="b"
          focusRequestKey="workspace:b:session-b"
          onOpenTask={() => {}}
        />,
      );
      const conversation = screen.getByTestId("thread-conversation-session-b");

      fireEvent[interaction](screen.getByTestId(COLUMN_B));

      expect(screen.getByTestId(COLUMN_B).getAttribute(FOCUSED_ATTR)).toBeNull();
      expect(screen.queryByTestId("thread-conversation-session-b")).toBe(conversation);
    },
  );
});

describe("ThreadsBoard — retiring the deep-link mark", () => {
  // @covers AC-TASKS-THREADS-ACTIONS-003.4, AC-TASKS-THREADS-ACTIONS-003.5
  it("keeps a consumed URL request retired when its excluded column returns", () => {
    const props = { focusRequestKey: "workspace:b:session-b", onOpenTask: () => {} };
    const allThreads = [thread({ taskId: "a" }), thread({ taskId: "b" })];
    const view = render(<ThreadsBoard {...props} threads={allThreads} focusedTaskId="b" />);
    fireEvent.pointerDown(screen.getByTestId(COLUMN_A));
    view.rerender(
      <ThreadsBoard {...props} threads={[thread({ taskId: "a" })]} focusedTaskId={null} />,
    );
    view.rerender(<ThreadsBoard {...props} threads={allThreads} focusedTaskId="b" />);

    expect(screen.getByTestId(COLUMN_B).getAttribute(FOCUSED_ATTR)).toBeNull();
    expect(screen.queryByTestId(CONVERSATION_A)).not.toBeNull();
    expect(screen.queryByTestId("thread-conversation-session-b")).toBeNull();
  });

  it("honors a new URL request after a consumed request loses its resolved column", () => {
    const props = { onOpenTask: () => {} };
    const view = render(
      <ThreadsBoard
        {...props}
        threads={[thread({ taskId: "a" }), thread({ taskId: "b" })]}
        focusedTaskId="b"
        focusRequestKey="workspace:b:session-b"
      />,
    );
    fireEvent.pointerDown(screen.getByTestId(COLUMN_A));
    view.rerender(
      <ThreadsBoard
        {...props}
        threads={[thread({ taskId: "a" })]}
        focusedTaskId={null}
        focusRequestKey="workspace:b:session-b"
      />,
    );
    view.rerender(
      <ThreadsBoard
        {...props}
        threads={[thread({ taskId: "a" })]}
        focusedTaskId="a"
        focusRequestKey="workspace:a:session-a"
      />,
    );
    expect(screen.getByTestId(COLUMN_A).getAttribute(FOCUSED_ATTR)).toBe("true");
  });

  it.each(["pointerDown", "wheel"] as const)("drops the mark on reader %s", (event) => {
    render(
      <ThreadsBoard
        threads={[thread({ taskId: "a" }), thread({ taskId: "b" })]}
        focusedTaskId="b"
        onOpenTask={() => {}}
      />,
    );
    expect(screen.getByTestId(COLUMN_B).getAttribute(FOCUSED_ATTR)).toBe("true");

    fireEvent[event](screen.getByTestId(COLUMN_A));

    expect(screen.getByTestId(COLUMN_B).getAttribute(FOCUSED_ATTR)).toBeNull();
  });

  it("drops the mark when focus lands anywhere in the deck", () => {
    render(
      <ThreadsBoard threads={[thread({ taskId: "b" })]} focusedTaskId="b" onOpenTask={() => {}} />,
    );

    fireEvent.focusIn(screen.getByTestId(COLUMN_B));

    expect(screen.getByTestId(COLUMN_B).getAttribute(FOCUSED_ATTR)).toBeNull();
  });

  it("marks again when a later deep link asks for another column", () => {
    const { rerender } = render(
      <ThreadsBoard
        threads={[thread({ taskId: "a" }), thread({ taskId: "b" })]}
        focusedTaskId="b"
        onOpenTask={() => {}}
      />,
    );
    fireEvent.pointerDown(screen.getByTestId(COLUMN_A));
    expect(screen.getByTestId(COLUMN_B).getAttribute(FOCUSED_ATTR)).toBeNull();

    rerender(
      <ThreadsBoard
        threads={[thread({ taskId: "a" }), thread({ taskId: "b" })]}
        focusedTaskId="a"
        onOpenTask={() => {}}
      />,
    );

    expect(screen.getByTestId(COLUMN_A).getAttribute(FOCUSED_ATTR)).toBe("true");
  });

  it("keeps the mark retired while the same deep link stays in the URL", () => {
    const { rerender } = render(
      <ThreadsBoard threads={[thread({ taskId: "b" })]} focusedTaskId="b" onOpenTask={() => {}} />,
    );
    fireEvent.pointerDown(screen.getByTestId(COLUMN_B));

    rerender(
      <ThreadsBoard
        threads={[thread({ taskId: "b", queuedPromptCount: 2 })]}
        focusedTaskId="b"
        onOpenTask={() => {}}
      />,
    );

    expect(screen.getByTestId(COLUMN_B).getAttribute(FOCUSED_ATTR)).toBeNull();
  });
});
