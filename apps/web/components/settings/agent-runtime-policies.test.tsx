import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { AgentUpdateStatus } from "@/lib/api";
import { StateProvider } from "@/components/state-provider";
import {
  RuntimePolicyControllers,
  RuntimePolicyDetail,
  useRuntimePolicyControllers,
} from "./agent-runtime-policies";
import { RuntimePolicyDirtyFooter } from "./agent-runtime-policy-footer";
import { SettingsSaveProvider } from "./settings-save-provider";

const setAutomatic = vi.fn();
const refreshStatuses = vi.fn();

vi.mock("@/lib/api", () => ({
  setAgentAutomaticUpdates: (...args: unknown[]) => setAutomatic(...args),
}));
vi.mock("@/lib/agents/runtime-update-statuses", () => ({
  refreshRuntimeUpdateStatuses: (...args: unknown[]) => refreshStatuses(...args),
}));
vi.mock("@/hooks/domains/auth/use-is-admin", () => ({ useIsAdmin: () => true }));

const status: AgentUpdateStatus = {
  agent_name: "opencode-acp",
  display_name: "OpenCode",
  runtime_id: "npm:opencode-ai",
  owner: "kandev",
  mechanism: "npm_candidate",
  management: "managed",
  update_mode: "pinned",
  package: "opencode-ai",
  source: "opencode-ai",
  available: true,
  enabled: true,
  auto_update_supported: true,
  auto_update: false,
  default_version: "1.0.0",
  effective_version: "1.0.0",
  current_version: "1.0.0",
  latest_version: "1.1.0",
  check_state: "update_available",
};

function FooterHarness() {
  const controllers = useRuntimePolicyControllers();
  return <RuntimePolicyDirtyFooter controllers={controllers} canManage embedded />;
}

function Harness({ showDetail = true }: { showDetail?: boolean }) {
  return (
    <StateProvider>
      <SettingsSaveProvider>
        <RuntimePolicyControllers statuses={[status]}>
          {showDetail && <RuntimePolicyDetail status={status} />}
          <FooterHarness />
        </RuntimePolicyControllers>
      </SettingsSaveProvider>
    </StateProvider>
  );
}

beforeEach(() => {
  setAutomatic.mockResolvedValue(undefined);
  refreshStatuses.mockResolvedValue(true);
});
afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe("runtime policy drafts", () => {
  // @covers AC-AGENTS-RUNTIME-MANAGER-001.5
  it("keeps a draft and reachable Save/Reset while the detail view unmounts", async () => {
    const view = render(<Harness />);
    const toggle = screen.getByRole("switch");
    fireEvent.click(toggle);
    await waitFor(() => expect(screen.getByTestId("runtime-policy-save")).toBeTruthy());

    view.rerender(<Harness showDetail={false} />);
    expect(screen.getByTestId("runtime-policy-dirty-footer")).toBeTruthy();
    expect(screen.getByTestId("runtime-policy-reset")).toBeTruthy();
  });

  it("resets the draft without writing when Reset is used", async () => {
    render(<Harness />);
    fireEvent.click(screen.getByRole("switch"));
    await waitFor(() => expect(screen.getByTestId("runtime-policy-reset")).toBeTruthy());
    fireEvent.click(screen.getByTestId("runtime-policy-reset"));
    await waitFor(() => expect(screen.queryByTestId("runtime-policy-dirty-footer")).toBeNull());
    expect(screen.getByRole("switch").getAttribute("aria-checked")).toBe("false");
    expect(setAutomatic).not.toHaveBeenCalled();
  });

  it("saves the draft explicitly through the shared coordinator", async () => {
    render(<Harness />);
    fireEvent.click(screen.getByRole("switch"));
    await waitFor(() => expect(screen.getByTestId("runtime-policy-save")).toBeTruthy());
    fireEvent.click(screen.getByTestId("runtime-policy-save"));
    await waitFor(() => expect(setAutomatic).toHaveBeenCalledWith("opencode-acp", true));
    await waitFor(() => expect(screen.queryByTestId("runtime-policy-dirty-footer")).toBeNull());
  });

  it("keeps manual guidance and retained outcomes in the details", () => {
    const manual: AgentUpdateStatus = {
      ...status,
      owner: "external",
      management: "manual",
      mechanism: "native",
      managed_fallback: true,
      update_mode: undefined,
      guidance_url: "https://opencode.ai/docs/cli/",
      last_outcome: {
        id: "outcome-1",
        status: "interrupted",
        previous_version: "1.0.0",
        target_version: "1.1.0",
        finished_at: "2026-01-01T00:00:00Z",
      },
    };
    render(
      <StateProvider>
        <SettingsSaveProvider>
          <RuntimePolicyControllers statuses={[manual]}>
            <RuntimePolicyDetail status={manual} />
          </RuntimePolicyControllers>
        </SettingsSaveProvider>
      </StateProvider>,
    );
    expect(screen.getByRole("link", { name: "Manual update guidance" }).getAttribute("href")).toBe(
      "https://opencode.ai/docs/cli/",
    );
    expect(screen.getByRole("status").textContent).toContain("could not be confirmed");
    expect(screen.queryByRole("switch")).toBeNull();
  });
});
