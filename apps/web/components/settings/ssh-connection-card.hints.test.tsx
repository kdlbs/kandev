import { cleanup, fireEvent, render, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { SettingsSaveProvider } from "./settings-save-provider";
import { SSHConnectionCard, type SSHExecutorConfig } from "./ssh-connection-card";
import type { SSHTestResult, SSHTestStep } from "@/lib/types/http-ssh";

vi.mock("@/lib/api/domains/ssh-api", () => ({ testSSHConnection: vi.fn() }));

const DAEMON_STEP_TESTID = "ssh-test-step-docker-daemon";
const DAEMON_HINT_TESTID = `${DAEMON_STEP_TESTID}-hint`;

const initial: SSHExecutorConfig = {
  name: "build-box",
  host: "build-box",
  identity_source: "agent",
};

function failedDaemonStep(hint?: string): SSHTestStep {
  return {
    name: "Docker daemon",
    duration_ms: 227,
    success: false,
    error: "docker ping failed: docker: remote user cannot access the Docker socket",
    ...(hint ? { hint } : {}),
  };
}

function resultWith(step: SSHTestStep): SSHTestResult {
  return { success: false, steps: [step], total_duration_ms: 895 };
}

async function renderTestedCard(result: SSHTestResult) {
  const test = vi.fn().mockResolvedValue(result);
  const view = render(
    <SettingsSaveProvider>
      <SSHConnectionCard initial={initial} onSave={vi.fn()} testConnection={test} />
    </SettingsSaveProvider>,
  );
  // The card renders its actions in both desktop and mobile layouts;
  // either instance drives the same handler.
  fireEvent.click(view.getAllByTestId("ssh-test-button")[0]);
  await waitFor(() => expect(test).toHaveBeenCalled());
  return view;
}

describe("SSHConnectionCard step remediation", () => {
  afterEach(() => {
    cleanup();
    vi.clearAllMocks();
  });

  // The daemon's own words name the failure but not the fix, and the three
  // causes need three different fixes on the remote host.
  it("shows the remediation for the hint the backend reported", async () => {
    const view = await renderTestedCard(
      resultWith(failedDaemonStep("remote_user_needs_docker_access")),
    );

    const hint = await waitFor(() => view.getAllByTestId(DAEMON_HINT_TESTID)[0]);
    expect(hint.textContent).toContain("docker group");
  });

  it("names the missing CLI rather than a dead daemon", async () => {
    const view = await renderTestedCard(
      resultWith(failedDaemonStep("remote_host_needs_docker_cli")),
    );

    const hint = await waitFor(() => view.getAllByTestId(DAEMON_HINT_TESTID)[0]);
    expect(hint.textContent).toContain("Docker CLI");
  });

  it("renders no remediation when the step carries no hint", async () => {
    const view = await renderTestedCard(resultWith(failedDaemonStep()));

    await waitFor(() => expect(view.getAllByTestId(DAEMON_STEP_TESTID)).not.toHaveLength(0));
    expect(view.queryAllByTestId(DAEMON_HINT_TESTID)).toHaveLength(0);
  });

  // An unrecognized identifier is a backend the frontend has not caught up
  // with. Showing nothing is correct; showing the raw identifier is not.
  it("renders no remediation for an unknown hint", async () => {
    const view = await renderTestedCard(resultWith(failedDaemonStep("remote_host_is_on_fire")));

    await waitFor(() => expect(view.getAllByTestId(DAEMON_STEP_TESTID)).not.toHaveLength(0));
    expect(view.queryAllByTestId(DAEMON_HINT_TESTID)).toHaveLength(0);
  });
});
