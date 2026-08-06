import { afterAll, describe, expect, it } from "vitest";
import { activateLocale, i18n } from "@/lib/i18n";
import { DIMENSION_METAS, getDimensionEnumOptions, getOpLabel } from "./filter-dimension-registry";

describe("sidebar filter dimensions", () => {
  it("offers the archived boolean dimension", () => {
    const archived = DIMENSION_METAS.find((meta) => meta.dimension === "archived");
    expect(archived).toMatchObject({
      dimension: "archived",
      valueKind: "boolean",
      ops: ["is", "is_not"],
      defaultOp: "is",
      defaultValue: true,
    });
  });
});

/**
 * The registry stores catalog KEYS, not resolved copy: it is module scope, so a
 * resolved `t()` would freeze at the boot locale. These cover the two helpers
 * that turn those keys into displayable labels, and assert that the persisted
 * `value` / `dimension` side stays English across a locale switch.
 */
describe("localized dimension labels", () => {
  afterAll(async () => {
    if (i18n.language !== "en") await activateLocale("en");
  });

  it("resolves fixed enum options to labels while keeping their values English", async () => {
    await activateLocale("en");
    const state = DIMENSION_METAS.find((m) => m.dimension === "state")!;

    const en = getDimensionEnumOptions(state)!;
    expect(en).toEqual([
      { value: "review", label: "Review" },
      { value: "in_progress", label: "In progress" },
      { value: "backlog", label: "Backlog" },
    ]);

    await activateLocale("pseudo");
    const pseudo = getDimensionEnumOptions(state)!;
    // Values are persisted in a saved view — they must not move.
    expect(pseudo.map((o) => o.value)).toEqual(["review", "in_progress", "backlog"]);
    expect(pseudo[0].label).not.toBe("Review");
    expect(pseudo[0].label).toBe(i18n.t("task:filterStateReview"));
  });

  it("returns undefined for a dimension with no fixed options", () => {
    const workflow = DIMENSION_METAS.find((m) => m.dimension === "workflow")!;
    expect(getDimensionEnumOptions(workflow)).toBeUndefined();
  });

  it("uses Show/Hide for boolean dimensions and the plain verb otherwise", async () => {
    await activateLocale("en");
    expect(getOpLabel("is", "boolean")).toBe("Show");
    expect(getOpLabel("is_not", "boolean")).toBe("Hide");
    expect(getOpLabel("in", "boolean")).toBe("in");
    expect(getOpLabel("is", "enum")).toBe("is");
    expect(getOpLabel("is_not", "text")).toBe("is not");
    expect(getOpLabel("matches", "text")).toBe("contains");
    expect(getOpLabel("not_matches", "text")).toBe("does not contain");
  });

  it("follows a locale switch", async () => {
    await activateLocale("pseudo");
    expect(getOpLabel("is", "boolean")).toBe(i18n.t("task:filterOpShow"));
    expect(getOpLabel("matches", "text")).toBe(i18n.t("task:filterOpContains"));
  });
});
