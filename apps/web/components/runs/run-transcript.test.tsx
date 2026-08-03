import { render, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const mockFetchTaskSession = vi.hoisted(() => vi.fn());

vi.mock("@/lib/api/domains/session-api", () => ({
  fetchTaskSession: (...args: unknown[]) => mockFetchTaskSession(...args),
}));

// Prefetching executors and agent settings is orthogonal to session hydration,
// and it fires network calls this test has no opinion about.
vi.mock("@/hooks/domains/settings/use-settings-data", () => ({
  useSettingsData: () => {},
}));

import { StateProvider } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { RunTranscript } from "./run-transcript";

const SESSION_ID = "session-1";
const TASK_ID = "task-1";

function transcript() {
  return (
    <StateProvider>
      <ToastProvider>
        <TooltipProvider>
          <RunTranscript sessionId={SESSION_ID} taskId={TASK_ID} />
        </TooltipProvider>
      </ToastProvider>
    </StateProvider>
  );
}

beforeEach(() => {
  vi.clearAllMocks();
  mockFetchTaskSession.mockResolvedValue({
    session: { id: SESSION_ID, task_id: TASK_ID, state: "WAITING_FOR_INPUT" },
  });
});

afterEach(() => {
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
});
