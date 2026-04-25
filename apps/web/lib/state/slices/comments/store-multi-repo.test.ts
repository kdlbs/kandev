import { describe, it, expect, beforeEach } from "vitest";
import { useCommentsStore } from "./comments-store";
import type { DiffComment } from "./types";

function diffComment(overrides: Partial<DiffComment>): DiffComment {
  return {
    id: "c-" + Math.random().toString(36).slice(2),
    sessionId: "sess",
    source: "diff",
    text: "looks off",
    filePath: "src/app.tsx",
    startLine: 1,
    endLine: 1,
    side: "additions",
    codeContent: "const x = 1;",
    createdAt: new Date().toISOString(),
    status: "pending",
    ...overrides,
  };
}

describe("getCommentsForFile (multi-repo)", () => {
  beforeEach(() => {
    useCommentsStore.setState({
      byId: {},
      bySession: {},
      pendingForChat: [],
      editingCommentId: null,
    });
  });

  it("filters by repositoryId when provided", () => {
    const store = useCommentsStore.getState();
    store.addComment(diffComment({ id: "front", repositoryId: "repo-front" }));
    store.addComment(diffComment({ id: "back", repositoryId: "repo-back" }));

    const front = useCommentsStore
      .getState()
      .getCommentsForFile("sess", "src/app.tsx", "repo-front");
    expect(front.map((c) => c.id)).toEqual(["front"]);

    const back = useCommentsStore
      .getState()
      .getCommentsForFile("sess", "src/app.tsx", "repo-back");
    expect(back.map((c) => c.id)).toEqual(["back"]);
  });

  it("returns all matching when repositoryId is omitted", () => {
    const store = useCommentsStore.getState();
    store.addComment(diffComment({ id: "front", repositoryId: "repo-front" }));
    store.addComment(diffComment({ id: "back", repositoryId: "repo-back" }));

    const all = useCommentsStore.getState().getCommentsForFile("sess", "src/app.tsx");
    expect(all.map((c) => c.id).sort()).toEqual(["back", "front"]);
  });

  it("legacy comments without repositoryId match any repo filter", () => {
    const store = useCommentsStore.getState();
    store.addComment(diffComment({ id: "legacy" })); // no repositoryId

    const result = useCommentsStore
      .getState()
      .getCommentsForFile("sess", "src/app.tsx", "repo-front");
    expect(result.map((c) => c.id)).toEqual(["legacy"]);
  });
});
