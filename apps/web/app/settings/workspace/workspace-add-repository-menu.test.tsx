import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { AddRepositoryMenu } from "./workspace-add-repository-menu";

afterEach(() => {
  cleanup();
});

function openMenu() {
  const trigger = screen.getByRole("button", { name: "Add repository" });
  fireEvent.pointerDown(trigger, { button: 0, ctrlKey: false });
  fireEvent.click(trigger);
}

describe("AddRepositoryMenu", () => {
  it("offers a local and a remote repository entry", async () => {
    const onAdd = vi.fn();
    render(<AddRepositoryMenu onAdd={onAdd} />);

    openMenu();
    fireEvent.click(await screen.findByRole("menuitem", { name: "Remote repository" }));

    expect(onAdd).toHaveBeenCalledWith("remote");
  });

  it("routes the local entry to the local flow", async () => {
    const onAdd = vi.fn();
    render(<AddRepositoryMenu onAdd={onAdd} />);

    openMenu();
    fireEvent.click(await screen.findByRole("menuitem", { name: "Local repository" }));

    expect(onAdd).toHaveBeenCalledWith("local");
  });
});
