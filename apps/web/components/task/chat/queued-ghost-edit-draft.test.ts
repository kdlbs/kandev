import { afterEach, describe, expect, it } from "vitest";
import {
  clearQueuedGhostEditDraft,
  readQueuedGhostEditDraft,
  writeQueuedGhostEditDraft,
} from "./queued-ghost-edit-draft";

const SESSION_ID = "session-draft";
const ENTRY_ID = "queue-entry-draft";

function reference(selectionId: string) {
  return {
    version: 1 as const,
    ref: "mention:v1:github:pull_request:github.com%2Facme%2Fapi:PR_kwDOAA",
    provider: "github",
    kind: "pull_request",
    id: "PR_kwDOAA",
    key: "acme/api#17",
    title: "Accepted PR",
    url: "https://github.com/acme/api/pull/17",
    scope: "github.com/acme/api",
    selection_id: selectionId,
    association_mode: "link_on_send" as const,
  };
}

describe("queued ghost edit draft storage", () => {
  afterEach(() => window.sessionStorage.clear());

  it("restores unsaved content and selection IDs for the same session and entry", () => {
    const draft = {
      content: "unchanged queued text",
      references: [reference("selection-after-reselect")],
    };
    writeQueuedGhostEditDraft(SESSION_ID, ENTRY_ID, draft);

    expect(readQueuedGhostEditDraft(SESSION_ID, ENTRY_ID)).toEqual(draft);
    expect(readQueuedGhostEditDraft(SESSION_ID, "another-entry")).toBeNull();
    expect(readQueuedGhostEditDraft("another-session", ENTRY_ID)).toBeNull();
  });

  it("ignores malformed data and removes completed drafts", () => {
    writeQueuedGhostEditDraft(SESSION_ID, ENTRY_ID, {
      content: "changed",
      references: [reference("selection-after-reselect")],
    });
    clearQueuedGhostEditDraft(SESSION_ID, ENTRY_ID);
    expect(readQueuedGhostEditDraft(SESSION_ID, ENTRY_ID)).toBeNull();

    window.sessionStorage.setItem("kandev:queued-edit-draft:session-draft:queue-entry-draft", "{");
    expect(readQueuedGhostEditDraft(SESSION_ID, ENTRY_ID)).toBeNull();
  });
});
