import { describe, expect, it } from "vitest";
import type { CustomPrompt } from "@/lib/types/http";
import {
  choiceFor,
  draftForm,
  formFromPrompt,
  hasRepositoryVersion,
  isDraftValid,
  isLinkedPrompt,
  planPromptSaves,
  repositoryVersion,
  showsEditor,
  withChoice,
  withDraftEdit,
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

  it("tracks a draft only while it differs from the saved prompt", () => {
    const edited = withDraftEdit({}, override, { content: "Edited" });
    expect(edited).toEqual({
      p2: { form: { name: "other", content: "Edited" }, updatedAt: override.updated_at },
    });
    expect(draftForm(override, edited)).toEqual({ name: "other", content: "Edited" });
    expect(draftForm(synced, edited)).toEqual({ name: "hook", content: "Mine" });
    expect(withDraftEdit(edited, override, { content: "Mine" })).toEqual({});
  });

  it("keeps the revision a draft started from through later edits", () => {
    const first = withDraftEdit({}, manual, { content: "A" });
    const newer = { ...manual, updated_at: "2026-02-01T00:00:00Z" };
    expect(withDraftEdit(first, newer, { content: "AB" }).p3!.updatedAt).toBe(manual.updated_at);
  });

  it("edits a manual prompt, or a linked prompt switched to Local", () => {
    expect(showsEditor(manual, {})).toBe(true);
    expect(showsEditor(override, {})).toBe(true);
    expect(showsEditor(synced, {})).toBe(false);
    expect(showsEditor(synced, { p1: "local" })).toBe(true);
    expect(showsEditor(override, { p2: "repository" })).toBe(false);
  });

  it("requires a name for a manual draft and content for every draft", () => {
    expect(isDraftValid(manual, { name: " ", content: "x" })).toBe(false);
    expect(isDraftValid(manual, { name: "n", content: " " })).toBe(false);
    expect(isDraftValid(synced, { name: "hook", content: "x" })).toBe(true);
    expect(isDraftValid(synced, { name: "hook", content: " " })).toBe(false);
  });

  it("plans one save per switched or edited prompt", () => {
    const plan = planPromptSaves({
      prompts: [synced, override, manual],
      pending: { p1: "local", p2: "repository" },
      drafts: {
        p2: { form: { name: "other", content: " Edited " }, updatedAt: "rev-2" },
        p3: { form: { name: "notes", content: "Changed" }, updatedAt: "rev-3" },
      },
    });
    expect(plan).toEqual([
      { kind: "versions", prompt: synced, payload: { use_local: true } },
      {
        kind: "versions",
        prompt: override,
        payload: { use_local: false, local_content: "Edited", expected_updated_at: "rev-2" },
      },
      {
        kind: "update",
        prompt: manual,
        form: { name: "notes", content: "Changed" },
        expectedUpdatedAt: "rev-3",
      },
    ]);
  });

  it("saves an edited linked prompt without a pending switch", () => {
    expect(
      planPromptSaves({
        prompts: [override],
        pending: {},
        drafts: { p2: { form: { name: "other", content: "New" }, updatedAt: "rev" } },
      }),
    ).toEqual([
      {
        kind: "versions",
        prompt: override,
        payload: { use_local: true, local_content: "New", expected_updated_at: "rev" },
      },
    ]);
  });

  it("plans nothing for a draft equal to the saved prompt or for a removed prompt", () => {
    expect(
      planPromptSaves({
        prompts: [override],
        pending: {},
        drafts: {
          p2: { form: formFromPrompt(override), updatedAt: "rev" },
          gone: { form: { name: "x", content: "y" }, updatedAt: "rev" },
        },
      }),
    ).toEqual([]);
  });
});

// @covers AC-INTEGRATIONS-PROMPT-SYNC-009.1
describe("hasRepositoryVersion", () => {
  it("has no repository version for an override whose file left the source", () => {
    expect(hasRepositoryVersion(synced)).toBe(true);
    expect(hasRepositoryVersion(override)).toBe(true);
    expect(hasRepositoryVersion({ ...override, alternate_content: "" })).toBe(false);
    expect(hasRepositoryVersion(manual)).toBe(false);
  });
});
