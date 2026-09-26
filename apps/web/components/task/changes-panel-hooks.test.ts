import { afterEach, describe, expect, it } from "vitest";
import i18n from "i18next";
import type { PerRepoOperationResult } from "@/hooks/domains/session/use-session-git";

import { describePerRepo } from "./changes-panel-hooks";

/**
 * The multi-repo git toast. Its three branches were English template literals
 * (`${operationName} partially succeeded`, `Failed in N repos: ...`) and are now
 * catalog messages with five interpolation variables between them.
 *
 * A misspelled variable, such as `repo` for `repos` or `count` for `succeeded`, makes
 * i18next leave the placeholder in the rendered string rather than throw, so it
 * ships a toast reading "succeeded in {{count}} repos" and nothing else catches
 * it. Every assertion below therefore checks for a leftover `{{`.
 */
afterEach(async () => {
  await i18n.changeLanguage("en");
});

const repo = (
  name: string,
  success: boolean,
  error?: string,
  errorCode?: string,
): PerRepoOperationResult =>
  ({ repository_name: name, success, error, error_code: errorCode }) as PerRepoOperationResult;

const noPlaceholders = (result: { title: string; description: string }) => {
  expect(result.title).not.toContain("{{");
  expect(result.description).not.toContain("{{");
};

describe("describePerRepo", () => {
  it("reports every repo succeeding", () => {
    const result = describePerRepo([repo("api", true), repo("web", true)], "Push");

    expect(result.variant).toBe("success");
    expect(result.title).toBe("Push successful");
    expect(result.description).toBe("Push succeeded in 2 repos: api, web");
    noPlaceholders(result);
  });

  it("reports every repo failing, with each repo's error", () => {
    const result = describePerRepo(
      [repo("api", false, "no upstream"), repo("web", false, "conflict")],
      "Rebase",
    );

    expect(result.variant).toBe("error");
    expect(result.title).toBe("Rebase failed");
    expect(result.description).toBe("Failed in 2 repos: api: no upstream; web: conflict");
    noPlaceholders(result);
  });

  it("surfaces a partial success as an error but names what did succeed", () => {
    const result = describePerRepo([repo("api", true), repo("web", false, "conflict")], "Merge");

    expect(result.variant).toBe("error");
    expect(result.title).toBe("Merge partially succeeded");
    expect(result.description).toBe(
      "Merge succeeded in 1 of 2 repos (api); failed in web: conflict",
    );
    noPlaceholders(result);
  });

  it("falls back to a generic error when a repo reports no message", () => {
    const result = describePerRepo([repo("api", false)], "Pull");

    expect(result.description).toBe("Failed in 1 repo: api: Unknown error");
    noPlaceholders(result);
  });

  it("uses the singular form for a single failing repo", () => {
    // `_one` vs `_other` is the reason `count` is passed at all; English differs
    // only in "repo"/"repos", but a locale with more forms needs the number.
    const one = describePerRepo([repo("api", false, "boom")], "Push");
    const two = describePerRepo([repo("api", false, "boom"), repo("web", false, "boom")], "Push");

    expect(one.description).toContain("1 repo:");
    expect(two.description).toContain("2 repos:");
  });

  it("uses localized recovery copy for a bounded publication failure", () => {
    const result = describePerRepo(
      [repo("api", false, "raw remote output", "empty_remote_remote_changed")],
      "Push",
    );

    expect(result.description).toBe(
      "Failed in 1 repo: api: The remote changed before the base branch was published. Refresh the task and try again.",
    );
    expect(result.description).not.toContain("raw remote output");
  });

  it("keeps every branch resolving after a locale switch", async () => {
    await i18n.changeLanguage("pseudo");

    for (const result of [
      describePerRepo([repo("api", true)], "Push"),
      describePerRepo([repo("api", false, "boom")], "Push"),
      describePerRepo([repo("api", true), repo("web", false, "boom")], "Push"),
    ]) {
      noPlaceholders(result);
      // The repo names are data and stay ASCII; the frame around them must not.
      expect(result.title).toMatch(/[^\x20-\x7E]/);
    }
  });
});
