import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { createElement } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

const api = vi.hoisted(() => ({ listProjectMembers: vi.fn() }));
vi.mock("@/lib/api/domains/gitlab-api", () => api);
import { toggleMemberId } from "./mr-reviewer-control";
import { MRReviewerControl } from "./mr-reviewer-control";

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((done) => {
    resolve = done;
  });
  return { promise, resolve };
}

describe("toggleMemberId", () => {
  it("adds a numeric GitLab member id once", () => {
    expect(toggleMemberId([4], 8)).toEqual([4, 8]);
  });

  it("removes an already selected member id", () => {
    expect(toggleMemberId([4, 8], 4)).toEqual([8]);
  });

  it("ignores a delayed member result after the MR identity changes", async () => {
    const first =
      deferred<Array<{ id: number; username: string; name: string; avatar_url: string }>>();
    const second =
      deferred<Array<{ id: number; username: string; name: string; avatar_url: string }>>();
    api.listProjectMembers.mockImplementation((_workspace: string, project: string) =>
      project === "group/a" ? first.promise : second.promise,
    );
    const props = {
      workspaceId: "ws",
      host: "https://gitlab.example",
      label: "Reviewers" as const,
      current: [],
      busy: false,
      onSave: vi.fn().mockResolvedValue(true),
    };
    const { rerender } = render(createElement(MRReviewerControl, { ...props, project: "group/a" }));
    fireEvent.click(screen.getByRole("button", { name: "Search reviewers" }));
    rerender(createElement(MRReviewerControl, { ...props, project: "group/b" }));
    fireEvent.click(screen.getByRole("button", { name: "Search reviewers" }));

    act(() => second.resolve([{ id: 2, username: "bob", name: "Bob", avatar_url: "" }]));
    await screen.findByText("@bob");
    act(() => first.resolve([{ id: 1, username: "alice", name: "Alice", avatar_url: "" }]));
    await waitFor(() => expect(screen.queryByText("@alice")).toBeNull());
  });
});
