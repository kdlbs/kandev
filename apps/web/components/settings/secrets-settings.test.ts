import { describe, expect, it } from "vitest";
import type { SecretListItem } from "@/lib/types/http-secrets";
import { getSecretDraftMeta } from "./secrets-settings";

const secret: SecretListItem = {
  id: "secret-1",
  name: "Saved secret",
  has_value: true,
  created_at: "",
  updated_at: "",
};

describe("getSecretDraftMeta", () => {
  it("treats a newly opened secret form as dirty before persistence", () => {
    const state = getSecretDraftMeta([], null, true, { name: "", value: "" });

    expect(state.isDirty).toBe(true);
    expect(state.revision.startsWith("new:")).toBe(true);
  });

  it("tracks existing secret edits against the saved name", () => {
    expect(
      getSecretDraftMeta([secret], secret.id, false, { name: secret.name, value: "" }).isDirty,
    ).toBe(false);
    expect(
      getSecretDraftMeta([secret], secret.id, false, { name: "Renamed", value: "" }).isDirty,
    ).toBe(true);
  });
});
