import { renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

type MockStoreState = {
  tasks: { activeSessionId: string | null };
  environmentIdBySessionId: Record<string, string>;
};

let mockStoreState: MockStoreState;

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: MockStoreState) => unknown) => selector(mockStoreState),
}));

import { useEnvironmentSessionId } from "./use-environment-session-id";

const ENVIRONMENT_ID = "environment-shared";
const FIRST_SESSION_ID = "session-first";
const SECOND_SESSION_ID = "session-second";

function setActiveSession(activeSessionId: string, sessionIds: string[]) {
  mockStoreState = {
    tasks: { activeSessionId },
    environmentIdBySessionId: Object.fromEntries(
      sessionIds.map((sessionId) => [sessionId, ENVIRONMENT_ID]),
    ),
  };
}

beforeEach(() => {
  setActiveSession(FIRST_SESSION_ID, [FIRST_SESSION_ID, SECOND_SESSION_ID]);
});

describe("useEnvironmentSessionId", () => {
  // @covers AC-TASKS-SESSION-DELETE-RESOURCE-CLEANUP-001.9
  it("promotes the active same-environment session after the cached session is purged", () => {
    const { result, rerender } = renderHook(() => useEnvironmentSessionId());

    expect(result.current).toBe(FIRST_SESSION_ID);

    setActiveSession(SECOND_SESSION_ID, [FIRST_SESSION_ID, SECOND_SESSION_ID]);
    rerender();
    expect(result.current).toBe(FIRST_SESSION_ID);

    setActiveSession(SECOND_SESSION_ID, [SECOND_SESSION_ID]);
    rerender();

    expect(result.current).toBe(SECOND_SESSION_ID);
  });

  it("keeps a live lookup handle stable across same-environment session switches", () => {
    const { result, rerender } = renderHook(() => useEnvironmentSessionId());

    setActiveSession(SECOND_SESSION_ID, [FIRST_SESSION_ID, SECOND_SESSION_ID]);
    rerender();

    expect(result.current).toBe(FIRST_SESSION_ID);
  });

  it("uses the active session when the environment changes", () => {
    const { result, rerender } = renderHook(() => useEnvironmentSessionId());

    mockStoreState = {
      tasks: { activeSessionId: SECOND_SESSION_ID },
      environmentIdBySessionId: {
        [FIRST_SESSION_ID]: ENVIRONMENT_ID,
        [SECOND_SESSION_ID]: "environment-second",
      },
    };
    rerender();

    expect(result.current).toBe(SECOND_SESSION_ID);
  });

  it("uses the active session as the pre-hydration lookup handle", () => {
    setActiveSession(FIRST_SESSION_ID, []);

    const { result } = renderHook(() => useEnvironmentSessionId());

    expect(result.current).toBe(FIRST_SESSION_ID);
  });
});
