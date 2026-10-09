import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { WorkspaceSwitcher } from "./workspace-switcher";

afterEach(cleanup);

it("leaves pointer isolation to the containing task picker during workspace selection", async () => {
  const onSelect = vi.fn();
  render(
    <WorkspaceSwitcher
      workspaces={[
        { id: "workspace-a", name: "Workspace A" },
        { id: "workspace-b", name: "Workspace B" },
      ]}
      activeWorkspaceId="workspace-a"
      onSelect={onSelect}
    />,
  );
  fireEvent.keyDown(screen.getByRole("button", { name: "Workspace A" }), { key: "ArrowDown" });
  const choice = await screen.findByRole("menuitem", { name: "Workspace B" });
  expect(document.body.style.pointerEvents).not.toBe("none");
  fireEvent.click(choice);
  expect(onSelect).toHaveBeenCalledExactlyOnceWith("workspace-b");
  expect(document.body.style.pointerEvents).not.toBe("none");
});
