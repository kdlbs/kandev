import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, renderHook } from "@testing-library/react";
import type { ActivityItem } from "@/lib/api/domains/coordinator-activity-api";

const listMembers = vi.hoisted(() => vi.fn());
vi.mock("@/lib/api/domains/team-access-api", () => ({ listWorkspaceMembers: listMembers }));

import { resolvePerson, SECOND_READ_AFTER_MS, useActivityMembers } from "./use-activity-members";

const rowBy = (actor: string | null, undoneBy: string | null = null) =>
  ({ actor_user_id: actor, undone_by: undoneBy }) as ActivityItem;

const members = (...names: Array<[string, string]>) => ({
  members: names.map(([user_id, display_name]) => ({ user_id, display_name })),
});

beforeEach(() => {
  vi.useFakeTimers();
  listMembers.mockReset();
});
afterEach(() => vi.useRealTimers());

async function mountLoaded(first = members(["u-1", "Ana"])) {
  listMembers.mockResolvedValueOnce(first);
  const hook = renderHook(() => useActivityMembers("ws-1"));
  await act(async () => undefined);
  return hook;
}

describe("resolvePerson", () => {
  const loaded = {
    status: "loaded" as const,
    names: new Map([
      ["u-1", "Ana"],
      ["u-2", ""],
    ]),
  };
  it("names a listed member, and treats no id or an empty name as no person", () => {
    expect(resolvePerson(loaded, "u-1")).toEqual({ kind: "named", name: "Ana" });
    expect(resolvePerson(loaded, null)).toEqual({ kind: "none" });
    expect(resolvePerson(loaded, "u-2")).toEqual({ kind: "none" });
  });
  it("calls an absent id a former member only once the list has loaded", () => {
    expect(resolvePerson(loaded, "u-9")).toEqual({ kind: "former" });
    expect(resolvePerson({ status: "loading", names: new Map() }, "u-9")).toEqual({ kind: "none" });
    expect(resolvePerson({ status: "failed", names: new Map() }, "u-9")).toEqual({ kind: "none" });
  });
});

describe("useActivityMembers", () => {
  it("reads the list once on mount", async () => {
    const { result } = await mountLoaded();
    expect(listMembers).toHaveBeenCalledTimes(1);
    expect(result.current.status).toBe("loaded");
    expect(result.current.resolve("u-1")).toEqual({ kind: "named", name: "Ana" });
  });

  it("does not re-read for an absent person within 30 seconds", async () => {
    const { result } = await mountLoaded();
    act(() => vi.advanceTimersByTime(SECOND_READ_AFTER_MS));
    act(() => result.current.noteArrival([rowBy("u-9")]));
    expect(listMembers).toHaveBeenCalledTimes(1);
  });

  it("re-reads once after 30 seconds when an arrival names an absent person, then never again", async () => {
    const { result } = await mountLoaded();
    act(() => vi.advanceTimersByTime(SECOND_READ_AFTER_MS + 1));
    listMembers.mockResolvedValueOnce(members(["u-1", "Ana"], ["u-9", "Cy"]));
    act(() => result.current.noteArrival([rowBy("u-1", "u-9")]));
    expect(listMembers).toHaveBeenCalledTimes(2);
    await act(async () => undefined);
    expect(result.current.resolve("u-9")).toEqual({ kind: "named", name: "Cy" });
    act(() => result.current.noteArrival([rowBy("u-8")]));
    expect(listMembers).toHaveBeenCalledTimes(2);
  });

  it("does not re-read when every recorded person is present or there are none", async () => {
    const { result } = await mountLoaded();
    act(() => vi.advanceTimersByTime(SECOND_READ_AFTER_MS + 1));
    act(() => result.current.noteArrival([rowBy("u-1"), rowBy(null)]));
    expect(listMembers).toHaveBeenCalledTimes(1);
  });

  it("re-reads after a failed first read, and a failed second read reports failed", async () => {
    listMembers.mockRejectedValueOnce(new Error("x"));
    const { result } = renderHook(() => useActivityMembers("ws-1"));
    await act(async () => undefined);
    expect(result.current.status).toBe("failed");
    act(() => vi.advanceTimersByTime(SECOND_READ_AFTER_MS + 1));
    listMembers.mockRejectedValueOnce(new Error("y"));
    act(() => result.current.noteArrival([rowBy("u-1")]));
    expect(listMembers).toHaveBeenCalledTimes(2);
    await act(async () => undefined);
    expect(result.current.status).toBe("failed");
  });

  it("does not start a second read while one is in flight", async () => {
    listMembers.mockReturnValue(new Promise(() => undefined));
    const { result } = renderHook(() => useActivityMembers("ws-1"));
    act(() => vi.advanceTimersByTime(SECOND_READ_AFTER_MS + 1));
    act(() => result.current.noteArrival([rowBy("u-1")]));
    expect(listMembers).toHaveBeenCalledTimes(1);
  });
});
