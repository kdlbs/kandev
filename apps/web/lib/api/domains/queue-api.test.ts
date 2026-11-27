/* eslint-disable sonarjs/no-duplicate-string -- Wire action and fixture IDs are intentionally repeated for readability. */
import { beforeEach, describe, it, expect, vi } from "vitest";
import type { EntityReference } from "@/lib/types/entity-reference";
import { planCommentAdmissionConflict } from "@/lib/plan-comment-refs";
import { WebSocketRequestError } from "@/lib/ws/request-error";

const getWebSocketClientMock = vi.hoisted(() => vi.fn());
const QUEUE_ADD_ACTION = "message.queue.add";
const SESSION_ID = `session-1`;
const INCARNATION_ID = `incarnation-1`;

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: getWebSocketClientMock,
}));

import {
  MergeReferenceOverflowError,
  QueueSendNowError,
  QueueEntryNotFoundError,
  QueueReorderError,
  beginQueuedMessageEdit,
  endQueuedMessageEdit,
  mergeQueuedEntry,
  queueMessage,
  renewQueuedMessageEdit,
  reorderQueuedEntries,
  rethrowQueueError,
  sendQueuedNow,
  updateQueuedMessage,
} from "./queue-api";
import * as queueApi from "./queue-api";

const reference: EntityReference = {
  version: 1,
  ref: "mention:v1:github:issue:acme%2Frepo:42",
  provider: "github",
  kind: "issue",
  id: "42",
  key: "acme/repo#42",
  title: "Fix composer references",
  url: "https://github.com/acme/repo/issues/42",
  scope: "acme/repo",
};

beforeEach(() => {
  getWebSocketClientMock.mockReset();
});

describe("structured plan comment conflicts", () => {
  it("preserves structured plan comment conflicts and their snapshots", () => {
    const details = {
      snapshot: {
        task_id: "task-1",
        plan_id: "plan-1",
        revision: 3,
        comments: [],
      },
    };
    const original = new WebSocketRequestError(
      "Plan comments changed",
      "plan_comments_changed",
      details,
    );
    let caught: unknown;
    try {
      rethrowQueueError(original);
    } catch (err) {
      caught = err;
    }

    expect(caught).toBe(original);
    expect((caught as WebSocketRequestError).code).toBe("plan_comments_changed");
    expect((caught as WebSocketRequestError).details).toEqual(details);
    expect(planCommentAdmissionConflict(caught)).toMatchObject({
      code: "plan_comments_changed",
      snapshot: details.snapshot,
    });
  });

  it("preserves structured primary-session conflicts and their details", () => {
    const details = {
      primary_session_id: "session-2",
      primary_session_state: "RUNNING",
    };
    const original = new WebSocketRequestError(
      "Primary session changed",
      "primary_session_changed",
      details,
    );
    let caught: unknown;
    try {
      rethrowQueueError(original);
    } catch (err) {
      caught = err;
    }

    expect(caught).toBe(original);
    expect((caught as WebSocketRequestError).code).toBe("primary_session_changed");
    expect((caught as WebSocketRequestError).details).toEqual(details);
    expect(planCommentAdmissionConflict(caught)).toMatchObject({
      code: "primary_session_changed",
      primarySessionId: "session-2",
      primarySessionState: "RUNNING",
    });
  });

  it("preserves admission-only validation errors for non-admission operations", () => {
    const details = { field: "content" };
    const original = new WebSocketRequestError("Invalid queue edit", "VALIDATION_ERROR", details);
    let caught: unknown;
    try {
      rethrowQueueError(original);
    } catch (err) {
      caught = err;
    }

    expect(caught).toBe(original);
    expect((caught as WebSocketRequestError).code).toBe("VALIDATION_ERROR");
    expect((caught as WebSocketRequestError).details).toEqual(details);
  });
});

describe("queue reference payloads", () => {
  it("forwards entity references through message.queue.add", async () => {
    const request = vi.fn().mockResolvedValue({ id: "q-1" });
    getWebSocketClientMock.mockReturnValue({ request });

    await queueMessage({
      session_id: SESSION_ID,
      session_incarnation_id: INCARNATION_ID,
      task_id: "task-1",
      content: "queued reference",
      entity_references: [reference],
    });

    expect(request).toHaveBeenCalledWith(
      QUEUE_ADD_ACTION,
      {
        session_id: SESSION_ID,
        session_incarnation_id: INCARNATION_ID,
        task_id: "task-1",
        content: "queued reference",
        entity_references: [reference],
      },
      35_000,
    );
  });

  it("forwards context file metadata through message.queue.add", async () => {
    const request = vi.fn().mockResolvedValue({ id: "q-1" });
    getWebSocketClientMock.mockReturnValue({ request });

    await queueMessage({
      session_id: SESSION_ID,
      session_incarnation_id: INCARNATION_ID,
      task_id: "task-1",
      content: "queued context",
      context_files: [{ path: "src/components", name: "components", is_directory: true }],
    });

    expect(request).toHaveBeenCalledWith(
      QUEUE_ADD_ACTION,
      {
        session_id: SESSION_ID,
        session_incarnation_id: INCARNATION_ID,
        task_id: "task-1",
        content: "queued context",
        context_files: [{ path: "src/components", name: "components", is_directory: true }],
      },
      35_000,
    );
  });
});

describe("queued message reference updates", () => {
  it("sends an explicit empty reference array when replacing a queued message", async () => {
    const request = vi.fn().mockResolvedValue({ entry_id: "q-1" });
    getWebSocketClientMock.mockReturnValue({ request });

    await updateQueuedMessage({
      session_id: SESSION_ID,
      task_id: "task-1",
      session_incarnation_id: INCARNATION_ID,
      entry_id: "q-1",
      content: "reference removed",
    });

    expect(request).toHaveBeenCalledWith(
      "message.queue.update",
      {
        task_id: "task-1",
        session_id: SESSION_ID,
        session_incarnation_id: INCARNATION_ID,
        entry_id: "q-1",
        content: "reference removed",
        entity_references: [],
      },
      35_000,
    );
  });

  it("forwards surviving references through message.queue.update", async () => {
    const request = vi.fn().mockResolvedValue({ entry_id: "q-1" });
    getWebSocketClientMock.mockReturnValue({ request });

    await updateQueuedMessage({
      task_id: "task-1",
      session_id: SESSION_ID,
      session_incarnation_id: INCARNATION_ID,
      entry_id: "q-1",
      content: "reference kept",
      entity_references: [reference],
    });

    expect(request).toHaveBeenCalledWith(
      "message.queue.update",
      {
        task_id: "task-1",
        session_id: SESSION_ID,
        session_incarnation_id: INCARNATION_ID,
        entry_id: "q-1",
        content: "reference kept",
        entity_references: [reference],
      },
      35_000,
    );
  });
});

describe("queued message edit leases", () => {
  const lease = {
    session_id: "session-1",
    entry_id: "q-1",
    lease_id: "lease-1",
    target_revision: 3,
  };

  it("uses dedicated begin, renew, and end actions", async () => {
    const request = vi
      .fn()
      .mockResolvedValueOnce(lease)
      .mockResolvedValueOnce({ ...lease, lease_generation: 2 })
      .mockResolvedValueOnce(undefined);
    getWebSocketClientMock.mockReturnValue({ request });

    await expect(beginQueuedMessageEdit("session-1", "q-1")).resolves.toEqual(lease);
    await expect(renewQueuedMessageEdit(lease)).resolves.toMatchObject({
      lease_generation: 2,
    });
    await expect(endQueuedMessageEdit(lease)).resolves.toBeUndefined();
    expect(request).toHaveBeenNthCalledWith(
      1,
      "message.queue.edit.begin",
      {
        session_id: "session-1",
        entry_id: "q-1",
      },
      35_000,
    );
    expect(request).toHaveBeenNthCalledWith(2, "message.queue.edit.renew", lease, 35_000);
    expect(request).toHaveBeenNthCalledWith(3, "message.queue.edit.end", lease, 35_000);
  });
  it("requests a policy-preserving drain after a successful save", async () => {
    const request = vi.fn().mockResolvedValue(undefined);
    getWebSocketClientMock.mockReturnValue({ request });

    await endQueuedMessageEdit(lease, true);

    expect(request).toHaveBeenCalledWith(
      "message.queue.edit.end",
      {
        ...lease,
        dispatch_if_auto_run: true,
      },
      35_000,
    );
  });

  it("forwards operation and target revision fences when replacing content", async () => {
    const request = vi
      .fn()
      .mockResolvedValue({ entry_id: "q-1", operation_id: "op-1", target_revision: 4 });
    getWebSocketClientMock.mockReturnValue({ request });

    await updateQueuedMessage({
      task_id: "task-1",
      session_id: "session-1",
      session_incarnation_id: INCARNATION_ID,
      entry_id: "q-1",
      lease_id: "lease-1",
      operation_id: "op-1",
      expected_target_revision: 3,
      content: "edited",
      entity_references: [],
    });

    expect(request).toHaveBeenCalledWith(
      "message.queue.update",
      {
        task_id: "task-1",
        session_id: "session-1",
        session_incarnation_id: INCARNATION_ID,
        entry_id: "q-1",
        lease_id: "lease-1",
        operation_id: "op-1",
        expected_target_revision: 3,
        content: "edited",
        entity_references: [],
      },
      35_000,
    );
  });
});

describe("mergeQueuedEntry", () => {
  it("forwards session, entry, and caller identity through message.queue.merge", async () => {
    const request = vi.fn().mockResolvedValue({ entry_id: "q-a" });
    getWebSocketClientMock.mockReturnValue({ request });

    await mergeQueuedEntry({
      task_id: "task-1",
      session_id: SESSION_ID,
      session_incarnation_id: INCARNATION_ID,
      entry_id: "q-b",
      user_id: "user-1",
    });

    expect(request).toHaveBeenCalledWith(
      "message.queue.merge",
      {
        task_id: "task-1",
        session_id: SESSION_ID,
        session_incarnation_id: INCARNATION_ID,
        entry_id: "q-b",
        user_id: "user-1",
      },
      35_000,
    );
  });

  it("omits user_id when the caller identity is unknown", async () => {
    const request = vi.fn().mockResolvedValue({ entry_id: "q-a" });
    getWebSocketClientMock.mockReturnValue({ request });

    await mergeQueuedEntry({
      task_id: "task-1",
      session_id: SESSION_ID,
      session_incarnation_id: INCARNATION_ID,
      entry_id: "q-b",
    });

    expect(request).toHaveBeenCalledWith(
      "message.queue.merge",
      {
        task_id: "task-1",
        session_id: SESSION_ID,
        session_incarnation_id: INCARNATION_ID,
        entry_id: "q-b",
      },
      35_000,
    );
  });

  it("maps a drained-source rejection to QueueEntryNotFoundError", async () => {
    const request = vi
      .fn()
      .mockRejectedValue({ code: "entry_not_found", message: "Already drained" });
    getWebSocketClientMock.mockReturnValue({ request });

    await expect(
      mergeQueuedEntry({
        task_id: "task-1",
        session_id: SESSION_ID,
        session_incarnation_id: INCARNATION_ID,
        entry_id: "q-b",
      }),
    ).rejects.toBeInstanceOf(QueueEntryNotFoundError);
  });

  it("maps a reference-overflow rejection to MergeReferenceOverflowError", async () => {
    const request = vi.fn().mockRejectedValue({
      code: "merge_reference_overflow",
      message: "merge would exceed the per-message entity reference limit",
    });
    getWebSocketClientMock.mockReturnValue({ request });

    await expect(
      mergeQueuedEntry({
        task_id: "task-1",
        session_id: SESSION_ID,
        session_incarnation_id: INCARNATION_ID,
        entry_id: "q-b",
      }),
    ).rejects.toBeInstanceOf(MergeReferenceOverflowError);
  });
});

describe("sendQueuedNow", () => {
  it("sends an exact entry scope", async () => {
    const request = vi.fn().mockResolvedValue({
      session_id: SESSION_ID,
      dispatched: true,
      sent_count: 1,
    });
    getWebSocketClientMock.mockReturnValue({ request });

    const params = {
      task_id: "task-1",
      session_id: SESSION_ID,
      session_incarnation_id: INCARNATION_ID,
      scope: "entry",
      entry_id: "q-2",
    } as const;
    await sendQueuedNow(params);
    expect(request).toHaveBeenCalledWith("message.queue.send_now", params, 35_000);
  });

  it("omits entry_id for an all scope snapshot", async () => {
    const request = vi.fn().mockResolvedValue({
      session_id: SESSION_ID,
      dispatched: true,
      sent_count: 3,
    });
    getWebSocketClientMock.mockReturnValue({ request });

    const params = {
      task_id: "task-1",
      session_id: SESSION_ID,
      session_incarnation_id: INCARNATION_ID,
      scope: "all",
    } as const;
    await sendQueuedNow(params);
    expect(request).toHaveBeenCalledWith("message.queue.send_now", params, 35_000);
  });

  it("maps send-now conflict codes to a typed error", async () => {
    const conflict = new WebSocketRequestError(
      "Another cancellation is in progress",
      "send_now_conflict",
      { session_id: SESSION_ID },
    );
    const request = vi.fn().mockRejectedValue(conflict);
    getWebSocketClientMock.mockReturnValue({ request });

    await expect(
      sendQueuedNow({
        task_id: "task-1",
        session_id: SESSION_ID,
        session_incarnation_id: INCARNATION_ID,
        scope: "all",
      }),
    ).rejects.toMatchObject({
      code: "send_now_conflict",
    });
    await expect(
      sendQueuedNow({
        task_id: "task-1",
        session_id: SESSION_ID,
        session_incarnation_id: INCARNATION_ID,
        scope: "all",
      }),
    ).rejects.toBeInstanceOf(QueueSendNowError);
  });
});

describe("setQueueAutoRun", () => {
  it("sets the per-session queue policy through its exact WebSocket action", async () => {
    const request = vi.fn().mockResolvedValue({
      session_id: SESSION_ID,
      auto_run: false,
      dispatched: false,
    });
    getWebSocketClientMock.mockReturnValue({ request });
    const setQueueAutoRun = (
      queueApi as typeof queueApi & {
        setQueueAutoRun?: (
          identity: {
            task_id: string;
            session_id: string;
            session_incarnation_id: string;
          },
          enabled: boolean,
        ) => Promise<{ session_id: string; auto_run: boolean; dispatched: boolean }>;
      }
    ).setQueueAutoRun;

    expect(setQueueAutoRun).toBeTypeOf("function");
    await setQueueAutoRun!(
      { task_id: "task-1", session_id: SESSION_ID, session_incarnation_id: INCARNATION_ID },
      false,
    );

    expect(request).toHaveBeenCalledWith(
      "message.queue.auto_run.set",
      {
        session_id: SESSION_ID,
        task_id: "task-1",
        session_incarnation_id: INCARNATION_ID,
        enabled: false,
      },
      35_000,
    );
  });
});
describe("setQueueAutoMerge", () => {
  it("sets the session override with the complete immutable identity", async () => {
    const request = vi.fn().mockResolvedValue({
      task_id: "task-1",
      session_id: SESSION_ID,
      session_incarnation_id: INCARNATION_ID,
      auto_merge_enabled: false,
      auto_merge_source: "session",
      auto_merge_revision: 1,
    });
    getWebSocketClientMock.mockReturnValue({ request });
    const setQueueAutoMerge = (
      queueApi as typeof queueApi & {
        setQueueAutoMerge?: (
          identity: {
            task_id: string;
            session_id: string;
            session_incarnation_id: string;
          },
          enabled: boolean,
        ) => Promise<unknown>;
      }
    ).setQueueAutoMerge;

    expect(setQueueAutoMerge).toBeTypeOf("function");
    await setQueueAutoMerge!(
      {
        task_id: "task-1",
        session_id: SESSION_ID,
        session_incarnation_id: INCARNATION_ID,
      },
      false,
    );

    expect(request).toHaveBeenCalledWith(
      "message.queue.auto_merge.set",
      {
        task_id: "task-1",
        session_id: SESSION_ID,
        session_incarnation_id: INCARNATION_ID,
        enabled: false,
      },
      35_000,
    );
  });
});

describe("reorderQueuedEntries", () => {
  it("sends the full ordered id list through message.queue.reorder", async () => {
    const request = vi.fn().mockResolvedValue({ session_id: SESSION_ID, reordered: 3 });
    getWebSocketClientMock.mockReturnValue({ request });

    await reorderQueuedEntries({
      task_id: "task-1",
      session_id: SESSION_ID,
      session_incarnation_id: INCARNATION_ID,
      ordered_ids: ["q-c", "q-a", "q-b"],
    });

    expect(request).toHaveBeenCalledWith(
      "message.queue.reorder",
      {
        task_id: "task-1",
        session_id: SESSION_ID,
        session_incarnation_id: INCARNATION_ID,
        ordered_ids: ["q-c", "q-a", "q-b"],
      },
      35_000,
    );
  });

  it("maps a queue_changed rejection to QueueReorderError", async () => {
    const request = vi.fn().mockRejectedValue({
      code: "queue_changed",
      message: "Queue changed before the reorder could be applied",
    });
    getWebSocketClientMock.mockReturnValue({ request });

    await expect(
      reorderQueuedEntries({
        task_id: "task-1",
        session_id: SESSION_ID,
        session_incarnation_id: INCARNATION_ID,
        ordered_ids: ["q-a"],
      }),
    ).rejects.toBeInstanceOf(QueueReorderError);
  });

  it("falls through to the shared error mapping for other failures", async () => {
    const request = vi.fn().mockRejectedValue({ code: "entry_not_found", message: "Gone" });
    getWebSocketClientMock.mockReturnValue({ request });

    await expect(
      reorderQueuedEntries({
        task_id: "task-1",
        session_id: SESSION_ID,
        session_incarnation_id: INCARNATION_ID,
        ordered_ids: ["q-a"],
      }),
    ).rejects.toBeInstanceOf(QueueEntryNotFoundError);
  });
});
