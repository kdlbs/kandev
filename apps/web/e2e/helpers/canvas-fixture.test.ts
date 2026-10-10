import type { Page } from "@playwright/test";
import { beforeEach, expect, it, vi } from "vitest";
import type { ApiClient } from "./api-client";
import type { SeedData } from "../fixtures/test-base";
import { seedTaskCanvas } from "../tests/canvas/canvas-fixture";

const fixture = vi.hoisted(() => ({
  mkdir: vi.fn(),
  write: vi.fn(),
  remove: vi.fn(),
  send: vi.fn(),
  sendMobile: vi.fn(),
}));
vi.mock("node:fs", () => ({
  default: { mkdirSync: fixture.mkdir, writeFileSync: fixture.write, rmSync: fixture.remove },
}));
vi.mock("@playwright/test", async () => ({ expect: (await import("vitest")).expect }));
vi.mock("../pages/session-page", () => ({
  SessionPage: class {
    async waitForLoad() {}
    async waitForChatIdle() {}
    sendMessage = fixture.send;
    sendMessageViaButton = fixture.sendMobile;
  },
}));

beforeEach(() => vi.clearAllMocks());

it.each([false, true])(
  "waits for canvas creation to finish before writing or publishing (mobile=%s)",
  async (mobile) => {
    let creationFinished = false;
    let published = false;
    const canvas = {
      id: "canvas",
      title: "E2E Plugin Canvas",
      workspace_id: "workspace",
      task_id: "task",
      scope_kind: "task",
      status: "active",
    };
    const sessions = vi.fn(async () => ({
      sessions: [
        {
          id: "session",
          state: creationFinished ? "WAITING_FOR_INPUT" : "RUNNING",
          workspace_path: "/workspace",
        },
      ],
    }));
    const publish = async () => {
      published = true;
    };
    fixture.send.mockImplementation(publish);
    fixture.sendMobile.mockImplementation(publish);
    const api = {
      createTaskWithAgent: vi.fn(async () => ({ id: "task", session_id: "session" })),
      listTaskSessions: sessions,
      rawRequest: vi.fn(async (_method: string, url: string) =>
        Response.json(
          url.includes("/tasks/")
            ? { canvases: [canvas] }
            : { ...canvas, ...(published ? { pending_release: { id: "release" } } : {}) },
        ),
      ),
    } as unknown as ApiClient;
    const page = { goto: vi.fn(async () => {}) } as unknown as Page;
    const seed = seedTaskCanvas(
      page,
      api,
      {
        workspaceId: "workspace",
        agentProfileId: "agent",
        workflowId: "workflow",
        startStepId: "step",
        repositoryId: "repo",
      } as SeedData,
      mobile,
    );

    try {
      // The database row exists while agentctl is still staging its source tree.
      // Observe either an early write or a second read of the running session.
      await expect
        .poll(() => fixture.write.mock.calls.length > 0 || sessions.mock.calls.length >= 2)
        .toBe(true);
      expect(fixture.write.mock.calls.length).toBe(0);
      expect(fixture.send).not.toHaveBeenCalled();
      expect(fixture.sendMobile).not.toHaveBeenCalled();
    } finally {
      creationFinished = true;
    }
    const result = await seed;
    expect(result.canvas.pending_release?.id).toBe("release");
    expect(fixture.write).toHaveBeenCalled();
    expect(mobile ? fixture.sendMobile : fixture.send).toHaveBeenCalledOnce();
  },
);
