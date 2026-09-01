import { afterEach, describe, expect, it } from "vitest";
import { i18n } from "@/lib/i18n";
import { getCleanupSummary, getBulkCleanupSummary } from "./task-cleanup-summary";

const AGENT_STOP_EFFECT = "Any running agent sessions will be stopped.";

afterEach(async () => {
  await i18n.changeLanguage("en");
});

describe("getCleanupSummary (single task)", () => {
  it("reassures the user that local executor leaves the repo untouched", () => {
    const summary = getCleanupSummary("local");
    expect(summary.effects).toEqual([AGENT_STOP_EFFECT]);
    expect(summary.notes).toHaveLength(1);
    expect(summary.notes[0]).toMatch(/directly in your repo/i);
    expect(summary.notes[0]).toMatch(/not touched/i);
  });

  it("describes worktree deletion + branch removal", () => {
    const { effects, notes } = getCleanupSummary("worktree");
    expect(effects[0]).toMatch(/worktree/i);
    expect(effects[0]).toMatch(/branch/i);
    expect(effects[0]).toMatch(/deleted/i);
    expect(notes.join(" ")).toMatch(/not affected/i);
  });

  it("describes local_docker container removal", () => {
    const { effects, notes } = getCleanupSummary("local_docker");
    expect(effects[0]).toMatch(/Docker container/i);
    expect(effects[0]).toMatch(/removed/i);
    expect(notes.join(" ")).toMatch(/host repo/i);
  });

  it("describes remote_docker container removal", () => {
    const { effects, notes } = getCleanupSummary("remote_docker");
    expect(effects[0]).toMatch(/remote Docker container/i);
    expect(effects[0]).toMatch(/removed/i);
    expect(notes).toEqual([]);
  });

  it("warns that sprites sandbox destruction loses uncommitted work", () => {
    const { effects } = getCleanupSummary("sprites");
    expect(effects[0]).toMatch(/Sprites/i);
    expect(effects[0]).toMatch(/uncommitted work/i);
  });

  it("describes ssh remote directory removal as best-effort", () => {
    const { effects, notes } = getCleanupSummary("ssh");
    expect(effects[0]).toMatch(/remote/i);
    expect(effects[0]).toMatch(/removed/i);
    expect(notes.join(" ")).toMatch(/best-effort/i);
  });

  it("describes exact Kubernetes Pod and workspace cleanup", () => {
    const { effects } = getCleanupSummary("k8s");
    expect(effects[0]).toMatch(/Kubernetes Pod/i);
    expect(effects[0]).toMatch(/existing claims are kept/i);
  });

  it("always appends the agent-session-stop effect", () => {
    expect(getCleanupSummary("local").effects).toContain(AGENT_STOP_EFFECT);
    expect(getCleanupSummary("worktree").effects).toContain(AGENT_STOP_EFFECT);
  });

  it("falls back to a generic effect when executor type is unknown or missing", () => {
    expect(getCleanupSummary(undefined)).toEqual({
      effects: [AGENT_STOP_EFFECT],
      notes: [],
    });
    expect(getCleanupSummary(null)).toEqual({ effects: [AGENT_STOP_EFFECT], notes: [] });
    expect(getCleanupSummary("totally-made-up")).toEqual({
      effects: [AGENT_STOP_EFFECT],
      notes: [],
    });
    expect(getCleanupSummary("constructor")).toEqual({
      effects: [AGENT_STOP_EFFECT],
      notes: [],
    });
  });

  it("is case-insensitive for executor type", () => {
    expect(getCleanupSummary("WORKTREE").effects[0]).toMatch(/worktree/i);
    expect(getCleanupSummary("Local").notes.join(" ")).toMatch(/directly in your repo/i);
  });

  it("returns structured effects and notes instead of a flat line list", () => {
    const summary = getCleanupSummary("worktree");

    expect(summary).not.toHaveProperty("lines");
    expect(Array.isArray(summary.effects)).toBe(true);
    expect(Array.isArray(summary.notes)).toBe(true);
  });
});

describe("getBulkCleanupSummary", () => {
  it("groups by executor type and counts each group", () => {
    const { effects, notes } = getBulkCleanupSummary([
      "worktree",
      "worktree",
      "local_docker",
      "local",
      "sprites",
    ]);
    expect(effects.some((line) => /2 worktrees/.test(line) && /branches/.test(line))).toBe(true);
    expect(effects.some((line) => /1 Docker container/.test(line))).toBe(true);
    expect(notes.some((line) => /1 local task/.test(line) && /won't be touched/.test(line))).toBe(
      true,
    );
    expect(effects.some((line) => /1 Sprites sandbox/.test(line))).toBe(true);
  });

  it("uses singular wording for groups of size one", () => {
    const { effects } = getBulkCleanupSummary(["worktree", "local_docker"]);
    expect(effects.some((line) => /1 worktree\b/.test(line))).toBe(true);
    expect(effects.some((line) => /1 Docker container\b/.test(line))).toBe(true);
  });

  it("uses plural wording for groups of size > 1", () => {
    const { effects } = getBulkCleanupSummary(["worktree", "worktree", "worktree"]);
    expect(effects.some((line) => /3 worktrees/.test(line))).toBe(true);
  });

  it("groups remote_docker tasks with their own line", () => {
    const { effects } = getBulkCleanupSummary(["remote_docker", "remote_docker"]);
    expect(effects.some((line) => /2 remote Docker containers/.test(line))).toBe(true);
  });

  it("groups ssh tasks with best-effort wording", () => {
    const { effects, notes } = getBulkCleanupSummary(["ssh", "ssh"]);
    expect(effects.some((line) => /2 remote task directories/.test(line))).toBe(true);
    expect(notes.join(" ")).toMatch(/best-effort/i);
  });

  it("groups Kubernetes tasks with exact resource cleanup wording", () => {
    const { effects } = getBulkCleanupSummary(["k8s", "k8s"]);
    expect(effects.some((line) => /2 Kubernetes Pods/.test(line))).toBe(true);
  });

  it("keeps resource effects separate from local-reassurance notes", () => {
    const { effects, notes } = getBulkCleanupSummary(["local", "worktree"]);
    expect(effects.findIndex((line) => /worktree/i.test(line))).toBeGreaterThanOrEqual(0);
    expect(notes.findIndex((line) => /local task/i.test(line))).toBeGreaterThanOrEqual(0);
  });

  it("always appends the agent-session-stop effect", () => {
    const { effects } = getBulkCleanupSummary(["worktree"]);
    expect(effects[effects.length - 1]).toBe(AGENT_STOP_EFFECT);
  });

  it("ignores unknown / mock executor types in the grouped output", () => {
    const { effects, notes } = getBulkCleanupSummary([
      "worktree",
      undefined,
      "mock_remote",
      "what",
    ]);
    expect(effects.some((line) => /worktree/i.test(line))).toBe(true);
    expect(effects.some((line) => /mock/i.test(line))).toBe(false);
    expect(effects.some((line) => /what/i.test(line))).toBe(false);
    expect(notes).toEqual(getBulkCleanupSummary(["worktree"]).notes);
  });

  it("returns only the generic effect for an empty input", () => {
    expect(getBulkCleanupSummary([])).toEqual({
      effects: [AGENT_STOP_EFFECT],
      notes: [],
    });
  });
});

describe("localization", () => {
  it("resolves at call time, so a locale switch changes the copy", async () => {
    await i18n.changeLanguage("pseudo");
    const { effects, notes } = getCleanupSummary("worktree");
    // Every item, not just the first. The generic trailer must follow locale switches too.
    for (const line of [...effects, ...notes]) expect(line).toMatch(/[^\p{ASCII}]/u);
  });

  it("routes the bulk counts through catalog plurals rather than English suffixes", async () => {
    await i18n.changeLanguage("pseudo");
    const { effects } = getBulkCleanupSummary(["worktree", "worktree"]);
    // The count is interpolated, so it survives pseudo. The surrounding words must not.
    const worktreeLine = effects.find((line) => line.includes("2"));
    expect(worktreeLine).toBeDefined();
    expect(worktreeLine).not.toMatch(/worktrees/);
    expect(worktreeLine).toMatch(/[^\p{ASCII}]/u);
  });

  it("picks the singular catalog form for a group of one", () => {
    const { effects } = getBulkCleanupSummary(["sprites"]);
    expect(effects[0]).toBe("1 Sprites sandbox will be destroyed (uncommitted work lost).");
  });
});
