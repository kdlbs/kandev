import { afterEach, describe, expect, it, vi } from "vitest";
import {
  extractRepoName,
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

  it("prefers local master over origin/main", () => {
    const branches = [
      { name: "master", type: "local" },
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

  it("falls back to Math.random UUID when crypto.randomUUID is undefined (HTTP/non-secure)", () => {
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
