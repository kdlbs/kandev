import { cleanup, fireEvent, render, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { SettingsSaveProvider } from "./settings-save-provider";
import { SSHConnectionCard, type SSHExecutorConfig } from "./ssh-connection-card";
import { testSSHConnection } from "@/lib/api/domains/ssh-api";

vi.mock("@/lib/api/domains/ssh-api", () => ({ testSSHConnection: vi.fn() }));

const initial: SSHExecutorConfig = {
  name: "build-box",
  host: "build-box",
  identity_source: "agent",
};

describe("SSHConnectionCard testConnection override", () => {
  // Queries are scoped to each render and the tree is unmounted between
  // tests; a leftover tree answers with stale, unmounted handlers.
  afterEach(() => {
    cleanup();
    vi.clearAllMocks();
  });

  it("runs the supplied endpoint instead of the SSH one", async () => {
    const remoteTest = vi.fn().mockResolvedValue({
      success: true,
      fingerprint: "SHA256:remote",
      steps: [{ name: "Docker daemon", success: true, duration_ms: 3 }],
      total_duration_ms: 5,
    });

    const view = render(
      <SettingsSaveProvider>
        <SSHConnectionCard initial={initial} onSave={vi.fn()} testConnection={remoteTest} />
      </SettingsSaveProvider>,
    );

    // The card renders its actions in both desktop and mobile layouts;
    // either instance drives the same handler.
    fireEvent.click(view.getAllByTestId("ssh-test-button")[0]);

    await waitFor(() => expect(remoteTest).toHaveBeenCalledTimes(1));
    // The SSH endpoint must not also fire: a remote Docker profile tested
    // against the SSH endpoint would report a green connection without ever
    // reaching the daemon.
    expect(vi.mocked(testSSHConnection)).not.toHaveBeenCalled();
    expect(remoteTest.mock.calls[0][0]).toMatchObject({ host: "build-box" });
  });

  it("still gates save on an explicit trust of the returned fingerprint", async () => {
    const remoteTest = vi.fn().mockResolvedValue({
      success: true,
      fingerprint: "SHA256:remote",
      steps: [],
      total_duration_ms: 1,
    });
    const onSave = vi.fn().mockResolvedValue(undefined);

    const view = render(
      <SettingsSaveProvider>
        <SSHConnectionCard initial={initial} onSave={onSave} testConnection={remoteTest} />
      </SettingsSaveProvider>,
    );

    // The card renders its actions in both desktop and mobile layouts;
    // either instance drives the same handler.
    fireEvent.click(view.getAllByTestId("ssh-test-button")[0]);
    await waitFor(() => expect(remoteTest).toHaveBeenCalled());

    const save = view.getAllByTestId("ssh-save-button")[0] as HTMLButtonElement;
    expect(save.disabled).toBe(true);

    fireEvent.click(view.getAllByTestId("ssh-trust-checkbox")[0]);
    await waitFor(() => expect(save.disabled).toBe(false));
  });
});
