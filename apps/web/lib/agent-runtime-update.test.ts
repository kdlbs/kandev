import { afterEach, describe, expect, it } from "vitest";
import i18n from "i18next";
import type { AgentUpdateJob, AgentUpdatePreview } from "@/lib/api";
import {
  canApproveAgentRuntimeUpdate,
  latestRuntimeVersions,
  resolveRuntimeActiveVersion,
  resolveRuntimeEffectiveVersion,
  resolveRuntimeOperation,
  resolveRuntimeVersionPair,
  runtimeOperationLabelKey,
} from "./agent-runtime-update";

describe("latestRuntimeVersions", () => {
  it("preserves the complete backend projection", () => {
    const versions = Array.from({ length: 12 }, (_, index) => ({
      version: `1.0.${12 - index}`,
      latest: index === 0,
    }));

    expect(latestRuntimeVersions(versions)).toEqual(versions);
  });
});

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

describe("resolveRuntimeActiveVersion", () => {
  it("uses the successful job activation snapshot", () => {
    expect(
      resolveRuntimeActiveVersion(
        preview({ active_version: "0.62.0" }),
        job({ status: "succeeded", active_version: "0.63.0" }),
      ),
    ).toBe("0.63.0");
  });

  it("keeps the preview selection when activation fails", () => {
    expect(
      resolveRuntimeActiveVersion(
        preview({ active_version: "0.62.0" }),
        job({ status: "failed", active_version: "0.62.0" }),
      ),
    ).toBe("0.62.0");
  });

  it("clears the preview selection after a successful default reset", () => {
    expect(
      resolveRuntimeActiveVersion(
        preview({ active_version: "0.62.0" }),
        job({
          status: "succeeded",
          operation: "use_default",
          active_version: undefined,
        }),
      ),
    ).toBeUndefined();
  });
});

describe("resolveRuntimeEffectiveVersion", () => {
  it("prefers the terminal job effective version", () => {
    expect(
      resolveRuntimeEffectiveVersion(
        preview({ effective_version: "0.62.0" }),
        job({ status: "succeeded", effective_version: "0.64.0" }),
      ),
    ).toBe("0.64.0");
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
    ["up-to-date operation", { ...ready, preview: preview({ operation: "up_to_date" }) }, false],
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

  it("allows a repair when the backend says the observed version is unknown", () => {
    expect(
      canApproveAgentRuntimeUpdate({
        ...ready,
        preview: preview({ current_version: "", operation: "repair" }),
      }),
    ).toBe(true);
  });

  it("uses operation state instead of translated labels", () => {
    const state = preview({ operation: "rollback" });
    expect(resolveRuntimeOperation(state)).toBe("rollback");
    expect(
      resolveRuntimeOperation(preview({ operation: "update" }), job({ operation: "repair" })),
    ).toBe("repair");
    expect(runtimeOperationLabelKey("rollback")).toBe("agents:rollBackRuntime");
    expect(runtimeOperationLabelKey("repair")).toBe("agents:repairRuntime");
    expect(runtimeOperationLabelKey("up_to_date")).toBe("agents:upToDateRuntime");
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
