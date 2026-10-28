import { describe, expect, it } from "vitest";
import type { EntityReference } from "@/lib/types/entity-reference";
import { entityReferenceMarkdown } from "@/lib/entity-references/message-references";
import {
  queueReferenceSelectionsChanged,
  selectQueueReference,
  removeQueueReference,
} from "./queue-registration";

const RESELECTED_ID = "selection-2";

const reference: EntityReference = {
  version: 1,
  ref: "mention:v1:github:pull_request:github.com%2Facme%2Fapp:PR_1",
  provider: "github",
  kind: "pull_request",
  id: "PR_1",
  key: "acme/app#12",
  title: "Change",
  url: "https://github.com/acme/app/pull/12",
  scope: "github.com/acme/app",
  selection_id: "selection-1",
  association_mode: "link_on_send",
  registration_status: "unlinked",
};

describe("queued reference edit admission", () => {
  it("does not treat background status changes as a new selection", () => {
    expect(
      queueReferenceSelectionsChanged(
        [reference],
        [{ ...reference, registration_status: "linked" }],
      ),
    ).toBe(false);
  });
  it("admits a remove-and-reselect with identical final text", () => {
    const text = entityReferenceMarkdown(reference);
    const removed = removeQueueReference(text, [reference], reference.ref);
    const selected = selectQueueReference(removed.value, removed.references, {
      ...reference,
      selection_id: RESELECTED_ID,
    });
    expect(selected.value).toBe(text);
    expect(queueReferenceSelectionsChanged([reference], selected.references)).toBe(true);
    expect(selected.references[0].selection_id).toBe(RESELECTED_ID);
  });
  it("detects a context-only mode change even when text stays identical", () => {
    expect(
      queueReferenceSelectionsChanged(
        [reference],
        [{ ...reference, association_mode: "context_only" }],
      ),
    ).toBe(true);
  });
  it("replaces an explicitly reselected reference without duplicating its text", () => {
    const text = `Review ${entityReferenceMarkdown(reference)}`;
    const selected = selectQueueReference(text, [reference], {
      ...reference,
      selection_id: RESELECTED_ID,
    });
    expect(selected.value).toBe(text);
    expect(selected.references).toHaveLength(1);
    expect(selected.references[0].selection_id).toBe(RESELECTED_ID);
  });
  it("removes only the selected reference and keeps unrelated text", () => {
    const result = removeQueueReference(
      `Before ${entityReferenceMarkdown(reference)} after`,
      [reference],
      reference.ref,
    );
    expect(result.value).toBe("Before  after");
    expect(result.references).toEqual([]);
  });
});
