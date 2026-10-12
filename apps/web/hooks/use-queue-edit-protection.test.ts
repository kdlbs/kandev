import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { QueuedMessage } from "@/lib/state/slices/session/types";
import { toast } from "@/lib/toast/sonner";
import {
  beginQueuedMessageEdit,
  endQueuedMessageEdit,
  renewQueuedMessageEdit,
  type QueueEditLease,
} from "@/lib/api/domains/queue-api";
import { useQueueEditProtection } from "./use-queue-edit-protection";

vi.mock("@/lib/api/domains/queue-api", () => ({
  beginQueuedMessageEdit: vi.fn(),
  endQueuedMessageEdit: vi.fn(),
  renewQueuedMessageEdit: vi.fn(),
}));

vi.mock("@/lib/toast/sonner", () => ({
  toast: { error: vi.fn() },
}));
const SESSION_A = "session-a";
const SESSION_B = "session-b";
const ENTRY_ID = "entry-1";

function entry(): QueuedMessage {
  return {
    id: ENTRY_ID,
    session_id: SESSION_A,
    task_id: "task-1",
    content: "queued",
    plan_mode: false,
    queued_at: "2026-08-28T00:00:00Z",
    queued_by: "user",
  };
}

beforeEach(() => {
  vi.mocked(toast.error).mockReset();
  vi.mocked(beginQueuedMessageEdit).mockClear();
  vi.mocked(endQueuedMessageEdit).mockClear();
  vi.mocked(renewQueuedMessageEdit).mockClear();
  vi.mocked(beginQueuedMessageEdit).mockResolvedValue({
    session_id: SESSION_A,
    entry_id: ENTRY_ID,
    lease_id: "lease-1",
    target_revision: 0,
  });
  vi.mocked(endQueuedMessageEdit).mockResolvedValue();
  vi.mocked(renewQueuedMessageEdit).mockResolvedValue({
    session_id: SESSION_A,
    entry_id: ENTRY_ID,
    lease_id: "lease-1",
    target_revision: 0,
  });
});

describe("useQueueEditProtection", () => {
  it("releases the target lease when the session changes during editing", async () => {
    const { result, rerender } = renderHook(
      ({ sessionId }: { sessionId: string }) =>
        useQueueEditProtection({
          sessionId,
          entries: [entry()],
        }),
      { initialProps: { sessionId: SESSION_A } },
    );

    await act(async () => {
      expect(await result.current.beginEdit(ENTRY_ID)).toBeTruthy();
    });

    rerender({ sessionId: SESSION_B });

    await waitFor(() =>
      expect(endQueuedMessageEdit).toHaveBeenCalledWith({
        session_id: SESSION_A,
        entry_id: ENTRY_ID,
        lease_id: "lease-1",
        target_revision: 0,
      }),
    );
  });
});

it("allows only one lease acquisition while a begin request is pending", async () => {
  let resolveBegin!: (lease: QueueEditLease) => void;
  const pendingBegin = new Promise<QueueEditLease>((resolve) => {
    resolveBegin = resolve;
  });
  vi.mocked(beginQueuedMessageEdit).mockReturnValueOnce(pendingBegin);
  const { result } = renderHook(() =>
    useQueueEditProtection({
      sessionId: SESSION_A,
      entries: [entry()],
    }),
  );

  let firstEdit!: Promise<string | false>;
  act(() => {
    firstEdit = result.current.beginEdit(ENTRY_ID);
  });
  let secondEdit!: Promise<string | false>;
  act(() => {
    secondEdit = result.current.beginEdit("entry-2");
  });

  await expect(secondEdit).resolves.toBe(false);
  expect(beginQueuedMessageEdit).toHaveBeenCalledTimes(1);

  await act(async () => {
    resolveBegin({
      session_id: SESSION_A,
      entry_id: ENTRY_ID,
      lease_id: "lease-pending",
      target_revision: 0,
    });
    await expect(firstEdit).resolves.toBeTruthy();
  });
});

it("releases a lease when the target disappears during acquisition", async () => {
  let resolveBegin!: (lease: QueueEditLease) => void;
  const pendingBegin = new Promise<QueueEditLease>((resolve) => {
    resolveBegin = resolve;
  });
  vi.mocked(beginQueuedMessageEdit).mockReturnValueOnce(pendingBegin);
  const { result, rerender } = renderHook(
    ({ entries }: { entries: QueuedMessage[] }) =>
      useQueueEditProtection({ sessionId: SESSION_A, entries }),
    { initialProps: { entries: [entry()] } },
  );

  let pendingEdit!: Promise<string | false>;
  act(() => {
    pendingEdit = result.current.beginEdit(ENTRY_ID);
  });
  rerender({ entries: [] });

  await act(async () => {
    resolveBegin({
      session_id: SESSION_A,
      entry_id: ENTRY_ID,
      lease_id: "lease-gone",
      target_revision: 0,
    });
    await expect(pendingEdit).resolves.toBe(false);
  });

  expect(endQueuedMessageEdit).toHaveBeenCalledWith(
    expect.objectContaining({ lease_id: "lease-gone" }),
  );
});

it("releases the lease when the target disappears after acquisition", async () => {
  const { result, rerender } = renderHook(
    ({ entries }: { entries: QueuedMessage[] }) =>
      useQueueEditProtection({ sessionId: SESSION_A, entries }),
    { initialProps: { entries: [entry()] } },
  );

  await act(async () => {
    expect(await result.current.beginEdit(ENTRY_ID)).toBeTruthy();
  });

  rerender({ entries: [] });

  await waitFor(() => expect(result.current.editingEntryId).toBeNull());
  expect(endQueuedMessageEdit).toHaveBeenCalledWith(
    expect.objectContaining({ lease_id: "lease-1" }),
  );
});

describe("late queued edit lease operations", () => {
  it("keeps a replacement lease when an older renewal fails", async () => {
    vi.useFakeTimers();
    try {
      let rejectRenewal!: (error: Error) => void;
      const firstRenewal = new Promise<never>((_, reject) => {
        rejectRenewal = reject;
      });
      vi.mocked(renewQueuedMessageEdit).mockReturnValueOnce(firstRenewal);
      const replacementLease = {
        session_id: SESSION_A,
        entry_id: ENTRY_ID,
        lease_id: "lease-2",
        target_revision: 0,
      };
      const { result } = renderHook(() =>
        useQueueEditProtection({
          sessionId: SESSION_A,
          entries: [entry()],
        }),
      );

      await act(async () => {
        expect(await result.current.beginEdit(ENTRY_ID)).toBeTruthy();
      });
      await act(async () => {
        vi.advanceTimersByTime(20_000);
        await Promise.resolve();
      });
      expect(renewQueuedMessageEdit).toHaveBeenCalledTimes(1);

      await act(async () => {
        await result.current.completeEdit(ENTRY_ID);
      });
      vi.mocked(beginQueuedMessageEdit).mockResolvedValueOnce(replacementLease);
      await act(async () => {
        expect(await result.current.beginEdit(ENTRY_ID)).toBeTruthy();
      });

      await act(async () => {
        rejectRenewal(new Error("stale renewal failed"));
        await Promise.resolve();
      });

      expect(result.current.editLease?.lease_id).toBe("lease-2");
      expect(result.current.editingEntryId).toBe(ENTRY_ID);
    } finally {
      vi.useRealTimers();
    }
  });

  it("releases a lease when acquisition finishes after unmount", async () => {
    let resolveBegin!: (lease: QueueEditLease) => void;
    const pendingBegin = new Promise<QueueEditLease>((resolve) => {
      resolveBegin = resolve;
    });
    vi.mocked(beginQueuedMessageEdit).mockReturnValueOnce(pendingBegin);
    const { result, unmount } = renderHook(() =>
      useQueueEditProtection({
        sessionId: SESSION_A,
        entries: [entry()],
      }),
    );

    let pendingEdit!: Promise<string | false>;
    act(() => {
      pendingEdit = result.current.beginEdit(ENTRY_ID);
    });
    unmount();

    await act(async () => {
      resolveBegin({
        session_id: SESSION_A,
        entry_id: ENTRY_ID,
        lease_id: "lease-after-unmount",
        target_revision: 0,
      });
      await expect(pendingEdit).resolves.toBe(false);
    });

    expect(endQueuedMessageEdit).toHaveBeenCalledWith(
      expect.objectContaining({ lease_id: "lease-after-unmount" }),
    );
  });
});
it("does not let stale session completion release a replacement lease", async () => {
  const replacementLease: QueueEditLease = {
    session_id: SESSION_B,
    entry_id: ENTRY_ID,
    lease_id: "lease-b",
    target_revision: 0,
  };
  const { result, rerender } = renderHook(
    ({ sessionId }: { sessionId: string }) =>
      useQueueEditProtection({
        sessionId,
        entries: [{ ...entry(), session_id: sessionId }],
      }),
    { initialProps: { sessionId: SESSION_A } },
  );

  await act(async () => {
    expect(await result.current.beginEdit(ENTRY_ID)).toBeTruthy();
  });
  rerender({ sessionId: SESSION_B });
  await waitFor(() =>
    expect(endQueuedMessageEdit).toHaveBeenCalledWith(
      expect.objectContaining({
        lease_id: "lease-1",
      }),
    ),
  );
  vi.mocked(beginQueuedMessageEdit).mockResolvedValueOnce(replacementLease);

  await act(async () => {
    expect(await result.current.beginEdit(ENTRY_ID)).toBeTruthy();
  });
  await act(async () => {
    await result.current.completeEdit(ENTRY_ID, SESSION_A);
  });

  expect(result.current.editLease?.lease_id).toBe("lease-b");
  expect(result.current.editingEntryId).toBe(ENTRY_ID);
  expect(endQueuedMessageEdit).not.toHaveBeenCalledWith(
    expect.objectContaining({
      lease_id: "lease-b",
    }),
  );
});
it("does not let stale same-session completion release a replacement lease", async () => {
  const { result } = renderHook(() =>
    useQueueEditProtection({
      sessionId: SESSION_A,
      entries: [entry()],
    }),
  );

  let firstToken!: string;
  await act(async () => {
    firstToken = (await result.current.beginEdit(ENTRY_ID)) as string;
  });
  await act(async () => {
    await result.current.completeEdit(ENTRY_ID, SESSION_A, firstToken);
  });

  const replacementLease: QueueEditLease = {
    session_id: SESSION_A,
    entry_id: ENTRY_ID,
    lease_id: "lease-2",
    target_revision: 0,
  };
  vi.mocked(beginQueuedMessageEdit).mockResolvedValueOnce(replacementLease);
  let secondToken!: string;
  await act(async () => {
    secondToken = (await result.current.beginEdit(ENTRY_ID)) as string;
  });

  await act(async () => {
    await result.current.completeEdit(ENTRY_ID, SESSION_A, firstToken);
  });

  expect(result.current.editLease?.lease_id).toBe("lease-2");
  expect(result.current.editingEntryId).toBe(ENTRY_ID);
  expect(endQueuedMessageEdit).not.toHaveBeenCalledWith(
    expect.objectContaining({ lease_id: "lease-2" }),
  );

  await act(async () => {
    await result.current.completeEdit(ENTRY_ID, SESSION_A, secondToken);
  });
});
it("keeps the lease when an overlapping older renewal fails after a newer generation succeeds", async () => {
  vi.useFakeTimers();
  try {
    let rejectOlderRenewal!: (error: Error) => void;
    const olderRenewal = new Promise<never>((_, reject) => {
      rejectOlderRenewal = reject;
    });
    vi.mocked(renewQueuedMessageEdit).mockReturnValueOnce(olderRenewal).mockResolvedValue({
      session_id: SESSION_A,
      entry_id: ENTRY_ID,
      lease_id: "lease-1",
      target_revision: 0,
      lease_generation: 2,
    });
    const { result } = renderHook(() =>
      useQueueEditProtection({
        sessionId: SESSION_A,
        entries: [entry()],
      }),
    );

    await act(async () => {
      expect(await result.current.beginEdit(ENTRY_ID)).toBeTruthy();
    });
    await act(async () => {
      vi.advanceTimersByTime(20_000);
      await Promise.resolve();
    });
    await act(async () => {
      vi.advanceTimersByTime(20_000);
      await Promise.resolve();
    });
    expect(renewQueuedMessageEdit).toHaveBeenCalledTimes(2);

    await act(async () => {
      rejectOlderRenewal(new Error("older renewal failed"));
      await Promise.resolve();
    });

    expect(result.current.editLease?.lease_generation).toBe(2);
    expect(result.current.editingEntryId).toBe(ENTRY_ID);
  } finally {
    vi.useRealTimers();
  }
});

it("ignores out-of-order and incomplete successful renewals", async () => {
  vi.useFakeTimers();
  try {
    let resolveOlder!: (lease: QueueEditLease) => void;
    let resolveNewer!: (lease: QueueEditLease) => void;
    let resolveIncomplete!: (lease: QueueEditLease) => void;
    const older = new Promise<QueueEditLease>((resolve) => {
      resolveOlder = resolve;
    });
    const newer = new Promise<QueueEditLease>((resolve) => {
      resolveNewer = resolve;
    });
    const incomplete = new Promise<QueueEditLease>((resolve) => {
      resolveIncomplete = resolve;
    });
    vi.mocked(beginQueuedMessageEdit).mockResolvedValueOnce({
      session_id: SESSION_A,
      entry_id: ENTRY_ID,
      lease_id: "lease-1",
      target_revision: 0,
      lease_generation: 1,
    });
    vi.mocked(renewQueuedMessageEdit)
      .mockReturnValueOnce(older)
      .mockReturnValueOnce(newer)
      .mockReturnValueOnce(incomplete);
    const { result } = renderHook(() =>
      useQueueEditProtection({
        sessionId: SESSION_A,
        entries: [entry()],
      }),
    );

    await act(async () => {
      expect(await result.current.beginEdit(ENTRY_ID)).toBeTruthy();
    });
    await act(async () => {
      vi.advanceTimersByTime(60_000);
      await Promise.resolve();
    });
    expect(renewQueuedMessageEdit).toHaveBeenCalledTimes(3);

    resolveNewer({
      session_id: SESSION_A,
      entry_id: ENTRY_ID,
      lease_id: "lease-1",
      target_revision: 0,
      lease_generation: 3,
    });
    await act(async () => {
      await Promise.resolve();
    });
    resolveOlder({
      session_id: SESSION_A,
      entry_id: ENTRY_ID,
      lease_id: "lease-1",
      target_revision: 0,
      lease_generation: 2,
    });
    await act(async () => {
      await Promise.resolve();
    });
    expect(result.current.editLease?.lease_generation).toBe(3);

    resolveIncomplete({
      session_id: SESSION_A,
      entry_id: ENTRY_ID,
      lease_id: "lease-1",
      target_revision: 0,
    });
    await act(async () => {
      await Promise.resolve();
    });
    expect(result.current.editLease?.lease_generation).toBe(3);
  } finally {
    vi.useRealTimers();
  }
});
it("fences overlapping renewals when lease generations are unavailable", async () => {
  vi.useFakeTimers();
  try {
    let resolveOlder!: (lease: QueueEditLease) => void;
    let resolveNewer!: (lease: QueueEditLease) => void;
    vi.mocked(renewQueuedMessageEdit)
      .mockReturnValueOnce(
        new Promise<QueueEditLease>((resolve) => {
          resolveOlder = resolve;
        }),
      )
      .mockReturnValueOnce(
        new Promise<QueueEditLease>((resolve) => {
          resolveNewer = resolve;
        }),
      );
    const { result } = renderHook(() =>
      useQueueEditProtection({
        sessionId: SESSION_A,
        entries: [entry()],
      }),
    );

    await act(async () => {
      expect(await result.current.beginEdit(ENTRY_ID)).toBeTruthy();
    });
    await act(async () => {
      vi.advanceTimersByTime(40_000);
      await Promise.resolve();
    });
    expect(renewQueuedMessageEdit).toHaveBeenCalledTimes(2);

    resolveNewer({
      session_id: SESSION_A,
      entry_id: ENTRY_ID,
      lease_id: "lease-1",
      target_revision: 0,
      expires_at: "new",
    });
    await act(async () => {
      await Promise.resolve();
    });
    resolveOlder({
      session_id: SESSION_A,
      entry_id: ENTRY_ID,
      lease_id: "lease-1",
      target_revision: 0,
      expires_at: "old",
    });
    await act(async () => {
      await Promise.resolve();
    });

    expect(result.current.editLease?.expires_at).toBe("new");
  } finally {
    vi.useRealTimers();
  }
});
it("requests dispatch only when completing a successful save", async () => {
  const { result } = renderHook(() =>
    useQueueEditProtection({
      sessionId: SESSION_A,
      entries: [entry()],
    }),
  );

  await act(async () => {
    await result.current.beginEdit(ENTRY_ID);
    await result.current.completeEdit(ENTRY_ID, SESSION_A, "queue-edit-1", true);
  });

  expect(endQueuedMessageEdit).toHaveBeenCalledWith(
    expect.objectContaining({
      session_id: SESSION_A,
      entry_id: ENTRY_ID,
      lease_id: "lease-1",
    }),
    true,
  );
});
