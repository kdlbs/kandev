import { describe, expect, it } from "vitest";
import {
  resolveTaskRepositorySlugs,
  type SidebarTaskRepositoryLink,
} from "./sidebar-task-repositories";

const REPO_A = "repo-a";
const REPO_B = "repo-b";
const REPO_A_SLUG = "owner/repo-a";
const REPO_B_SLUG = "owner/repo-b";

function link(repository_id: string, position?: number): SidebarTaskRepositoryLink {
  return { repository_id, position };
}

describe("resolveTaskRepositorySlugs", () => {
  it("returns empty for null, undefined, and empty input", () => {
    const repositorySlugs = new Map([[REPO_A, REPO_A_SLUG]]);

    expect(resolveTaskRepositorySlugs(null, repositorySlugs)).toEqual([]);
    expect(resolveTaskRepositorySlugs(undefined, repositorySlugs)).toEqual([]);
    expect(resolveTaskRepositorySlugs([], repositorySlugs)).toEqual([]);
  });

  it("sorts by attachment position and removes duplicate repositories", () => {
    const repositoryLinks = [link(REPO_B, 2), link(REPO_A, 1), link(REPO_A, 3), link("repo-c")];
    const repositorySlugs = new Map([
      [REPO_A, REPO_A_SLUG],
      [REPO_B, REPO_B_SLUG],
      ["repo-c", "owner/repo-c"],
    ]);

    expect(resolveTaskRepositorySlugs(repositoryLinks, repositorySlugs)).toEqual([
      REPO_A_SLUG,
      REPO_B_SLUG,
      "owner/repo-c",
    ]);
  });

  it("uses link order when positions are absent and omits unresolved repositories", () => {
    const repositoryLinks = [link("repo-missing"), link(REPO_A), link(REPO_B)];
    const repositorySlugs = new Map([
      [REPO_A, REPO_A_SLUG],
      [REPO_B, REPO_B_SLUG],
    ]);

    expect(resolveTaskRepositorySlugs(repositoryLinks, repositorySlugs)).toEqual([
      REPO_A_SLUG,
      REPO_B_SLUG,
    ]);
  });
});
