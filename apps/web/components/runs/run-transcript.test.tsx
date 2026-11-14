import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const mockFetchTaskSession = vi.hoisted(() => vi.fn());
const mockListTaskSessions = vi.hoisted(() => vi.fn());

vi.mock("@/lib/api/domains/session-api", () => ({
  fetchTaskSession: (...args: unknown[]) => mockFetchTaskSession(...args),
}));

vi.mock("@/lib/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/api")>()),
  listTaskSessions: (...args: unknown[]) => mockListTaskSessions(...args),
}));

// Prefetching executors and agent settings is orthogonal to session hydration,
// and it fires network calls this test has no opinion about.
vi.mock("@/hooks/domains/settings/use-settings-data", () => ({
  useSettingsData: () => {},
}));

import { StateProvider } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { focusRunTranscriptTurn, RunTranscript } from "./run-transcript";

const SESSION_ID = "session-1";
const TASK_ID = "task-1";
const OLD_TURN_ID = "turn-old";
const CURRENT_TURN_ID = "turn-current";
const CAPTURE_TITLE = "CLI passthrough output is not captured in this run's transcript.";
const CAPTURE_DESCRIPTION =
  "Disable CLI passthrough in the automation's agent profile to record output for future sessions. Previous terminal output cannot be recovered here.";
const CHAT_INPUT_AREA_TEST_ID = "chat-input-area";

function transcript(sessionId = SESSION_ID, taskId = TASK_ID) {
  return (
    <StateProvider>
      <ToastProvider>
        <TooltipProvider>
          <RunTranscript sessionId={sessionId} taskId={taskId} />
        </TooltipProvider>
      </ToastProvider>
    </StateProvider>
  );
}

function sessionInState(state: string) {
  return { session: { id: SESSION_ID, task_id: TASK_ID, state } };
}

beforeEach(() => {
  vi.clearAllMocks();
  mockListTaskSessions.mockResolvedValue({ sessions: [] });
  mockFetchTaskSession.mockResolvedValue({
    session: { id: SESSION_ID, task_id: TASK_ID, state: "WAITING_FOR_INPUT" },
  });
});

afterEach(() => {
  // Each case renders the whole chat stack; without this the trees stack up and
  // a by-test-id lookup starts matching the previous case's composer.
  cleanup();
  vi.restoreAllMocks();
});

describe("RunTranscript session hydration", () => {
  // Automation tasks are hidden from the boot payload by their origin, so a
  // direct load of /automations/<id> — a shared link, or a reload — arrives
  // with no session row in the store. Without hydration `useSession` never
  // subscribes and the composer rejects every reply as session-unavailable,
  // which is the one thing this surface exists to allow.
  //
  // Mounted against a real store with `useChatPanelState` left unmocked, so it
  // exercises the state a fresh navigation actually produces rather than a
  // stand-in for it.
  it("fetches the session the store was never given", async () => {
    render(transcript());

    await waitFor(() => expect(mockFetchTaskSession).toHaveBeenCalledWith(SESSION_ID));
  });

  it("asks only once, however many times the panel re-renders", async () => {
    const { rerender } = render(transcript());

    await waitFor(() => expect(mockFetchTaskSession).toHaveBeenCalledTimes(1));
    rerender(transcript());

    expect(mockFetchTaskSession).toHaveBeenCalledTimes(1);
  });

  it.each(["RUNNING", "WAITING_FOR_INPUT"])(
    "explains uncaptured output for a %s passthrough run",
    async (state) => {
      mockFetchTaskSession.mockResolvedValue({
        session: {
          id: SESSION_ID,
          task_id: TASK_ID,
          state,
          is_passthrough: true,
          agent_profile_snapshot: { cli_passthrough: false },
        },
      });

      render(transcript());

      expect(await screen.findByText(CAPTURE_TITLE)).toBeTruthy();
      expect(screen.getByText(CAPTURE_DESCRIPTION)).toBeTruthy();
      expect(await screen.findByTestId(CHAT_INPUT_AREA_TEST_ID)).toBeTruthy();
    },
  );

  it("follows the selected session when moving from passthrough to structured", async () => {
    const structuredSessionId = "session-structured";
    mockFetchTaskSession.mockImplementation((sessionId: string) =>
      Promise.resolve({
        session: {
          id: sessionId,
          task_id: TASK_ID,
          state: "WAITING_FOR_INPUT",
          is_passthrough: sessionId === SESSION_ID,
        },
      }),
    );
    const { rerender } = render(transcript());

    expect(await screen.findByText(CAPTURE_TITLE)).toBeTruthy();

    rerender(transcript(structuredSessionId));

    await waitFor(() => expect(screen.queryByText(CAPTURE_TITLE)).toBeNull());
  });
});

describe("RunTranscript turn selection", () => {
  it("focuses the selected turn without removing surrounding transcript rows", () => {
    const root = document.createElement("div");
    const previous = document.createElement("div");
    previous.dataset.turnId = OLD_TURN_ID;
    const selected = document.createElement("div");
    selected.dataset.turnId = CURRENT_TURN_ID;
    selected.scrollIntoView = vi.fn();
    root.append(previous, selected);
    document.body.append(root);

    expect(focusRunTranscriptTurn(root, CURRENT_TURN_ID)).toBe(true);
    expect(root.querySelectorAll("[data-turn-id]")).toHaveLength(2);
    expect(document.activeElement).toBe(selected);
    expect(selected.scrollIntoView).toHaveBeenCalledWith({ behavior: "auto", block: "center" });
    root.remove();
  });
});

describe("RunTranscript live-session controls", () => {
  // Replying to a run starts the agent, it works the prompt, and it shuts down
  // again. Controls that speak to a live ACP session — the model picker above
  // all — must not outlive the turn: they still look operable, and changing a
  // model on a process that no longer exists silently does nothing.
  it("offers the model selector while a turn is running", async () => {
    mockFetchTaskSession.mockResolvedValue(sessionInState("RUNNING"));

    render(transcript());

    await waitFor(() => expect(screen.getByTestId("toolbar-item-model")).toBeTruthy());
  });

  it("unmounts the model selector once the turn ends and the run parks", async () => {
    // WAITING_FOR_INPUT is the state a finished automation run parks in — it is
    // what keeps the run repliable, and it is exactly the state with no agent
    // behind it.
    mockFetchTaskSession.mockResolvedValue(sessionInState("WAITING_FOR_INPUT"));

    render(transcript());

    await waitFor(() => expect(screen.getByTestId(CHAT_INPUT_AREA_TEST_ID)).toBeTruthy());
    expect(screen.queryByTestId("toolbar-item-model")).toBeNull();
  });

  it("keeps the composer itself, because the run is still repliable", async () => {
    mockFetchTaskSession.mockResolvedValue(sessionInState("WAITING_FOR_INPUT"));

    render(transcript());

    await waitFor(() => expect(screen.getByTestId(CHAT_INPUT_AREA_TEST_ID)).toBeTruthy());
    expect(screen.getByTestId("chat-input-toolbar")).toBeTruthy();
  });

  it("draws the composer on the page's own background, not the workbench plate", async () => {
    // A lighter strip along the bottom of a `bg-background` page read as a
    // panel that had been left behind.
    mockFetchTaskSession.mockResolvedValue(sessionInState("WAITING_FOR_INPUT"));

    render(transcript());

    const area = await screen.findByTestId(CHAT_INPUT_AREA_TEST_ID);
    expect(area.className).toContain("bg-background");
    expect(area.className).not.toContain("bg-card");
  });
});
