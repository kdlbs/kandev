import { afterEach, describe, expect, it } from "vitest";
import { act, cleanup, render, screen } from "@testing-library/react";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import type { TaskSession } from "@/lib/types/http";
import { SenderTaskBadge, type SenderTaskInfo } from "./sender-task-badge";

const TASK_ID = "task-1";
const TASK_TITLE = "Review Contributor PR 3143";
const SESSION_ID = "12345678-aaaa-bbbb-cccc-123456789abc";
const BADGE_TEST_ID = "sender-task-badge";
const SNAPSHOT_SESSION_NAME = "Luna snapshot";

afterEach(cleanup);

type RenderBadgeOptions = {
  session?: { taskId?: string; name: string | null } | null;
  snapshotSessionName?: string;
  modelName?: string;
  captureStore?: (store: ReturnType<typeof useAppStoreApi>) => void;
};

function renderBadge({
  session = { taskId: TASK_ID, name: "Luna" },
  snapshotSessionName = "Old session label",
  modelName,
  captureStore,
}: RenderBadgeOptions = {}) {
  const initialState = {
    kanban: {
      tasks: [{ id: TASK_ID, title: TASK_TITLE }],
    },
    taskSessions: {
      items: session
        ? {
            [SESSION_ID]: {
              id: SESSION_ID,
              task_id: session.taskId ?? TASK_ID,
              name: session.name,
              state: "WAITING_FOR_INPUT",
              started_at: "2026-09-28T00:00:00Z",
              updated_at: "2026-09-28T00:00:00Z",
            } as TaskSession,
          }
        : {},
    },
    sessionModels: {
      bySessionId: modelName
        ? {
            [SESSION_ID]: {
              currentModelId: "current-model",
              models: [{ modelId: "current-model", name: modelName }],
              configOptions: [],
            },
          }
        : {},
    },
  };
  const sender: SenderTaskInfo = {
    id: TASK_ID,
    snapshotTitle: TASK_TITLE,
    sessionId: SESSION_ID,
    sessionName: snapshotSessionName,
  };

  render(
    <StateProvider initialState={initialState as never}>
      {captureStore && <StoreCapture capture={captureStore} />}
      <SenderTaskBadge sender={sender} destinationTaskId={TASK_ID} />
    </StateProvider>,
  );
}

function StoreCapture({
  capture,
}: {
  capture: (store: ReturnType<typeof useAppStoreApi>) => void;
}) {
  capture(useAppStoreApi());
  return null;
}

describe("SenderTaskBadge", () => {
  it("uses the loaded sender session label instead of repeating the task title", () => {
    renderBadge();

    const badge = screen.getByTestId(BADGE_TEST_ID);
    expect(badge.textContent?.trim()).toBe("Luna");
  });

  it("tracks the same current model label as the session tab", () => {
    let store: ReturnType<typeof useAppStoreApi> | undefined;
    renderBadge({
      session: { name: null },
      modelName: "Luna",
      captureStore: (value) => (store = value),
    });

    const badge = screen.getByTestId(BADGE_TEST_ID);
    expect(badge.textContent?.trim()).toBe("Luna");

    act(() => {
      store?.getState().setSessionModels(SESSION_ID, {
        currentModelId: "model-astra",
        models: [{ modelId: "model-astra", name: "Astra" }],
        configOptions: [],
      });
    });

    expect(badge.textContent?.trim()).toBe("Astra");
  });

  it("uses the captured session name when the sender session is not loaded", () => {
    renderBadge({ session: null, snapshotSessionName: SNAPSHOT_SESSION_NAME });

    expect(screen.getByTestId(BADGE_TEST_ID).textContent?.trim()).toBe(SNAPSHOT_SESSION_NAME);
  });

  it("shows a stable session identifier when no sender label is available", () => {
    renderBadge({ session: null, snapshotSessionName: "" });

    const badge = screen.getByTestId(BADGE_TEST_ID);
    expect(badge.textContent?.trim()).toBe("Agent 12345678");
    expect(badge.getAttribute("aria-label")).toBe(
      `From session "Agent ${SESSION_ID}" in task "${TASK_TITLE}"`,
    );
  });

  it("does not use a loaded session that belongs to another task", () => {
    renderBadge({
      session: { taskId: "task-other", name: "Wrong session" },
      snapshotSessionName: SNAPSHOT_SESSION_NAME,
    });

    const badge = screen.getByTestId(BADGE_TEST_ID);
    expect(badge.textContent).toContain(SNAPSHOT_SESSION_NAME);
    expect(badge.textContent).not.toContain("Wrong session");
  });
});
