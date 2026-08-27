import { cleanup, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { EntityReferenceSearchGroup } from "@/lib/types/entity-reference";

const useEntityReferenceSearchMock = vi.fn();

vi.mock("@/hooks/use-entity-reference-search", () => ({
  useEntityReferenceSearch: (...args: unknown[]) => useEntityReferenceSearchMock(...args),
}));

import { useEntityReferenceComposer } from "./use-entity-reference-composer";

afterEach(cleanup);

beforeEach(() => {
  useEntityReferenceSearchMock.mockReturnValue({
    groups: [],
    isSearching: false,
    error: null,
    retry: vi.fn(),
  });
});

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

  it("keeps plugin pull-request search groups on the generic # composer path", () => {
    const groups: EntityReferenceSearchGroup[] = [
      {
        source: "plugin:example:pull-requests",
        provider: "plugin:example:reviews",
        kind: "pull_request",
        display_name: "Example reviews",
        kind_label: "Pull request",
        status: "ok",
        results: [
          {
            version: 1,
            ref: "mention:v1:plugin%3Aexample%3Areviews:pull_request:workspace-1:pull-42",
            provider: "plugin:example:reviews",
            kind: "pull_request",
            id: "pull-42",
            key: "PR-42",
            title: "Fix authentication",
            url: "https://reviews.example.test/pull-requests/42",
            scope: "workspace-1",
          },
        ],
      },
    ];
    useEntityReferenceSearchMock.mockReturnValue({
      groups,
      isSearching: false,
      error: null,
      retry: vi.fn(),
    });

    const { result } = renderHook(() =>
      useEntityReferenceComposer({
        enabled: true,
        workspaceId: "workspace-1",
        sessionId: "session-1",
      }),
    );

    expect(result.current.suggestion).toBeDefined();
    expect(result.current.groups).toEqual(groups);
  });
});
