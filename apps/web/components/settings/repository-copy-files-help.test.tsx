import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { CopyFilesField } from "./repository-copy-files-help";

afterEach(cleanup);

/**
 * Every sentence here is a `<Trans>` whose `<n>` indices address `<code>`
 * elements positionally, and each glob pattern travels as an interpolation VALUE
 * so the pseudo-locale cannot accent it into a pattern that matches nothing.
 * Both shapes fail silently: a drifted index renders duplicated fragments with
 * empty tags, and a pattern moved into the message body would only show up in a
 * translated build. These tests reconstruct the whole sentence and assert each
 * pattern lands inside a `<code>`.
 */
function renderField() {
  return render(<CopyFilesField repositoryId="repo-1" copyFiles="" onUpdate={vi.fn()} />).container;
}

function codeTexts(container: HTMLElement): string[] {
  return Array.from(container.querySelectorAll("code")).map((el) => el.textContent);
}

describe("CopyFilesField", () => {
  it("renders the symlink help as one sentence with each suffix in a code element", () => {
    const container = renderField();

    expect(document.getElementById("copy-files-help-repo-1")?.textContent).toBe(
      "Gitignored paths copied into new worktrees. Append :symlink to an entry to link it back to the main repo. Use ::symlink for a literal filename ending in :symlink.",
    );
    expect(codeTexts(container)).toEqual(expect.arrayContaining([":symlink", "::symlink"]));
  });

  it("keeps the glob patterns out of the translated message body", () => {
    const container = renderField();

    expect(codeTexts(container)).toEqual(
      expect.arrayContaining(["*", "?", "[abc]", "**", "**/.env", "{a,b}", ".env{,.local}"]),
    );
  });

  it("renders each supported-pattern bullet as a complete sentence", () => {
    const container = renderField();

    // Each bullet is assembled from `<code>` elements and text nodes, so it has
    // to be reconstructed — a drifted `<n>` index would drop a fragment here
    // while every individual `<code>` still rendered.
    const bullets = Array.from(container.querySelectorAll("li")).map((li) =>
      li.textContent?.replace(/\s+/g, " ").trim(),
    );
    expect(bullets).toEqual([
      ".env literal file or directory (directories copy recursively)",
      "*, ?, [abc] single-segment wildcards",
      "** matches any number of directories, e.g. **/.env",
      "{a,b} brace alternation, e.g. .env{,.local}",
    ]);
  });

  it("interpolates the remote size cap rather than baking it into the message", () => {
    renderField();

    expect(
      screen.getByText(
        "Files over 5 MiB are skipped when copying to remote executors. Local worktrees copy them without a size cap.",
      ),
    ).toBeTruthy();
  });
});
