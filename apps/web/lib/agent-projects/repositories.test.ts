import { describe, expect, it } from "vitest";
import { isRemoteBackedProjectRepository } from "./repositories";

const repository = {
  source_type: "github",
  remote_url: "",
  provider_owner: "acme",
  provider_name: "api",
  default_branch: "main",
};

describe("isRemoteBackedProjectRepository", () => {
  it("accepts provider-backed repositories without a clone URL", () => {
    expect(isRemoteBackedProjectRepository(repository)).toBe(true);
  });

  it("rejects local repositories even when they have remote metadata", () => {
    expect(isRemoteBackedProjectRepository({ ...repository, source_type: "local" })).toBe(false);
  });

  it("rejects repositories without a default branch or provider identity", () => {
    expect(isRemoteBackedProjectRepository({ ...repository, default_branch: "" })).toBe(false);
    expect(
      isRemoteBackedProjectRepository({ ...repository, provider_owner: "", provider_name: "" }),
    ).toBe(false);
  });
});
