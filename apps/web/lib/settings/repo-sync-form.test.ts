import { describe, expect, it } from "vitest";
import {
  formForProvider,
  formFromUrl,
  intervalInvalid,
  repoSyncPayload,
  repoTargetMissing,
  type RepoSyncFormState,
} from "./repo-sync-form";

const base: RepoSyncFormState = {
  github_host: "github.com",
  provider: "github",
  repo_owner: "",
  repo_name: "",
  project_path: "",
  branch: "main",
  path: ".kandev/prompts",
  interval_seconds: 300,
  poll_enabled: true,
};

describe("repo-sync-form", () => {
  it("fills owner and repo from a GitHub link and keeps extra fields", () => {
    const form = formFromUrl({ ...base, workspace_id: "ws-1" }, "https://github.com/acme/eng");
    expect(form.repo_owner).toBe("acme");
    expect(form.repo_name).toBe("eng");
    expect(form.workspace_id).toBe("ws-1");
    expect(repoTargetMissing(form)).toBe(false);
  });

  it("builds provider-specific payloads", () => {
    const gitlab = { ...formForProvider(base, "gitlab"), project_path: " acme/eng " };
    expect(repoSyncPayload(gitlab)).toEqual({
      provider: "gitlab",
      project_path: "acme/eng",
      branch: "main",
      path: ".kandev/prompts",
      interval_seconds: 300,
      poll_enabled: true,
    });
    expect(gitlab.github_host).toBe("");
  });

  it("validates the polling interval only while polling", () => {
    expect(intervalInvalid({ ...base, interval_seconds: 30 })).toBe(true);
    expect(intervalInvalid({ ...base, interval_seconds: 30, poll_enabled: false })).toBe(false);
  });
});
