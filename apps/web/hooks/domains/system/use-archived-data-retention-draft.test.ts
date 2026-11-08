import { describe, expect, it } from "vitest";
import type { ArchivedDataPolicy } from "@/lib/types/archived-data-retention";
import { rebaseArchivedDraft } from "./use-archived-data-retention-draft";

const policy = (overrides: Partial<ArchivedDataPolicy> = {}): ArchivedDataPolicy => ({
  enabled: false,
  archived_age: { days: 14 },
  cleanup_age: { days: 7 },
  revision: 2,
  ...overrides,
});

describe("rebaseArchivedDraft", () => {
  it("follows the saved policy when the draft is unedited", () => {
    const previous = policy();
    const saved = policy({ enabled: true, revision: 3 });
    expect(rebaseArchivedDraft(previous, previous, saved)).toBe(saved);
    expect(rebaseArchivedDraft(null, previous, saved)).toBe(saved);
  });

  it("keeps an age edit but takes the enable flag a preparation committed", () => {
    // During preparation the saved policy reads enabled=false; preparation then
    // enables it. The edited window must survive without the stale flag.
    const previous = policy();
    const draft = policy({ archived_age: { days: 30 } });
    const saved = policy({ enabled: true });
    expect(rebaseArchivedDraft(draft, previous, saved)).toEqual(
      policy({ enabled: true, archived_age: { days: 30 } }),
    );
  });

  it("keeps an edited window on the revision a cancellation advanced", () => {
    const previous = policy({ enabled: true });
    const draft = policy({ enabled: true, cleanup_age: { days: 10 } });
    const saved = policy({ enabled: false, revision: 3 });
    expect(rebaseArchivedDraft(draft, previous, saved)).toEqual(
      policy({ enabled: false, cleanup_age: { days: 10 }, revision: 3 }),
    );
  });

  it("keeps an explicit enable change", () => {
    const previous = policy();
    const draft = policy({ enabled: true });
    const saved = policy({ archived_age: { days: 20 }, revision: 3 });
    expect(rebaseArchivedDraft(draft, previous, saved)).toEqual(
      policy({ enabled: true, archived_age: { days: 20 }, revision: 3 }),
    );
  });
});
