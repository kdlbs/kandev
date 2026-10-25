import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { DiscardLocalChangesDialog } from "./discard-local-changes-dialog";

afterEach(cleanup);

describe("DiscardLocalChangesDialog", () => {
  it("keeps fresh-branch dirty-file consent in a scope-explaining modal", () => {
    render(
      <DiscardLocalChangesDialog
        open
        onOpenChange={vi.fn()}
        dirtyFiles={["src/one.ts"]}
        repoPath="/tmp/repo"
        onConfirm={vi.fn()}
        onCancel={vi.fn()}
      />,
    );

    expect(screen.getByRole("alertdialog")).toBeTruthy();
    expect(screen.getByTestId("discard-local-changes-files").textContent).toContain("src/one.ts");
    expect(screen.getByTestId("discard-local-changes-confirm")).toBeTruthy();
  });
});
