import { act, cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { SandboxPolicyCreate } from "./sandbox-policy-create";
import type { SandboxProvider, SandboxRevision } from "@/lib/types/sandbox";

const api = vi.hoisted(() => ({ publishSandboxPolicy: vi.fn() }));
vi.mock("@/lib/api/domains/sandbox-api", () => api);
vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({ usesDesktopWorkbench: true }),
}));
const provider: SandboxProvider = {
  installation_id: "install",
  provider_key: "generic",
  display_name: "Provider",
  version: "1",
  available: true,
};
afterEach(cleanup);
beforeEach(() => vi.clearAllMocks());

async function openDraft() {
  fireEvent.click(screen.getByRole("button", { name: "Create policy" }));
  fireEvent.change(await screen.findByLabelText("Policy name"), {
    target: { value: "Restricted" },
  });
  return screen.getByRole("dialog");
}

describe("operator policy publication", () => {
  it("requires an explicit action and never publishes twice before rerender", async () => {
    let finish!: (value: SandboxRevision) => void;
    api.publishSandboxPolicy.mockImplementation(
      () =>
        new Promise((resolve) => {
          finish = resolve;
        }),
    );
    const onCreated = vi.fn();
    render(<SandboxPolicyCreate provider={provider} onCreated={onCreated} />);
    const dialog = await openDraft();
    expect(api.publishSandboxPolicy).not.toHaveBeenCalled();
    const action = within(dialog).getByRole("button", { name: "Create policy" });
    act(() => {
      fireEvent.click(action);
      fireEvent.click(action);
    });
    expect(api.publishSandboxPolicy).toHaveBeenCalledTimes(1);
    expect(api.publishSandboxPolicy.mock.calls[0][1]).toBe(0);
    const policy = api.publishSandboxPolicy.mock.calls[0][0];
    expect(policy.grants).toEqual({
      filesystem: [],
      network: [],
      environment_keys: [],
      kandev_mcp: false,
    });
    await act(async () => {
      finish({ id: "revision", policy } as SandboxRevision);
    });
    await waitFor(() => expect(onCreated).toHaveBeenCalledOnce());
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("does not notify a new settings scope after the original editor unmounts", async () => {
    let finish!: (value: SandboxRevision) => void;
    api.publishSandboxPolicy.mockImplementation(
      () =>
        new Promise((resolve) => {
          finish = resolve;
        }),
    );
    const onCreated = vi.fn();
    const view = render(<SandboxPolicyCreate provider={provider} onCreated={onCreated} />);
    const dialog = await openDraft();
    fireEvent.click(within(dialog).getByRole("button", { name: "Create policy" }));
    view.unmount();
    await act(async () => {
      finish({ id: "revision" } as SandboxRevision);
    });
    expect(onCreated).not.toHaveBeenCalled();
  });
});
