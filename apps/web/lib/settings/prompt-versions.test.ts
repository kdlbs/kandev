import { describe, expect, it } from "vitest";
import type { CustomPrompt } from "@/lib/types/http";
import {
  choiceFor,
  formFromPrompt,
  isLinkedPrompt,
  planPromptSaves,
  repositoryVersion,
  withChoice,
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
  id: "p2",
  name: "other",
  source: "override",
  content: "Mine",
  alternate_content: "Repo",
};
const manual: CustomPrompt = { ...base, id: "p3", name: "notes", source: "manual" };

// @covers AC-INTEGRATIONS-PROMPT-SYNC-007.1 AC-INTEGRATIONS-PROMPT-SYNC-009.2
describe("prompt versions", () => {
  it("edits a manual prompt's content, and a linked prompt's local version", () => {
    expect(isLinkedPrompt(manual)).toBe(false);
    expect(formFromPrompt(manual)).toEqual({ name: "notes", content: "Repo" });
    expect(formFromPrompt(synced)).toEqual({ name: "hook", content: "Mine" });
    expect(formFromPrompt(override)).toEqual({ name: "other", content: "Mine" });
    expect(repositoryVersion(synced)).toBe("Repo");
    expect(repositoryVersion(override)).toBe("Repo");
  });

  it("starts the local version from the synced content when none is kept", () => {
    expect(formFromPrompt({ ...base, source: "sync" }).content).toBe("Repo");
    expect(repositoryVersion({ ...base, source: "override" })).toBe("Repo");
  });

  it("keeps a pending choice only while it differs from the saved one", () => {
    expect(choiceFor(synced, {})).toBe("repository");
    expect(choiceFor(override, {})).toBe("local");
    const pending = withChoice({}, synced, "local");
    expect(pending).toEqual({ p1: "local" });
    expect(choiceFor(synced, pending)).toBe("local");
    expect(withChoice(pending, synced, "repository")).toEqual({});
  });

  it("plans one save per switched prompt, with the open prompt's edit", () => {
    const plan = planPromptSaves({
      prompts: [synced, override, manual],
      pending: { p1: "local", p2: "repository" },
      editing: {
        prompt: override,
        form: { name: "other", content: " Edited " },
        updatedAt: "rev-2",
      },
    });
    expect(plan).toEqual([
      { kind: "versions", prompt: synced, payload: { use_local: true } },
      {
        kind: "versions",
        prompt: override,
        payload: { use_local: false, local_content: "Edited", expected_updated_at: "rev-2" },
      },
    ]);
  });

  it("saves an edit to an open prompt without a pending switch", () => {
    expect(
      planPromptSaves({
        prompts: [override, manual],
        pending: {},
        editing: { prompt: override, form: { name: "other", content: "New" }, updatedAt: "rev" },
      }),
    ).toEqual([
      {
        kind: "versions",
        prompt: override,
        payload: { use_local: true, local_content: "New", expected_updated_at: "rev" },
      },
    ]);
    const form = { name: "notes", content: "Changed" };
    expect(
      planPromptSaves({
        prompts: [manual],
        pending: {},
        editing: { prompt: manual, form, updatedAt: "r" },
      }),
    ).toEqual([{ kind: "update", prompt: manual, form, expectedUpdatedAt: "r" }]);
  });

  it("plans nothing for an open prompt left unchanged", () => {
    expect(
      planPromptSaves({
        prompts: [override],
        pending: {},
        editing: { prompt: override, form: formFromPrompt(override), updatedAt: "rev" },
      }),
    ).toEqual([]);
  });
});
