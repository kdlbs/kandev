import { describe, expect, it } from "vitest";
import type { CustomPrompt } from "@/lib/types/http";
import {
  formFromPrompt,
  isLinkedPrompt,
  repositoryVersion,
  versionsSavePayload,
} from "./prompt-versions";

const base: CustomPrompt = {
  id: "p1",
  name: "hook",
  content: "Repo",
  builtin: false,
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
};
const synced: CustomPrompt = { ...base, source: "sync", alternate_content: "Mine" };
const override: CustomPrompt = {
  ...base,
  source: "override",
  content: "Mine",
  alternate_content: "Repo",
};

// @covers AC-INTEGRATIONS-PROMPT-SYNC-007.1 AC-INTEGRATIONS-PROMPT-SYNC-009.2
describe("prompt versions", () => {
  it("edits a manual prompt's content as before", () => {
    const manual = { ...base, source: "manual" as const };
    expect(isLinkedPrompt(manual)).toBe(false);
    expect(formFromPrompt(manual)).toEqual({ name: "hook", content: "Repo" });
  });

  it("edits the local version of a synced prompt or override, with the switch state", () => {
    expect(formFromPrompt(synced)).toEqual({ name: "hook", content: "Mine", useLocal: false });
    expect(formFromPrompt(override)).toEqual({ name: "hook", content: "Mine", useLocal: true });
    expect(repositoryVersion(synced)).toBe("Repo");
    expect(repositoryVersion(override)).toBe("Repo");
  });

  it("starts the local version from the synced content when none is kept", () => {
    const fresh = { ...base, source: "sync" as const };
    expect(formFromPrompt(fresh).content).toBe("Repo");
    expect(repositoryVersion({ ...base, source: "override" as const })).toBe("Repo");
  });

  it("sends the switch, and the local version only when it was edited", () => {
    expect(
      versionsSavePayload(synced, { name: "hook", content: "Mine", useLocal: true }, "rev-1"),
    ).toEqual({
      use_local: true,
      expected_updated_at: "rev-1",
    });
    expect(
      versionsSavePayload(
        override,
        { name: "hook", content: " Edited ", useLocal: false },
        "rev-2",
      ),
    ).toEqual({ use_local: false, local_content: "Edited", expected_updated_at: "rev-2" });
  });
});
