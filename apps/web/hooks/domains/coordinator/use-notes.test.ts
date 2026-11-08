import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

const api = { listNotes: vi.fn(), getNote: vi.fn(), getNoteReferences: vi.fn() };
vi.mock("@/lib/api/domains/coordinator-notes-api", () => ({
  listNotes: (...a: unknown[]) => api.listNotes(...a),
  getNote: (...a: unknown[]) => api.getNote(...a),
  getNoteReferences: (...a: unknown[]) => api.getNoteReferences(...a),
}));

import { useNoteDetail, useNotes } from "./use-notes";

const NOTE = { id: "n1" };

beforeEach(() => Object.values(api).forEach((m) => m.mockReset()));

describe("useNotes", () => {
  it("reports an error only while nothing has loaded, and keeps the list after a failed reload", async () => {
    api.listNotes.mockRejectedValueOnce(new Error("down"));
    const { result } = renderHook(() => useNotes("ws", "co", false));
    await waitFor(() => expect(result.current.status).toBe("error"));

    api.listNotes.mockResolvedValueOnce([NOTE]);
    await act(async () => result.current.retry());
    await waitFor(() => expect(result.current.status).toBe("ready"));

    api.listNotes.mockRejectedValueOnce(new Error("down"));
    await act(async () => result.current.reload());
    expect(result.current.status).toBe("ready");
    expect(result.current.notes).toEqual([NOTE]);
  });

  it("lets the last request sent win", async () => {
    let releaseFirst: (v: unknown[]) => void = () => {};
    api.listNotes.mockReturnValueOnce(new Promise((r) => (releaseFirst = r)));
    api.listNotes.mockResolvedValueOnce([NOTE]);
    const { result } = renderHook(() => useNotes("ws", "co", false));
    await act(async () => result.current.reload());
    await waitFor(() => expect(result.current.notes).toEqual([NOTE]));
    await act(async () => releaseFirst([]));
    expect(result.current.notes).toEqual([NOTE]);
  });

  it("refetches with the include-deleted flag", async () => {
    api.listNotes.mockResolvedValue([]);
    const { rerender } = renderHook(({ d }) => useNotes("ws", "co", d), {
      initialProps: { d: false },
    });
    await waitFor(() => expect(api.listNotes).toHaveBeenCalledWith("ws", "co", false));
    rerender({ d: true });
    await waitFor(() => expect(api.listNotes).toHaveBeenCalledWith("ws", "co", true));
  });
});

describe("useNoteDetail", () => {
  it("loads the note with its reference history and keeps it when a refresh fails", async () => {
    api.getNote.mockResolvedValueOnce({ ...NOTE, versions: [] });
    api.getNoteReferences.mockResolvedValue([]);
    const { result } = renderHook(() => useNoteDetail("ws", "co", "n1"));
    await waitFor(() => expect(result.current.status).toBe("ready"));

    api.getNote.mockRejectedValueOnce(new Error("down"));
    await act(async () => result.current.refresh());
    expect(result.current.status).toBe("ready");
    expect(result.current.detail?.id).toBe("n1");
  });

  it("reports an error when the first read fails", async () => {
    api.getNote.mockRejectedValue(new Error("down"));
    api.getNoteReferences.mockResolvedValue([]);
    const { result } = renderHook(() => useNoteDetail("ws", "co", "n1"));
    await waitFor(() => expect(result.current.status).toBe("error"));
  });
});
