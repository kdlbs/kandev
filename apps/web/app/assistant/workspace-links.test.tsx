import { afterEach, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { WorkspaceGrantForm } from "./workspace-grant-form";
const receiver = {
  profile_id: "central",
  profile_name: "Example central profile",
  profile_revision: "account-v1",
  authority_revision: "authority-v1",
};
afterEach(cleanup);
it("requires explicit confirmation of the receiving profile and sends the displayed revisions", () => {
  const save = vi.fn();
  render(
    <WorkspaceGrantForm
      bindingVersion={3}
      receiver={receiver}
      choices={[{ id: "linked", name: "Example workspace" }]}
      busy={false}
      onSave={save}
    />,
  );
  expect(screen.getByText(/Example central profile/)).toBeTruthy();
  fireEvent.change(screen.getByLabelText("Workspace"), { target: { value: "linked" } });
  const submit = screen.getByRole<HTMLButtonElement>("button", {
    name: "Confirm workspace access",
  });
  expect(submit.disabled).toBe(true);
  fireEvent.click(
    screen.getByLabelText("I confirm this workspace context may be sent to the displayed profile."),
  );
  fireEvent.click(submit);
  expect(save).toHaveBeenCalledWith("linked", {
    expected_binding_version: 3,
    expected_revision: 0,
    receiver,
    scope: { operations: ["observe"], context_exports: ["task_summary"] },
  });
});
it("does not carry confirmation across a receiving account change", () => {
  const props = {
    bindingVersion: 3,
    receiver,
    choices: [{ id: "linked", name: "Example workspace" }],
    busy: false,
    onSave: vi.fn(),
  };
  const { rerender } = render(<WorkspaceGrantForm {...props} />);
  fireEvent.change(screen.getByLabelText("Workspace"), { target: { value: "linked" } });
  fireEvent.click(
    screen.getByLabelText("I confirm this workspace context may be sent to the displayed profile."),
  );
  rerender(
    <WorkspaceGrantForm {...props} receiver={{ ...receiver, profile_revision: "account-v2" }} />,
  );
  expect(
    screen.getByRole<HTMLButtonElement>("button", { name: "Confirm workspace access" }).disabled,
  ).toBe(true);
});
