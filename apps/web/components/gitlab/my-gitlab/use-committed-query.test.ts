import { describe, it, expect } from "vitest";
import { act, renderHook } from "@testing-library/react";
import { useCommittedQuery } from "./use-committed-query";

const BUG = "labels=bug";

describe("useCommittedQuery (gitlab)", () => {
  it("initializes draft and committed to the initial value", () => {
    const { result } = renderHook(() => useCommittedQuery(BUG));
    expect(result.current.draft).toBe(BUG);
    expect(result.current.committed).toBe(BUG);
  });

  it("setDraft updates draft without touching committed", () => {
    const { result } = renderHook(() => useCommittedQuery(""));
    act(() => result.current.setDraft(BUG));
    expect(result.current.draft).toBe(BUG);
    expect(result.current.committed).toBe("");
  });

  it("commit syncs committed to the current draft", () => {
    const { result } = renderHook(() => useCommittedQuery(""));
    act(() => result.current.setDraft("scope=all"));
    act(() => result.current.commit());
    expect(result.current.committed).toBe("scope=all");
  });

  it("setImmediate updates draft and committed together", () => {
    const { result } = renderHook(() => useCommittedQuery(""));
    act(() => result.current.setImmediate("now"));
    expect(result.current.draft).toBe("now");
    expect(result.current.committed).toBe("now");
  });
});
