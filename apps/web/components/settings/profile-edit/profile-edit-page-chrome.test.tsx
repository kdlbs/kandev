import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { DeleteProfileDialog } from "./profile-edit-page-chrome";

afterEach(cleanup);

function renderDialog(relatedDockerContainerCount: number) {
  render(
    <DeleteProfileDialog
      open
      onOpenChange={() => {}}
      onDelete={vi.fn()}
      deleting={false}
      relatedDockerContainerCount={relatedDockerContainerCount}
    />,
  );
}

describe("DeleteProfileDialog related-container notice", () => {
  // The count-bearing message must come from a single `_one`/`_other` key.
  // Concatenating an English "s" renders correctly here and is untranslatable
  // everywhere else, so assert both forms read as whole sentences.
  it("uses the singular form for exactly one container", () => {
    renderDialog(1);

    expect(screen.getByText("1 related Docker container will also be removed.")).toBeTruthy();
  });

  it("uses the plural form for more than one container", () => {
    renderDialog(3);

    expect(screen.getByText("3 related Docker containers will also be removed.")).toBeTruthy();
  });

  it("omits the notice entirely when no containers are related", () => {
    renderDialog(0);

    expect(screen.queryByText(/will also be removed/)).toBeNull();
    expect(screen.queryByLabelText("Remove related Docker containers")).toBeNull();
  });
});
