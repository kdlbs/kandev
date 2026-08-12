import { afterEach, describe, expect, it } from "vitest";
import i18n from "i18next";
import type { AgentUpdateJob, AgentUpdatePreview } from "@/lib/api";
import { canApproveAgentRuntimeUpdate, resolveRuntimeVersionPair } from "./agent-runtime-update";

const preview = (overrides: Partial<AgentUpdatePreview> = {}): AgentUpdatePreview => ({
  agent_name: "claude-acp",
  package: "@agentclientprotocol/claude-agent-acp",
  current_version: "0.62.0",
  target_version: "0.63.0",
  command: ["npm", "exec"],
  command_string: "npm exec",
  ...overrides,
});

const job = (overrides: Partial<AgentUpdateJob> = {}): AgentUpdateJob => ({
  job_id: "runtime-update-job-1",
  agent_name: "claude-acp",
  status: "failed",
  started_at: "2026-07-26T12:00:00.000Z",
  ...overrides,
});

describe("resolveRuntimeVersionPair", () => {
  it("uses job versions when a retry has newer resolved values", () => {
    expect(
      resolveRuntimeVersionPair(
        preview(),
        job({ current_version: "0.63.0", target_version: "0.63.0" }),
      ),
    ).toEqual({
      currentVersion: "0.63.0",
      hasCurrentVersion: true,
      targetVersion: "0.63.0",
      versionsMatch: true,
    });
  });

  it("falls back to preview versions before a job exists", () => {
    expect(resolveRuntimeVersionPair(preview())).toEqual({
      currentVersion: "0.62.0",
      hasCurrentVersion: true,
      targetVersion: "0.63.0",
      versionsMatch: false,
    });
  });
});

describe("canApproveAgentRuntimeUpdate", () => {
  const ready = {
    preview: preview(),
    job: undefined,
    previewError: null,
    loading: false,
    updateInFlight: false,
    starting: false,
    installInFlight: false,
  };

  it.each([
    ["differing versions", ready, true],
    ["equal versions", { ...ready, preview: preview({ target_version: "0.62.0" }) }, false],
    ["missing current version", { ...ready, preview: preview({ current_version: "" }) }, false],
    ["missing target version", { ...ready, preview: preview({ target_version: "" }) }, false],
    ["loading preview", { ...ready, loading: true }, false],
    ["preview error", { ...ready, previewError: "preview failed" }, false],
    ["update in flight", { ...ready, updateInFlight: true }, false],
    ["starting update", { ...ready, starting: true }, false],
    ["install in flight", { ...ready, installInFlight: true }, false],
    [
      "stale retry with equal job versions",
      { ...ready, job: job({ current_version: "0.63.0", target_version: "0.63.0" }) },
      false,
    ],
  ])("returns %s = %s", (_name, state, expected) => {
    expect(canApproveAgentRuntimeUpdate(state)).toBe(expected);
  });
});

/**
 * The missing-version predicate used to be `currentVersion !== "Unknown"`, a
 * comparison against the English placeholder. Once `currentVersion` became
 * localized, that test was true in every other locale, so a missing version read
 * as known and the approval gate opened. These pin the state to a flag and the
 * string to display only.
 */
describe("missing current version under a non-English locale", () => {
  afterEach(async () => {
    await i18n.changeLanguage("en");
  });

  const missing = {
    preview: preview({ current_version: "" }),
    previewError: null,
    loading: false,
    updateInFlight: false,
    starting: false,
    installInFlight: false,
  };

  it.each(["en", "pt-pt", "zh-cn", "zh-tw", "zh-hk", "pseudo"])(
    "stays un-approvable in %s",
    async (locale) => {
      await i18n.changeLanguage(locale);

      const pair = resolveRuntimeVersionPair(preview({ current_version: "" }));
      expect(pair.hasCurrentVersion).toBe(false);
      expect(canApproveAgentRuntimeUpdate(missing)).toBe(false);
    },
  );

  it("still shows a localized placeholder to the user", async () => {
    await i18n.changeLanguage("pseudo");
    const pair = resolveRuntimeVersionPair(preview({ current_version: "" }));

    expect(pair.currentVersion).not.toBe("Unknown");
    expect(pair.currentVersion).toMatch(/[^\x20-\x7E]/);
  });

  it("does not treat a localized placeholder as matching a target version", async () => {
    await i18n.changeLanguage("pt-pt");
    const pair = resolveRuntimeVersionPair(
      preview({ current_version: "", target_version: "0.63.0" }),
    );

    expect(pair.versionsMatch).toBe(false);
  });
});
