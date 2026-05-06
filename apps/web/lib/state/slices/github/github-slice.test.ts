import { describe, it, expect } from "vitest";
import { create } from "zustand";
import { immer } from "zustand/middleware/immer";
import { createGitHubSlice } from "./github-slice";
import type { GitHubSlice } from "./types";
import type { GitHubStatus } from "@/lib/types/github";

function makeStore() {
  return create<GitHubSlice>()(immer((...a) => createGitHubSlice(...a)));
}

const FUTURE_RESET = "2030-01-01T00:00:00Z";
const NOW = "2026-05-04T12:00:00Z";

const baseStatus: GitHubStatus = {
  authenticated: true,
  username: "octocat",
  auth_method: "pat",
  token_configured: true,
  required_scopes: ["repo"],
};

describe("applyGitHubRateLimitUpdate", () => {
  it("merges incoming snapshots into the existing status", () => {
    const store = makeStore();
    store.getState().setGitHubStatus({ ...baseStatus });

    store.getState().applyGitHubRateLimitUpdate({
      trigger: "graphql",
      snapshots: [
        {
          resource: "graphql",
          remaining: 0,
          limit: 5000,
          reset_at: FUTURE_RESET,
          updated_at: NOW,
        },
        {
          resource: "core",
          remaining: 4500,
          limit: 5000,
          reset_at: FUTURE_RESET,
          updated_at: NOW,
        },
      ],
    });

    const status = store.getState().githubStatus.status;
    expect(status?.rate_limit?.graphql?.remaining).toBe(0);
    expect(status?.rate_limit?.graphql?.limit).toBe(5000);
    expect(status?.rate_limit?.core?.remaining).toBe(4500);
  });

  it("overwrites only the resources present in the update", () => {
    const store = makeStore();
    store.getState().setGitHubStatus({
      ...baseStatus,
      rate_limit: {
        core: {
          resource: "core",
          remaining: 4500,
          limit: 5000,
          reset_at: FUTURE_RESET,
          updated_at: NOW,
        },
      },
    });

    store.getState().applyGitHubRateLimitUpdate({
      trigger: "graphql",
      snapshots: [
        {
          resource: "graphql",
          remaining: 100,
          limit: 5000,
          reset_at: FUTURE_RESET,
          updated_at: NOW,
        },
      ],
    });

    const rl = store.getState().githubStatus.status?.rate_limit;
    expect(rl?.core?.remaining).toBe(4500); // untouched
    expect(rl?.graphql?.remaining).toBe(100);
  });

  it("is a no-op when status has not been hydrated yet", () => {
    const store = makeStore();
    expect(store.getState().githubStatus.status).toBeNull();

    store.getState().applyGitHubRateLimitUpdate({
      trigger: "core",
      snapshots: [
        {
          resource: "core",
          remaining: 0,
          limit: 5000,
          reset_at: FUTURE_RESET,
          updated_at: NOW,
        },
      ],
    });

    expect(store.getState().githubStatus.status).toBeNull();
  });
});
