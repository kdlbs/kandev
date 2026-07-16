import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { SettingsSaveProvider } from "./settings-save-provider";
import { SSHConnectionCard, type SSHExecutorConfig } from "./ssh-connection-card";
import { testSSHConnection } from "@/lib/api/domains/ssh-api";

vi.mock("@/lib/api/domains/ssh-api", () => ({ testSSHConnection: vi.fn() }));

const initial: SSHExecutorConfig = {
  name: "Primary host",
  host: "host.example.com",
  identity_source: "agent",
  host_fingerprint: "SHA256:old",
};

describe("SSHConnectionCard", () => {
  it("uses the shared save action for an existing executor", async () => {
    vi.mocked(testSSHConnection).mockResolvedValue({
      success: true,
      fingerprint: "SHA256:old",
      steps: [],
      total_duration_ms: 1,
    });
    const onSave = vi.fn().mockResolvedValue(undefined);
    render(
      <SettingsSaveProvider>
        <SSHConnectionCard
          initial={initial}
          onSave={onSave}
          coordinatedSaveId="ssh-executor:executor-1"
        />
      </SettingsSaveProvider>,
    );

    fireEvent.change(screen.getByTestId("ssh-input-name"), {
      target: { value: "Renamed host" },
    });
    expect(screen.getByTestId("ssh-input-name").getAttribute("data-settings-dirty")).toBe("true");
    expect(screen.getByTestId("ssh-connection-card").getAttribute("data-settings-dirty")).toBe(
      "true",
    );
    expect(screen.queryByTestId("ssh-save-button")).toBeNull();
    expect(onSave).not.toHaveBeenCalled();

    fireEvent.click(screen.getByTestId("ssh-test-button"));
    await waitFor(() => expect(screen.getByTestId("ssh-trust-checkbox")).toBeTruthy());
    fireEvent.click(screen.getByTestId("ssh-trust-checkbox"));
    fireEvent.click(screen.getByRole("button", { name: "Save changes" }));

    await waitFor(() =>
      expect(onSave).toHaveBeenCalledWith(
        expect.objectContaining({ name: "Renamed host", host_fingerprint: "SHA256:old" }),
      ),
    );
    await waitFor(() =>
      expect(screen.getByTestId("ssh-connection-card").getAttribute("data-settings-dirty")).toBe(
        "false",
      ),
    );
  });
});
