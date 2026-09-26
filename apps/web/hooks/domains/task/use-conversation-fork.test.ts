import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

const mockListCandidates = vi.fn();
const mockListMessages = vi.fn();
const mockListTurns = vi.fn();
const mockCreateDraft = vi.fn();
const mockGetForkContent = vi.fn();
const mockGetForkDraft = vi.fn();
const mockEstimateFork = vi.fn();
const mockDiscardFork = vi.fn();

vi.mock("@/lib/api/domains/conversation-fork-api", () => ({
  listConversationForkCandidates: (...args: unknown[]) => mockListCandidates(...args),
  createConversationForkDraft: (...args: unknown[]) => mockCreateDraft(...args),
  getConversationForkDraft: (...args: unknown[]) => mockGetForkDraft(...args),
  getConversationForkContent: (...args: unknown[]) => mockGetForkContent(...args),
  estimateConversationForkDraft: (...args: unknown[]) => mockEstimateFork(...args),
  discardConversationForkDraft: (...args: unknown[]) => mockDiscardFork(...args),
}));

vi.mock("@/lib/api/domains/session-api", () => ({
  listTaskSessionMessages: (...args: unknown[]) => mockListMessages(...args),
  listSessionTurns: (...args: unknown[]) => mockListTurns(...args),
}));

import { useConversationFork } from "./use-conversation-fork";

const FIXED_CUTOFF_TIME = "2026-09-23T10:00:00Z";
const CUTOFF_MESSAGE_ID = "cutoff";
const EARLIER_USER_MESSAGE_ID = "earlier-user";
const NEW_SESSION_ID = "session-new";

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((done) => {
    resolve = done;
  });
  return { promise, resolve };
}

function candidate(sessionId: string) {
  return {
    task_id: "task-1",
    session_id: sessionId,
    task_title: "Task title",
    revision: 1,
    cutoff_message_id: "cutoff",
    cutoff_turn_complete: true,
    attachments: [],
    attachments_has_more: false,
  };
}

function forkDescriptor(id = "fork-1") {
  return {
    id,
    source_task_id: "task-1",
    source_session_id: "session-1",
    source_message_id: CUTOFF_MESSAGE_ID,
    source_revision: 1,
    compiler_version: "v1",
    content_hash: "hash-1",
    message_count: 1,
    text_bytes: 10,
    omissions: {},
    attachments: [],
    estimate: { estimated_tokens: 12, method: "test", attachments_unmeasured: false },
    created_at: FIXED_CUTOFF_TIME,
    expires_at: "2026-09-24T10:00:00Z",
    state: "draft",
  };
}

beforeEach(() => {
  vi.resetAllMocks();
  mockDiscardFork.mockResolvedValue(undefined);
  mockListMessages.mockImplementation(async (sessionId: string) => ({
    messages: [
      {
        id: "earlier-assistant",
        session_id: sessionId,
        task_id: "task-1",
        author_type: "agent",
        type: "message",
        content: "Earlier answer",
        turn_id: "missing-turn",
        created_at: "2026-09-23T09:00:00Z",
      },
      {
        id: EARLIER_USER_MESSAGE_ID,
        session_id: sessionId,
        task_id: "task-1",
        author_type: "user",
        type: "message",
        content: "Earlier request",
        turn_id: "missing-turn",
        created_at: "2026-09-23T09:30:00Z",
      },
      {
        id: CUTOFF_MESSAGE_ID,
        session_id: sessionId,
        task_id: "task-1",
        author_type: "agent",
        type: "message",
        content: "Answer",
        created_at: FIXED_CUTOFF_TIME,
      },
    ],
    has_more: false,
    cursor: CUTOFF_MESSAGE_ID,
  }));
  mockListTurns.mockResolvedValue({ turns: [], total: 0 });
});

describe("useConversationFork source response ordering", () => {
  it("ignores an older source response after a newer cutoff request starts", async () => {
    const first = deferred<ReturnType<typeof candidate>>();
    const second = deferred<ReturnType<typeof candidate>>();
    mockListCandidates.mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise);
    const { result } = renderHook(() => useConversationFork());

    let firstLoad!: Promise<void>;
    let secondLoad!: Promise<void>;
    act(() => {
      firstLoad = result.current.loadSource("session-old", CUTOFF_MESSAGE_ID);
      secondLoad = result.current.loadSource(NEW_SESSION_ID, CUTOFF_MESSAGE_ID);
    });
    await act(async () => {
      second.resolve(candidate(NEW_SESSION_ID));
      await secondLoad;
    });
    await waitFor(() => expect(result.current.source?.sessionId).toBe(NEW_SESSION_ID));

    await act(async () => {
      first.resolve(candidate("session-old"));
      await firstLoad;
    });

    expect(result.current.source?.sessionId).toBe(NEW_SESSION_ID);
  });
});

describe("useConversationFork cutoff boundaries", () => {
  it("offers persisted user messages and only assistant messages with a completed turn", async () => {
    mockListCandidates.mockResolvedValue(candidate("session-1"));
    mockListTurns.mockResolvedValue({
      turns: [{ id: "complete-turn", completed_at: FIXED_CUTOFF_TIME }],
      total: 1,
    });
    const { result } = renderHook(() => useConversationFork());

    await act(async () => {
      await result.current.loadSource("session-1", "cutoff");
    });

    expect(result.current.source?.boundaries).toEqual([
      expect.objectContaining({
        message: expect.objectContaining({ id: "earlier-assistant" }),
        finalized: false,
      }),
      expect.objectContaining({
        message: expect.objectContaining({ id: "earlier-user" }),
        finalized: true,
      }),
      expect.objectContaining({
        message: expect.objectContaining({ id: "cutoff" }),
        finalized: true,
      }),
    ]);
  });
});

describe("useConversationFork attachment candidates", () => {
  it("reloads attachment candidates for the selected start boundary", async () => {
    mockListCandidates.mockResolvedValueOnce(candidate("session-1")).mockResolvedValueOnce({
      ...candidate("session-1"),
      attachments: [{ source_id: "in-range", name: "notes.txt", available: true }],
    });
    const { result } = renderHook(() => useConversationFork());

    await act(async () => {
      await result.current.loadSource("session-1", "cutoff");
    });
    await act(async () => {
      await result.current.loadAttachments("earlier-user");
    });

    expect(mockListCandidates).toHaveBeenLastCalledWith(
      "session-1",
      "cutoff",
      expect.objectContaining({ startMessageId: EARLIER_USER_MESSAGE_ID }),
    );
    expect(result.current.source?.attachments).toEqual([
      expect.objectContaining({ source_id: "in-range" }),
    ]);
  });
});

describe("useConversationFork snapshot retries", () => {
  it("reuses the draft request and returned descriptor when content loading can be retried", async () => {
    mockListCandidates.mockResolvedValue(candidate("session-1"));
    mockCreateDraft.mockResolvedValue(forkDescriptor());
    mockGetForkContent
      .mockRejectedValueOnce(new Error("temporary content read failure"))
      .mockResolvedValueOnce({
        content: "frozen history",
        content_hash: "hash-1",
        compiler_version: "v1",
      });
    const { result } = renderHook(() => useConversationFork());

    await act(async () => {
      await result.current.loadSource("session-1", "cutoff");
    });
    const selection = { includeToolEvidence: false, attachmentIds: [] };
    await act(async () => {
      await result.current.createSnapshot(selection);
    });
    await act(async () => {
      await result.current.createSnapshot(selection);
    });

    expect(mockCreateDraft).toHaveBeenCalledTimes(1);
    expect(mockCreateDraft.mock.calls[0]?.[1].draft_request_id).toBeTruthy();
    expect(result.current.snapshot?.descriptor.id).toBe("fork-1");
    expect(result.current.snapshot?.content.content).toBe("frozen history");
  });

  it("discards a descriptor that was returned before preview content failed", async () => {
    mockListCandidates.mockResolvedValue(candidate("session-1"));
    mockCreateDraft.mockResolvedValue(forkDescriptor());
    mockGetForkContent.mockRejectedValueOnce(new Error("temporary content read failure"));
    const { result } = renderHook(() => useConversationFork());

    await act(async () => {
      await result.current.loadSource("session-1", "cutoff");
    });
    await act(async () => {
      await result.current.createSnapshot({ includeToolEvidence: false, attachmentIds: [] });
    });
    await act(async () => {
      await result.current.discardSnapshot();
    });

    expect(mockDiscardFork).toHaveBeenCalledWith("fork-1");
  });
});

describe("useConversationFork snapshot replacement", () => {
  it("keeps the current snapshot usable when a replacement preview fails", async () => {
    mockListCandidates.mockResolvedValue(candidate("session-1"));
    mockCreateDraft
      .mockResolvedValueOnce(forkDescriptor("fork-current"))
      .mockResolvedValueOnce(forkDescriptor("fork-replacement"));
    mockGetForkContent
      .mockResolvedValueOnce({
        content: "current history",
        content_hash: "hash-1",
        compiler_version: "v1",
      })
      .mockRejectedValueOnce(new Error("temporary replacement read failure"));
    const { result } = renderHook(() => useConversationFork());

    await act(async () => {
      await result.current.loadSource("session-1", "cutoff");
    });
    await act(async () => {
      await result.current.createSnapshot({ includeToolEvidence: false, attachmentIds: [] });
    });
    await act(async () => {
      await result.current.createSnapshot({ includeToolEvidence: true, attachmentIds: [] });
    });

    expect(result.current.snapshot?.descriptor.id).toBe("fork-current");
    expect(result.current.snapshot?.content.content).toBe("current history");
    expect(mockDiscardFork).not.toHaveBeenCalledWith("fork-current");
  });
});

describe("useConversationFork same-selection concurrency", () => {
  it("does not discard an idempotent draft shared by overlapping same-selection requests", async () => {
    mockListCandidates.mockResolvedValue(candidate("session-1"));
    mockCreateDraft.mockResolvedValue(forkDescriptor());
    const firstContent = deferred<{
      content: string;
      content_hash: string;
      compiler_version: string;
    }>();
    const secondContent = deferred<{
      content: string;
      content_hash: string;
      compiler_version: string;
    }>();
    mockGetForkContent
      .mockReturnValueOnce(firstContent.promise)
      .mockReturnValueOnce(secondContent.promise);
    const { result } = renderHook(() => useConversationFork());
    await act(async () => {
      await result.current.loadSource("session-1", "cutoff");
    });

    let first!: Promise<unknown>;
    let second!: Promise<unknown>;
    act(() => {
      first = result.current.createSnapshot({ includeToolEvidence: false, attachmentIds: [] });
    });
    await waitFor(() => expect(mockGetForkContent).toHaveBeenCalledTimes(1));
    act(() => {
      second = result.current.createSnapshot({ includeToolEvidence: false, attachmentIds: [] });
    });
    await waitFor(() => expect(mockGetForkContent).toHaveBeenCalledTimes(2));
    await act(async () => {
      firstContent.resolve({ content: "history", content_hash: "hash-1", compiler_version: "v1" });
      secondContent.resolve({ content: "history", content_hash: "hash-1", compiler_version: "v1" });
      await Promise.all([first, second]);
    });

    expect(mockDiscardFork).not.toHaveBeenCalledWith("fork-1");
    expect(result.current.snapshot?.descriptor.id).toBe("fork-1");
  });
});

describe("useConversationFork stale-selection concurrency", () => {
  it("does not let a stale selection replace the accepted snapshot", async () => {
    mockListCandidates.mockResolvedValue(candidate("session-1"));
    mockCreateDraft
      .mockResolvedValueOnce(forkDescriptor("fork-old-selection"))
      .mockResolvedValueOnce(forkDescriptor("fork-new-selection"));
    const oldContent = deferred<{
      content: string;
      content_hash: string;
      compiler_version: string;
    }>();
    const newContent = deferred<{
      content: string;
      content_hash: string;
      compiler_version: string;
    }>();
    mockGetForkContent
      .mockReturnValueOnce(oldContent.promise)
      .mockReturnValueOnce(newContent.promise);
    const { result } = renderHook(() => useConversationFork());
    await act(async () => {
      await result.current.loadSource("session-1", "cutoff");
    });

    let oldSelection!: Promise<unknown>;
    let newSelection!: Promise<unknown>;
    act(() => {
      oldSelection = result.current.createSnapshot({
        includeToolEvidence: false,
        attachmentIds: [],
      });
    });
    await waitFor(() => expect(mockGetForkContent).toHaveBeenCalledTimes(1));
    act(() => {
      newSelection = result.current.createSnapshot({
        includeToolEvidence: true,
        attachmentIds: [],
      });
    });
    await waitFor(() => expect(mockGetForkContent).toHaveBeenCalledTimes(2));

    await act(async () => {
      newContent.resolve({
        content: "new history",
        content_hash: "hash-1",
        compiler_version: "v1",
      });
      await newSelection;
    });
    await act(async () => {
      oldContent.resolve({
        content: "old history",
        content_hash: "hash-1",
        compiler_version: "v1",
      });
      await oldSelection;
    });

    expect(result.current.snapshot?.descriptor.id).toBe("fork-new-selection");
    expect(result.current.snapshot?.content.content).toBe("new history");
    expect(mockDiscardFork).toHaveBeenCalledWith("fork-old-selection");
  });
});

describe("useConversationFork active user cutoffs", () => {
  it("creates snapshots from an accepted user cutoff while its turn is active", async () => {
    mockListCandidates.mockResolvedValue({
      ...candidate("session-1"),
      cutoff_turn_complete: false,
    });
    mockListMessages.mockImplementation(async (sessionId: string) => ({
      messages: [
        {
          id: CUTOFF_MESSAGE_ID,
          session_id: sessionId,
          task_id: "task-1",
          author_type: "user",
          type: "message",
          content: "Still-running turn",
          created_at: FIXED_CUTOFF_TIME,
        },
      ],
      has_more: false,
      cursor: CUTOFF_MESSAGE_ID,
    }));
    mockCreateDraft.mockResolvedValue(forkDescriptor());
    mockGetForkContent.mockResolvedValue({
      content: "user context",
      content_hash: "hash-1",
      compiler_version: "v1",
    });
    const { result } = renderHook(() => useConversationFork());

    await act(async () => {
      await result.current.loadSource("session-1", "cutoff");
    });
    let snapshot: unknown;
    await act(async () => {
      snapshot = await result.current.createSnapshot({
        includeToolEvidence: false,
        attachmentIds: [],
      });
    });

    expect(snapshot).toEqual(
      expect.objectContaining({ descriptor: expect.objectContaining({ id: "fork-1" }) }),
    );
    expect(mockCreateDraft).toHaveBeenCalledTimes(1);
  });
});
