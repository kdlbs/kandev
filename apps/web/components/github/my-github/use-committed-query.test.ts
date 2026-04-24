import { describe, it, expect } from "vitest";
import { act, renderHook } from "@testing-library/react";
import { useCommittedQuery } from "./use-committed-query";

describe("useCommittedQuery", () => {
  it("initializes draft and committed to the initial value", () => {
    const { result } = renderHook(() => useCommittedQuery("hello"));
    expect(result.current.draft).toBe("hello");
    expect(result.current.committed).toBe("hello");
  });

  it("setDraft updates draft without touching committed", () => {
    const { result } = renderHook(() => useCommittedQuery(""));
    act(() => result.current.setDraft("typed"));
    expect(result.current.draft).toBe("typed");
    expect(result.current.committed).toBe("");
  });

  it("commit syncs committed to the current draft", () => {
    const { result } = renderHook(() => useCommittedQuery(""));
    act(() => result.current.setDraft("query"));
    act(() => result.current.commit());
    expect(result.current.draft).toBe("query");
    expect(result.current.committed).toBe("query");
  });

  it("setImmediate updates draft and committed together", () => {
    const { result } = renderHook(() => useCommittedQuery(""));
    act(() => result.current.setImmediate("now"));
    expect(result.current.draft).toBe("now");
    expect(result.current.committed).toBe("now");
  });

  it("commit after setImmediate is a no-op for committed", () => {
    const { result } = renderHook(() => useCommittedQuery(""));
    act(() => result.current.setImmediate("abc"));
    act(() => result.current.commit());
    expect(result.current.committed).toBe("abc");
  });
});
