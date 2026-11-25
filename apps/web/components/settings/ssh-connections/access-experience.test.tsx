import { useState } from "react";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import type { SSHManager } from "@/hooks/domains/settings/use-ssh-connections";
import type { SSHObservation, SSHRoute } from "@/lib/types/http-ssh-connections";
import { ConnectionTrust } from "./connection-trust";
import { ConnectionAccess } from "./connection-access";

const viewport = vi.hoisted(() => ({ isMobile: false, isFinePointer: true }));
vi.mock("@/hooks/use-responsive-breakpoint", () => ({ useResponsiveBreakpoint: () => viewport }));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (key: string) => key }) }));
afterEach(() => {
  cleanup();
  viewport.isMobile = false;
});
const route: SSHRoute = {
  destination: {
    id: "host",
    name: "Host",
    host: "host.invalid",
    port: 22,
    user: "deploy",
    identity_id: "key",
    fingerprint: "SHA256:old",
    public_key: "public",
    version: 1,
    revision: 1,
  },
  stamp: "revision",
};
const observation: SSHObservation = {
  receipt: "receipt",
  stamp: "revision",
  fingerprint: "SHA256:new",
  public_key: "public",
};
const confirmLabel = "settings:sshManager.trust";
function setup() {
  const trust = vi.fn().mockResolvedValue(route);
  const onNewConnection = vi.fn();
  const manager = {
    busy: false,
    trust,
    identities: [],
    observe: vi.fn(),
    test: vi.fn(),
  } as unknown as SSHManager;
  function Owner({ current = route }: { current?: SSHRoute }) {
    const [observed, setObserved] = useState<SSHObservation | null>(observation);
    return (
      <ConnectionTrust
        route={current}
        manager={manager}
        observation={observed}
        setObservation={setObserved}
        onNewConnection={onNewConnection}
      />
    );
  }
  return { trust, manager, onNewConnection, Owner, ...render(<Owner />) };
}
// @covers AC-EXECUTORS-SSH-CONNECTIONS-004.16
it("commits the fully displayed inline trust review once without a second popup", async () => {
  const { trust } = setup();
  expect(screen.getByText("SHA256:old")).toBeTruthy();
  expect(screen.getByText("SHA256:new")).toBeTruthy();
  const confirm = screen.getByRole("button", { name: confirmLabel });
  fireEvent.click(confirm);
  await waitFor(() => expect(trust).toHaveBeenCalledWith(route, observation));
  expect(screen.queryByRole("dialog")).toBeNull();
});
it("discards an inline trust decision when its route changes", async () => {
  const { trust, Owner, rerender } = setup();
  rerender(<Owner current={{ ...route, stamp: "changed" }} />);
  await waitFor(() => expect(screen.queryByRole("button", { name: confirmLabel })).toBeNull());
  expect(trust).not.toHaveBeenCalled();
});
it("discards the pending decision on a phone boundary change", async () => {
  const { trust, Owner, rerender } = setup();
  viewport.isMobile = true;
  rerender(<Owner />);
  await waitFor(() => expect(screen.queryByRole("button", { name: confirmLabel })).toBeNull());
  expect(trust).not.toHaveBeenCalled();
});

// @covers AC-EXECUTORS-SSH-CONNECTIONS-004.16, 004.17
it("keeps both keys and offers a fresh connection when existing trust is protected", () => {
  const { manager, onNewConnection, Owner, rerender } = setup();
  const confirmation = screen.getByRole("button", { name: confirmLabel });
  confirmation.focus();
  manager.operation = {
    kind: "trust",
    connectionId: route.destination.id,
    stamp: route.stamp,
    status: "failed",
    error: "ssh_resource_in_use",
  };
  rerender(<Owner />);
  expect(screen.getByText("SHA256:old")).toBeTruthy();
  expect(screen.getByText("SHA256:new")).toBeTruthy();
  expect(screen.getByRole("alert").textContent).toContain("trustInUseHelp");
  expect(screen.queryByText("settings:sshManager.errors.ssh_resource_in_use")).toBeNull();
  const recovery = screen.getByRole("button", { name: "settings:sshManager.newConnection" });
  expect(document.activeElement).toBe(recovery);
  fireEvent.click(recovery);
  expect(onNewConnection).toHaveBeenCalledTimes(1);
});

it.each(["ssh_trust_required", "ssh_management_forbidden"])(
  "preserves operation feedback for %s",
  (error) => {
    const { manager, Owner, rerender } = setup();
    manager.operation = {
      kind: "trust",
      connectionId: route.destination.id,
      stamp: route.stamp,
      status: "failed",
      error,
    };
    rerender(<Owner />);
    expect(screen.getByRole("alert").textContent).toContain(error);
    expect(screen.queryByRole("button", { name: "settings:sshManager.newConnection" })).toBeNull();
  },
);

it("groups recovery actions with a rejected login and retains the live test control", () => {
  const onSetupChecks = vi.fn();
  const manager = {
    test: vi.fn(),
    busy: true,
    identities: [{ id: "key", public_key: "ssh-ed25519 fixture" }],
    failedTests: { host: route.stamp },
    operation: {
      kind: "test",
      connectionId: route.destination.id,
      stamp: route.stamp,
      status: "pending",
    },
  } as unknown as SSHManager;
  const renderAccess = () => (
    <ConnectionAccess
      route={route}
      manager={manager}
      onEdit={vi.fn()}
      onSetupChecks={onSetupChecks}
    />
  );
  const { rerender } = render(renderAccess());
  const button = screen.getByRole<HTMLButtonElement>("button", {
    name: "settings:sshManager.test",
  });
  expect(button.getAttribute("aria-disabled")).toBe("true");
  manager.busy = false;
  manager.operation = {
    ...manager.operation!,
    status: "failed",
    error: "ssh_authentication_rejected",
  };
  rerender(renderAccess());
  expect(screen.getByRole("alert").textContent).toContain("ssh_authentication_rejected");
  expect(screen.getByRole("button", { name: "settings:sshManager.test" })).toBe(button);
  expect(screen.getByRole("button", { name: "settings:sshManager.ui.installKey" })).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "settings:sshManager.experience.setup" }));
  expect(onSetupChecks).toHaveBeenCalledTimes(1);
  expect(manager.test).not.toHaveBeenCalled();
});
