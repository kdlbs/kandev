import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { SettingsSaveProvider } from "@/components/settings/settings-save-provider";

const mocks = vi.hoisted(() => ({ get: vi.fn(), update: vi.fn() }));
vi.mock("@/lib/api/domains/azure-devops-api", () => ({
  getAzureDevOpsWorkspaceSettings: mocks.get,
  updateAzureDevOpsWorkspaceSettings: mocks.update,
}));
vi.mock("@/components/toast-provider", () => ({ useToast: () => ({ toast: vi.fn() }) }));

import { AzureDevOpsQuickActionsSection } from "./azure-devops-quick-actions";

beforeEach(() => {
  vi.clearAllMocks();
  const settings = {
    workItemActions: [
      {
        id: "implement",
        label: "Implement",
        hint: "Build it",
        icon: "code",
        promptTemplate: "Implement {{url}}",
      },
    ],
    pullRequestActions: [
      {
        id: "review",
        label: "Review",
        hint: "Read it",
        icon: "eye",
        promptTemplate: "Review {{url}}",
      },
    ],
  };
  mocks.get.mockResolvedValue(settings);
  mocks.update.mockResolvedValue(settings);
});
afterEach(cleanup);

function renderSection() {
  render(
    <SettingsSaveProvider>
      <AzureDevOpsQuickActionsSection workspaceId="workspace-1" />
    </SettingsSaveProvider>,
  );
}

describe("AzureDevOpsQuickActionsSection", () => {
  it("uses the GitHub-style tabbed action and prompt editor", async () => {
    renderSection();
    await waitFor(() =>
      expect((screen.getByLabelText("Pull request action label 1") as HTMLInputElement).value).toBe(
        "Review",
      ),
    );
    expect(screen.getByRole("tab", { name: "Pull requests" })).toBeTruthy();
    fireEvent.mouseDown(screen.getByRole("tab", { name: "Work items" }), {
      button: 0,
      ctrlKey: false,
    });
    expect((screen.getByLabelText("Work item action label 1") as HTMLInputElement).value).toBe(
      "Implement",
    );
    fireEvent.mouseDown(screen.getByRole("tab", { name: "Pull requests" }), {
      button: 0,
      ctrlKey: false,
    });
    expect(screen.getByRole("button", { name: "Reset" })).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Edit prompt" }));
    // The hint is a `<Trans>` whose `<n>` indices address the JSX children
    // positionally, so a reflow of those children silently reassembles the
    // sentence into fragments. Assert the whole reconstructed hint, including
    // the three prompt tokens, which are passed as values so neither i18next
    // interpolation nor the pseudo-locale rewrites them.
    const hint = screen.getByText(/available placeholders/);
    expect(hint.textContent).toBe(
      "Type {{ to see available placeholders. {{url}} and {{title}} are substituted when the action runs.",
    );
  });

  it("keeps edits local until the shared Save changes action is pressed", async () => {
    renderSection();
    const label = await screen.findByLabelText("Pull request action label 1");

    fireEvent.change(label, { target: { value: "Inspect" } });

    expect(mocks.update).not.toHaveBeenCalled();
    fireEvent.click(await screen.findByRole("button", { name: "Save changes" }));
    await waitFor(() => expect(mocks.update).toHaveBeenCalledTimes(1));
    expect(mocks.update).toHaveBeenCalledWith(
      "workspace-1",
      expect.objectContaining({
        pullRequestActions: [expect.objectContaining({ label: "Inspect" })],
      }),
    );
  });

  it("blocks saving when loading the existing quick actions fails", async () => {
    mocks.get.mockRejectedValueOnce(new Error("Settings unavailable"));
    renderSection();

    expect((await screen.findByRole("alert")).textContent).toContain("Settings unavailable");
    expect((screen.getByRole("button", { name: "Reset" }) as HTMLButtonElement).disabled).toBe(
      true,
    );
    expect(screen.queryByRole("button", { name: "Save changes" })).toBeNull();
    expect(mocks.update).not.toHaveBeenCalled();
  });
});
