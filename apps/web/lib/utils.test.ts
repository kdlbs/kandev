import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  extractRepoName,
  formatRelativeTime,
  formatUserHomePath,
  generateUUID,
  getRepositoryDisplayName,
  selectPreferredBranch,
  truncateRepoPath,
} from "./utils";

const UUID_V4_REGEX = /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/;

const TILDE_PROJECTS_APP = "~/Projects/App";

describe("formatUserHomePath", () => {
  it("replaces mac home path with tilde", () => {
    expect(formatUserHomePath("/Users/alex/Projects/App")).toBe(TILDE_PROJECTS_APP);
  });

  it("replaces linux home path with tilde", () => {
    expect(formatUserHomePath("/home/alex/projects/app")).toBe("~/projects/app");
  });

  it("replaces windows home path with tilde", () => {
    expect(formatUserHomePath("C:\\Users\\alex\\Projects\\App")).toBe(TILDE_PROJECTS_APP);
  });

  it("leaves non-home paths unchanged", () => {
    expect(formatUserHomePath("/var/tmp/project")).toBe("/var/tmp/project");
  });
});

describe("truncateRepoPath", () => {
  it("returns the path when under the limit", () => {
    expect(truncateRepoPath(TILDE_PROJECTS_APP, 40)).toBe(TILDE_PROJECTS_APP);
  });

  it("prefers last segments for long paths", () => {
    const path = "/Users/alex/Projects/Group/RepoName";
    expect(truncateRepoPath(path, 22)).toBe("~/.../Group/RepoName");
  });

  it("falls back to last segment when space is tight", () => {
    const path = "/Users/alex/Projects/Group/RepoName";
    expect(truncateRepoPath(path, 10)).toBe("~/.../Name");
  });
});

describe("selectPreferredBranch", () => {
  it("selects local main first", () => {
    const branches = [
      { name: "main", type: "local" },
      { name: "main", type: "remote", remote: "origin" },
    ];
    expect(selectPreferredBranch(branches)).toBe("main");
  });

  it("keeps local master ahead of origin/main", () => {
    const branches = [
      { name: "master", type: "local" },
      { name: "main", type: "remote", remote: "origin" },
    ];
    expect(selectPreferredBranch(branches)).toBe("master");
  });

  it("keeps local master ahead of remote conventional branches", () => {
    const branches = [
      { name: "master", type: "local" },
      { name: "master", type: "remote", remote: "origin" },
      { name: "main", type: "remote", remote: "origin" },
    ];
    expect(selectPreferredBranch(branches)).toBe("master");
  });

  it("falls back to origin/main when no local main/master", () => {
    const branches = [
      { name: "main", type: "remote", remote: "origin" },
      { name: "develop", type: "local" },
    ];
    expect(selectPreferredBranch(branches)).toBe("origin/main");
  });

  it("falls back to origin/master", () => {
    const branches = [{ name: "master", type: "remote", remote: "origin" }];
    expect(selectPreferredBranch(branches)).toBe("origin/master");
  });

  it("returns null when no preferred branches exist", () => {
    const branches = [{ name: "develop", type: "local" }];
    expect(selectPreferredBranch(branches)).toBeNull();
  });
});

describe("extractRepoName", () => {
  it("extracts org/name from ssh urls", () => {
    expect(extractRepoName("git@gitlab.com:org/repo.git")).toBe("org/repo");
  });

  it("extracts org/name from https urls", () => {
    expect(extractRepoName("https://bitbucket.org/org/repo")).toBe("org/repo");
  });

  it("returns null for local paths", () => {
    expect(extractRepoName("/Users/alex/Projects/App")).toBeNull();
  });
});

describe("getRepositoryDisplayName", () => {
  it("returns a tilde path for local repositories", () => {
    expect(getRepositoryDisplayName("/Users/alex/Projects/App")).toBe(TILDE_PROJECTS_APP);
  });

  it("returns org/name for remote repositories", () => {
    expect(getRepositoryDisplayName("https://github.com/org/repo.git")).toBe("org/repo");
  });
});

describe("generateUUID", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("uses crypto.randomUUID when available (secure context)", () => {
    const stub = vi.fn(() => "11111111-1111-4111-8111-111111111111");
    vi.stubGlobal("crypto", { randomUUID: stub });
    expect(generateUUID()).toBe("11111111-1111-4111-8111-111111111111");
    expect(stub).toHaveBeenCalledOnce();
  });

  it("uses crypto.getRandomValues when randomUUID is unavailable", () => {
    const getRandomValues = vi.fn((bytes: Uint8Array) => {
      bytes.fill(0x11);
      return bytes;
    });
    vi.stubGlobal("crypto", { getRandomValues });
    expect(generateUUID()).toBe("11111111-1111-4111-9111-111111111111");
    expect(getRandomValues).toHaveBeenCalledOnce();
  });

  it("falls back to a UUID-shaped value when Web Crypto is unavailable", () => {
    vi.stubGlobal("crypto", {});
    const id = generateUUID();
    expect(id).toMatch(UUID_V4_REGEX);
  });

  it("falls back when crypto itself is undefined", () => {
    vi.stubGlobal("crypto", undefined);
    const id = generateUUID();
    expect(id).toMatch(UUID_V4_REGEX);
  });

  it("produces distinct ids across calls in the fallback path", () => {
    vi.stubGlobal("crypto", {});
    const a = generateUUID();
    const b = generateUUID();
    expect(a).not.toBe(b);
  });
});

describe("formatRelativeTime", () => {
  const NOW = new Date("2026-03-15T12:00:00.000Z");
  const SECOND = 1000;
  const MINUTE = 60 * SECOND;
  const HOUR = 60 * MINUTE;
  const DAY = 24 * HOUR;

  /** ISO string for a timestamp `ms` milliseconds before the frozen NOW. */
  const ago = (ms: number) => new Date(NOW.getTime() - ms).toISOString();

  beforeEach(() => {
    vi.useFakeTimers();
    vi.setSystemTime(NOW);
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it("says 'just now' below the 10s boundary", () => {
    expect(formatRelativeTime(ago(0))).toBe("just now");
    expect(formatRelativeTime(ago(9 * SECOND))).toBe("just now");
  });

  it("switches to seconds at 10s and holds until the 60s boundary", () => {
    expect(formatRelativeTime(ago(10 * SECOND))).toBe("10s ago");
    expect(formatRelativeTime(ago(59 * SECOND))).toBe("59s ago");
  });

  it("switches to minutes at 60s and holds until the 60m boundary", () => {
    expect(formatRelativeTime(ago(MINUTE))).toBe("1m ago");
    expect(formatRelativeTime(ago(59 * MINUTE))).toBe("59m ago");
  });

  it("switches to hours at 60m and holds until the 24h boundary", () => {
    expect(formatRelativeTime(ago(HOUR))).toBe("1h ago");
    expect(formatRelativeTime(ago(23 * HOUR))).toBe("23h ago");
  });

  it("says 'yesterday' for the whole first day past the 24h boundary", () => {
    expect(formatRelativeTime(ago(DAY))).toBe("yesterday");
    expect(formatRelativeTime(ago(2 * DAY - SECOND))).toBe("yesterday");
  });

  it("switches to days at 48h and holds until the 7d boundary", () => {
    expect(formatRelativeTime(ago(2 * DAY))).toBe("2d ago");
    expect(formatRelativeTime(ago(6 * DAY))).toBe("6d ago");
  });

  it("falls back to an absolute date at 7d", () => {
    const sevenDaysAgo = new Date(NOW.getTime() - 7 * DAY);
    expect(formatRelativeTime(sevenDaysAgo.toISOString())).toBe(sevenDaysAgo.toLocaleDateString());
  });
});
