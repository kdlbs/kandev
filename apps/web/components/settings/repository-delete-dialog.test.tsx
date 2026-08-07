import { cleanup, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { DeleteRepositoryDialog } from "./repository-delete-dialog";

afterEach(cleanup);

// `@kandev/ui`'s DialogContent renders its own sr-only "Close" control, so the
// footer has to be scoped explicitly or the dismiss button is ambiguous.
function footer(): HTMLElement {
  return document.querySelector('[data-slot="dialog-footer"]') as HTMLElement;
}

/**
 * The active-session sentence used to inflect both its noun and its pronoun with
 * a ternary (`session`/`sessions`, `it`/`them`), so a bad `_one` / `_other` split
 * would have rendered "1 active agent sessions … finish them" without failing
 * anything. These assertions pin both forms and the no-sessions variant.
 */
function renderDialog(activeSessionCount: number) {
  return render(
    <DeleteRepositoryDialog
      open
      onOpenChange={vi.fn()}
      onDelete={vi.fn()}
      activeSessionCount={activeSessionCount}
      deleteLoading={false}
    />,
  );
}

describe("DeleteRepositoryDialog", () => {
  it("uses the singular noun and pronoun for exactly one active session", () => {
    renderDialog(1);

    expect(
      screen.getByText(
        "This repository is used by 1 active agent session. Stop or finish it before deleting the repository.",
      ),
    ).toBeTruthy();
  });

  it("uses the plural noun and pronoun for more than one active session", () => {
    renderDialog(3);

    expect(
      screen.getByText(
        "This repository is used by 3 active agent sessions. Stop or finish them before deleting the repository.",
      ),
    ).toBeTruthy();
  });

  it("offers only a Close button while sessions are active", () => {
    renderDialog(2);

    expect(within(footer()).getByRole("button", { name: "Close" })).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Delete Repository" })).toBeNull();
  });

  it("offers Cancel and Delete Repository when no session is active", () => {
    renderDialog(0);

    expect(
      screen.getByText(
        "This will remove the repository and its scripts. This action cannot be undone.",
      ),
    ).toBeTruthy();
    expect(screen.getByRole("button", { name: "Cancel" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "Delete Repository" })).toBeTruthy();
  });
});
