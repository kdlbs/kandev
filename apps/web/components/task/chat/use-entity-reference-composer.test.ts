import { cleanup, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

vi.mock("@/hooks/use-entity-reference-search", () => ({
  useEntityReferenceSearch: () => ({
    groups: [],
    isSearching: false,
    error: null,
    retry: vi.fn(),
  }),
}));

import { useEntityReferenceComposer } from "./use-entity-reference-composer";

afterEach(cleanup);

describe("useEntityReferenceComposer lifecycle", () => {
  it("installs the # suggestion on an enabled surface before workspace hydration", () => {
    const { result } = renderHook(() =>
      useEntityReferenceComposer({
        enabled: true,
        workspaceId: null,
        sessionId: "session-1",
      }),
    );

    expect(result.current.suggestion).toBeDefined();
    expect(result.current.isOpen).toBe(false);
  });

  it("does not install the # suggestion on an out-of-scope surface", () => {
    const { result } = renderHook(() =>
      useEntityReferenceComposer({
        enabled: false,
        workspaceId: "workspace-1",
        sessionId: "session-1",
      }),
    );

    expect(result.current.suggestion).toBeUndefined();
  });
});
