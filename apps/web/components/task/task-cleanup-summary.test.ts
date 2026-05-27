import { describe, it, expect } from "vitest";
import { getCleanupSummary, getBulkCleanupSummary } from "./task-cleanup-summary";

const AGENT_STOP_LINE = "Any running agent sessions will be stopped.";

describe("getCleanupSummary (single task)", () => {
  it("reassures the user that local executor leaves the repo untouched", () => {
    const { lines } = getCleanupSummary("local");
    expect(lines[0]).toMatch(/directly in your repo/i);
    expect(lines[0]).toMatch(/not touched/i);
  });

  it("describes worktree deletion + branch removal", () => {
    const { lines } = getCleanupSummary("worktree");
    expect(lines[0]).toMatch(/worktree/i);
    expect(lines[0]).toMatch(/branch/i);
    expect(lines[0]).toMatch(/not affected/i);
  });

  it("describes local_docker container removal", () => {
    const { lines } = getCleanupSummary("local_docker");
    expect(lines[0]).toMatch(/Docker container/i);
    expect(lines[0]).toMatch(/removed/i);
  });

  it("describes remote_docker container removal", () => {
    const { lines } = getCleanupSummary("remote_docker");
    expect(lines[0]).toMatch(/remote Docker container/i);
  });

  it("warns that sprites sandbox destruction loses uncommitted work", () => {
    const { lines } = getCleanupSummary("sprites");
    expect(lines[0]).toMatch(/Sprites/i);
    expect(lines[0]).toMatch(/uncommitted work/i);
  });

  it("describes ssh remote directory removal as best-effort", () => {
    const { lines } = getCleanupSummary("ssh");
    expect(lines[0]).toMatch(/remote/i);
    expect(lines[0]).toMatch(/best-effort/i);
  });

  it("always appends the agent-session-stop line", () => {
    expect(getCleanupSummary("local").lines).toContain(AGENT_STOP_LINE);
    expect(getCleanupSummary("worktree").lines).toContain(AGENT_STOP_LINE);
  });

  it("falls back to a generic line when executor type is unknown or missing", () => {
    expect(getCleanupSummary(undefined).lines).toEqual([AGENT_STOP_LINE]);
    expect(getCleanupSummary(null).lines).toEqual([AGENT_STOP_LINE]);
    expect(getCleanupSummary("totally-made-up").lines).toEqual([AGENT_STOP_LINE]);
  });

  it("is case-insensitive for executor type", () => {
    expect(getCleanupSummary("WORKTREE").lines[0]).toMatch(/worktree/i);
    expect(getCleanupSummary("Local").lines[0]).toMatch(/directly in your repo/i);
  });
});

describe("getBulkCleanupSummary", () => {
  it("groups by executor type and counts each group", () => {
    const { lines } = getBulkCleanupSummary([
      "worktree",
      "worktree",
      "local_docker",
      "local",
      "sprites",
    ]);
    expect(lines.some((l) => /2 worktrees/.test(l) && /branches/.test(l))).toBe(true);
    expect(lines.some((l) => /1 Docker container/.test(l))).toBe(true);
    expect(lines.some((l) => /1 local task/.test(l) && /won't be touched/.test(l))).toBe(true);
    expect(lines.some((l) => /1 Sprites sandbox/.test(l))).toBe(true);
  });

  it("uses singular wording for groups of size one", () => {
    const { lines } = getBulkCleanupSummary(["worktree", "local_docker"]);
    expect(lines.some((l) => /1 worktree\b/.test(l))).toBe(true);
    expect(lines.some((l) => /1 Docker container\b/.test(l))).toBe(true);
  });

  it("uses plural wording for groups of size > 1", () => {
    const { lines } = getBulkCleanupSummary(["worktree", "worktree", "worktree"]);
    expect(lines.some((l) => /3 worktrees/.test(l))).toBe(true);
  });

  it("groups remote_docker tasks with their own line", () => {
    const { lines } = getBulkCleanupSummary(["remote_docker", "remote_docker"]);
    expect(lines.some((l) => /2 remote Docker containers/.test(l))).toBe(true);
  });

  it("groups ssh tasks with best-effort wording", () => {
    const { lines } = getBulkCleanupSummary(["ssh", "ssh"]);
    expect(lines.some((l) => /2 remote task directories/.test(l) && /best-effort/.test(l))).toBe(
      true,
    );
  });

  it("emits worktree/container/sandbox lines before the local-reassurance line", () => {
    const { lines } = getBulkCleanupSummary(["local", "worktree"]);
    const worktreeIdx = lines.findIndex((l) => /worktree/i.test(l));
    const localIdx = lines.findIndex((l) => /local task/i.test(l));
    expect(worktreeIdx).toBeGreaterThanOrEqual(0);
    expect(localIdx).toBeGreaterThan(worktreeIdx);
  });

  it("always appends the agent-session-stop line", () => {
    const { lines } = getBulkCleanupSummary(["worktree"]);
    expect(lines[lines.length - 1]).toBe(AGENT_STOP_LINE);
  });

  it("ignores unknown / mock executor types in the grouped output", () => {
    const { lines } = getBulkCleanupSummary(["worktree", undefined, "mock_remote", "what"]);
    expect(lines.some((l) => /worktree/i.test(l))).toBe(true);
    expect(lines.some((l) => /mock/i.test(l))).toBe(false);
    expect(lines.some((l) => /what/i.test(l))).toBe(false);
  });

  it("returns only the generic line for an empty input", () => {
    expect(getBulkCleanupSummary([]).lines).toEqual([AGENT_STOP_LINE]);
  });
});
