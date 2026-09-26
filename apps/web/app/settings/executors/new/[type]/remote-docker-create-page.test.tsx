import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { StateProvider } from "@/components/state-provider";
import { SettingsSaveProvider } from "@/components/settings/settings-save-provider";
import { RemoteDockerCreatePage } from "./remote-docker-create-page";
import {
  createExecutor,
  createExecutorProfile,
  deleteExecutor,
} from "@/lib/api/domains/settings-api";
import { testRemoteDockerConnection } from "@/lib/api/domains/remote-docker-api";

vi.mock("@/lib/api/domains/settings-api", () => ({
  createExecutor: vi.fn(),
  createExecutorProfile: vi.fn(),
  deleteExecutor: vi.fn(),
}));
vi.mock("@/lib/api/domains/remote-docker-api", () => ({
  testRemoteDockerConnection: vi.fn(),
}));

const push = vi.fn();
vi.mock("@/lib/routing/client-router", () => ({ useRouter: () => ({ push }) }));

const EXECUTOR_ID = "executor-1";
const NOW = "2026-09-17T00:00:00Z";

const EXECUTOR = {
  id: EXECUTOR_ID,
  name: "prod-box",
  type: "remote_docker" as const,
  status: "active" as const,
  is_system: false,
  config: {},
  created_at: NOW,
  updated_at: NOW,
};

const PROFILE = {
  id: "profile-1",
  executor_id: EXECUTOR_ID,
  name: "prod-box",
  prepare_script: "",
  cleanup_script: "",
  created_at: NOW,
  updated_at: NOW,
};

/** Drives the card to a tested-and-trusted state, then saves. */
async function testTrustAndSave() {
  vi.mocked(testRemoteDockerConnection).mockResolvedValue({
    success: true,
    fingerprint: "SHA256:abc",
    steps: [{ name: "SSH connection", success: true, duration_ms: 1 }],
    total_duration_ms: 1,
  });

  fireEvent.change(screen.getByTestId("ssh-input-name"), { target: { value: "prod-box" } });
  fireEvent.change(screen.getByTestId("ssh-input-host"), { target: { value: "10.0.0.5" } });
  fireEvent.click(screen.getAllByTestId("ssh-test-button")[0]);

  await waitFor(() => expect(vi.mocked(testRemoteDockerConnection)).toHaveBeenCalled());
  fireEvent.click(screen.getAllByTestId("ssh-trust-checkbox")[0]);
  fireEvent.click(screen.getAllByTestId("ssh-save-button")[0]);
}

function renderPage() {
  return render(
    <StateProvider initialState={{ executors: { items: [] } }}>
      <SettingsSaveProvider>
        <RemoteDockerCreatePage />
      </SettingsSaveProvider>
    </StateProvider>,
  );
}

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe("RemoteDockerCreatePage save", () => {
  // The hub lists profiles, not bare executors. An executor saved without its
  // default profile is invisible there, so the administrator sees a failed
  // attempt while the row persists and a retry creates a duplicate.
  it("removes the executor when its default profile cannot be created", async () => {
    vi.mocked(createExecutor).mockResolvedValue(EXECUTOR);
    vi.mocked(createExecutorProfile).mockRejectedValue(new Error("profile create failed"));
    vi.mocked(deleteExecutor).mockResolvedValue({ success: true });

    renderPage();
    await testTrustAndSave();

    await waitFor(() => expect(vi.mocked(deleteExecutor)).toHaveBeenCalledWith(EXECUTOR_ID));
    expect(push).not.toHaveBeenCalled();
  });

  it("keeps the executor and navigates when both calls succeed", async () => {
    vi.mocked(createExecutor).mockResolvedValue(EXECUTOR);
    vi.mocked(createExecutorProfile).mockResolvedValue(PROFILE);

    renderPage();
    await testTrustAndSave();

    await waitFor(() => expect(push).toHaveBeenCalledWith("/settings/executors/profile-1"));
    expect(vi.mocked(deleteExecutor)).not.toHaveBeenCalled();
  });
});
