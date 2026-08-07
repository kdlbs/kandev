import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { SettingsSaveProvider } from "@/components/settings/settings-save-provider";

const mocks = vi.hoisted(() => ({ get: vi.fn(), update: vi.fn() }));
vi.mock("@/lib/api/domains/azure-devops-api", () => ({
  getAzureDevOpsWorkspaceSettings: mocks.get,
  updateAzureDevOpsWorkspaceSettings: mocks.update,
}));
vi.mock("@/components/toast-provider", () => ({ useToast: () => ({ toast: vi.fn() }) }));

import { AzureDevOpsDefaultQueriesSection } from "./azure-devops-default-queries";

const settings = {
  workspaceId: "workspace-1",
  workItemQueries: [
    {
      id: "recent",
      label: "Recently updated",
      group: "inbox",
      filters: { wiql: "SELECT [System.Id] FROM WorkItems", top: 50 },
    },
  ],
  pullRequestQueries: [
    {
      id: "review-requested",
      label: "Needs my review",
      group: "inbox",
      filters: { status: "active", reviewer: "@me" },
    },
  ],
  workItemActions: [],
  pullRequestActions: [],
};

beforeEach(() => {
  vi.clearAllMocks();
  mocks.get.mockResolvedValue(settings);
  mocks.update.mockImplementation(async (_workspaceId, payload) => ({
    ...settings,
    workItemQueries: payload.workItemQueries ?? settings.workItemQueries,
    pullRequestQueries: payload.pullRequestQueries ?? settings.pullRequestQueries,
  }));
});
afterEach(cleanup);

function renderSection() {
  render(
    <SettingsSaveProvider>
      <AzureDevOpsDefaultQueriesSection workspaceId="workspace-1" />
    </SettingsSaveProvider>,
  );
}

describe("AzureDevOpsDefaultQueriesSection", () => {
  it("edits provider-native queries with the shared GitHub-style save flow", async () => {
    renderSection();
    const label = await screen.findByLabelText("Pull request query label 1");

    expect(screen.getByRole("tab", { name: "Pull requests" })).toBeTruthy();
    expect(screen.getByRole("tab", { name: "Work items" })).toBeTruthy();
    fireEvent.change(label, { target: { value: "Review queue" } });

    expect(mocks.update).not.toHaveBeenCalled();
    fireEvent.click(await screen.findByRole("button", { name: "Save changes" }));
    await waitFor(() => expect(mocks.update).toHaveBeenCalledTimes(1));
    expect(mocks.update).toHaveBeenCalledWith(
      "workspace-1",
      expect.objectContaining({
        pullRequestQueries: [expect.objectContaining({ label: "Review queue" })],
      }),
    );
  });

  it("resets both query families through explicit null overrides", async () => {
    renderSection();
    await screen.findByLabelText("Pull request query label 1");

    fireEvent.click(screen.getByRole("button", { name: "Reset" }));
    fireEvent.click(await screen.findByRole("button", { name: "Save changes" }));

    await waitFor(() => expect(mocks.update).toHaveBeenCalledTimes(1));
    expect(mocks.update).toHaveBeenCalledWith("workspace-1", {
      pullRequestQueries: null,
      workItemQueries: null,
    });
  });
});
