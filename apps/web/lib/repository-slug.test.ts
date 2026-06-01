import { describe, it, expect } from "vitest";
import type { Repository } from "@/lib/types/http";
import { repositorySlug } from "./repository-slug";

function repo(overrides: Partial<Repository>): Repository {
  return {
    id: "r1",
    workspace_id: "w1",
    name: "",
    source_type: "local",
    local_path: "",
    provider: "",
    provider_repo_id: "",
    provider_owner: "",
    provider_name: "",
    default_branch: "main",
    ...overrides,
  } as Repository;
}

describe("repositorySlug", () => {
  it("uses owner/name for provider-backed repos", () => {
    expect(repositorySlug(repo({ provider_owner: "kdlbs", provider_name: "kandev" }))).toBe(
      "kdlbs/kandev",
    );
  });

  it("falls back to the repo name for local repos (matches board grouping identity)", () => {
    expect(
      repositorySlug(repo({ name: "kandev", local_path: "/home/carlos/Projects/kandev" })),
    ).toBe("kandev");
  });

  it("falls back to the last path segment when name is empty", () => {
    expect(repositorySlug(repo({ name: "", local_path: "/home/carlos/Projects/kandev" }))).toBe(
      "kandev",
    );
  });

  it("falls back to the full local_path when no path segment is available", () => {
    // "/" collapses to an empty array after split/filter, so the final
    // `|| repo.local_path` branch is the one actually exercised here.
    expect(repositorySlug(repo({ name: "", local_path: "/" }))).toBe("/");
  });
});
