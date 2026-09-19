import { describe, expect, it } from "vitest";
import { buildRepositoriesPayload } from "./task-create-dialog-helpers";
import { applyRemoteRepoPatch } from "./task-create-dialog-repositories-state";
import { parseCheckoutDirectories } from "./task-create-dialog-checkout-options";
import type { TaskRemoteRepoRow } from "./task-create-dialog-types";

const options = {
  version: 1 as const,
  download_mode: "on_demand" as const,
  sparse_directories: ["extensions/my extension"],
};
const row = {
  key: "one",
  url: "https://github.com/acme/repo",
  branch: "main",
  source: "paste",
  checkoutOptions: options,
} as TaskRemoteRepoRow;
describe("task-only repository checkout options", () => {
  it("keeps options in the task payload for only the selected row", () => {
    const payload = buildRepositoriesPayload({
      useRemote: true,
      remoteRepos: [
        row,
        { ...row, key: "two", url: "https://github.com/acme/other", checkoutOptions: undefined },
      ],
      repositories: [],
      discoveredRepositories: [],
    });
    expect(payload?.[0]).toHaveProperty("checkout_options", options);
    expect(payload?.[1]).not.toHaveProperty("checkout_options");
  });
  it("preserves branch edits and clears repository replacement", () => {
    expect(applyRemoteRepoPatch(row, { branch: "feature" }).checkoutOptions).toEqual(options);
    expect(
      applyRemoteRepoPatch(row, { url: "https://github.com/acme/new" }).checkoutOptions,
    ).toBeUndefined();
  });
  it("deduplicates directories and rejects invalid paths", () => {
    expect(parseCheckoutDirectories("app\n../invalid\n/absolute")).toHaveProperty(
      "errorLines",
      [2, 3],
    );
    expect(parseCheckoutDirectories(" app\napp\nshared files ")).toEqual({
      directories: ["app", "shared files"],
    });
    for (const value of ["../secret", "/app", "app//child", "app/*", "app\\child"])
      expect(parseCheckoutDirectories(value)).toHaveProperty("error");
  });
});
