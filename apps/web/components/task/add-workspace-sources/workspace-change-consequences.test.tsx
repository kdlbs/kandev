import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { WorkspaceChangeConsequences } from "./workspace-change-consequences";
afterEach(cleanup);
it("shows restart consequences only for expansion", () => {
  const { rerender } = render(<WorkspaceChangeConsequences restartsWorkspace={false} />);
  expect(screen.queryByTestId("workspace-change-consequences")).toBeNull();
  rerender(<WorkspaceChangeConsequences restartsWorkspace />);
  expect(screen.getByTestId("workspace-change-consequences").textContent).toContain(
    "This restarts the task workspace",
  );
  rerender(<WorkspaceChangeConsequences restartsWorkspace={false} />);
  expect(screen.queryByTestId("workspace-change-consequences")).toBeNull();
});
