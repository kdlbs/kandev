import { cleanup, render as renderReact } from "@testing-library/react";
import type { ReactElement } from "react";
import { StateProvider } from "@/components/state-provider";
import { afterEach, vi } from "vitest";

const sessionMocks = vi.hoisted(() => {
  const startedAt = "2026-08-27T10:00:00Z";
  const updatedAt = "2026-08-27T12:00:00Z";
  return {
    startedAt,
    updatedAt,
    sessionLists: new Map<string, Array<Record<string, unknown>>>(),
    sessionErrors: new Map<string, string | null>(),
    sessionLoaded: new Map<string, boolean>(),
    sessionLoaders: new Map<string, ReturnType<typeof vi.fn>>(),
    useTaskSessions: vi.fn((taskId: string) => {
      let sessions = sessionMocks.sessionLists.get(taskId);
      if (!sessions) {
        sessions = [
          {
            id: `session-${taskId}`,
            task_id: taskId,
            state: "RUNNING" as const,
            is_primary: true,
            started_at: sessionMocks.startedAt,
            updated_at: sessionMocks.updatedAt,
          },
        ];
        sessionMocks.sessionLists.set(taskId, sessions);
      }
      return {
        sessions,
        isLoading: false,
        isLoaded: sessionMocks.sessionLoaded.get(taskId) ?? true,
        error: sessionMocks.sessionErrors.get(taskId) ?? null,
        loadSessions: sessionMocks.sessionLoaders.get(taskId) ?? vi.fn(),
      };
    }),
  };
});

export { sessionMocks };

vi.mock("./thread-conversation", () => ({
  ThreadConversation: ({ sessionId }: { sessionId: string }) => (
    <div data-testid={`thread-conversation-${sessionId}`} />
  ),
}));

vi.mock("@/hooks/use-task-sessions", () => ({
  useTaskSessions: sessionMocks.useTaskSessions,
}));

export function render(element: ReactElement) {
  return renderReact(element, { wrapper: StateProvider });
}

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
  sessionMocks.useTaskSessions.mockClear();
  sessionMocks.sessionLists.clear();
  sessionMocks.sessionErrors.clear();
  sessionMocks.sessionLoaded.clear();
  sessionMocks.sessionLoaders.clear();
});
