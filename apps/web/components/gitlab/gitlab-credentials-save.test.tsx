import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { SettingsSaveProvider } from "@/components/settings/settings-save-provider";

const setGitLabConfig = vi.hoisted(() => vi.fn().mockResolvedValue({}));
vi.mock("@/lib/api/domains/gitlab-api", () => ({ setGitLabConfig }));
vi.mock("@/components/toast-provider", () => ({ useToast: () => ({ toast: vi.fn() }) }));

import { GitLabCredentialsForm } from "./gitlab-settings";

describe("GitLabCredentialsForm coordinated save", () => {
  it("switches from glab to PAT with one atomic config request", async () => {
    render(
      <SettingsSaveProvider>
        <GitLabCredentialsForm
          initial="glab_cli"
          initialHost="https://gitlab.example"
          host="https://gitlab.example"
          workspaceId="ws-1"
          hasToken={false}
          onChange={() => undefined}
          onSaved={() => undefined}
          onDirtyChange={() => undefined}
          onHostChange={() => undefined}
        />
      </SettingsSaveProvider>,
    );

    fireEvent.click(screen.getByRole("combobox", { name: "Authentication method" }));
    fireEvent.click(screen.getByRole("option", { name: "Personal access token" }));
    fireEvent.change(screen.getByPlaceholderText("glpat-xxxxxxxxxxxxxxxxxxxx"), {
      target: { value: "glpat-atomic" },
    });
    fireEvent.click(await screen.findByRole("button", { name: "Save changes" }));

    await waitFor(() => expect(setGitLabConfig).toHaveBeenCalledTimes(1));
    expect(setGitLabConfig).toHaveBeenCalledWith(
      {
        host: "https://gitlab.example",
        auth_method: "pat",
        token: "glpat-atomic",
      },
      { workspaceId: "ws-1" },
    );
  });
});
