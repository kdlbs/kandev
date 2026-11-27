import { afterEach, expect, it, vi } from "vitest";
import * as api from "@/lib/api";
import type { Task, TaskSession } from "@/lib/types/http";
import { resolveTaskRoute } from "./resolve-task-route";

afterEach(() => vi.restoreAllMocks());

it("returns only owned sessions with a matching total and honors the selected conversation", async () => {
  vi.spyOn(api, "fetchTask").mockResolvedValue({
    id: "task",
    primary_session_id: "primary",
  } as Task);
  vi.spyOn(api, "listTaskSessions").mockResolvedValue({
    sessions: [
      { id: "foreign", task_id: "other-task" },
      { id: "primary", task_id: "task" },
      { id: "secondary", task_id: "task" },
    ] as TaskSession[],
    total: 3,
  });
  const resolved = await resolveTaskRoute("task", "secondary");
  expect(resolved.sessionId).toBe("secondary");
  expect(resolved.allSessionsResponse.sessions.map((session) => session.id)).toEqual([
    "primary",
    "secondary",
  ]);
  expect(resolved.allSessionsResponse.total).toBe(2);
});
