import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { DeleteProfileDialog, ProfileFormActions } from "./profile-edit-page-chrome";
import { ProfileConnectionSettingsAction } from "./profile-connection-settings-action";

const { push } = vi.hoisted(() => ({ push: vi.fn() }));

vi.mock("@/lib/routing/client-router", () => ({ useRouter: () => ({ push }) }));

afterEach(() => {
  cleanup();
  push.mockReset();
});

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

describe("ProfileFormActions permissions", () => {
  it("keeps the Kubernetes delete control visible but disabled for members", () => {
    render(<ProfileFormActions onDelete={vi.fn()} disabled />);

    expect(screen.getByRole("button", { name: "Delete Profile" }).hasAttribute("disabled")).toBe(
      true,
    );
  });
});

describe("Profile connection settings actions", () => {
  it("leaves Kubernetes connection access to the leading cluster section", () => {
    const { container } = render(
      <ProfileConnectionSettingsAction executor={{ id: "executor/one", type: "k8s" }} />,
    );

    expect(container.childElementCount).toBe(0);
  });

  it("keeps the SSH header action touch-visible and routes by executor identity", () => {
    render(<ProfileConnectionSettingsAction executor={{ id: "executor/one", type: "ssh" }} />);

    const action = screen.getByRole("button", { name: /connection settings/i });
    expect(action.className).toContain("min-h-11");
    fireEvent.click(action);
    expect(push).toHaveBeenCalledWith("/settings/executors/ssh/executor%2Fone");
  });
});
