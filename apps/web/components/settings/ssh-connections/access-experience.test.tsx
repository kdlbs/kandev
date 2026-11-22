import { useState } from "react";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import type { SSHManager } from "@/hooks/domains/settings/use-ssh-connections";
import type { SSHObservation, SSHRoute } from "@/lib/types/http-ssh-connections";
import { ConnectionTrust } from "./connection-trust";

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
function setup() {
  const trust = vi.fn().mockResolvedValue(route);
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
      />
    );
  }
  return { trust, Owner, ...render(<Owner />) };
}
// @covers AC-EXECUTORS-SSH-CONNECTIONS-004.16
it("commits the fully displayed inline trust review once without a second popup", async () => {
  const { trust } = setup();
  expect(screen.getByText("SHA256:old")).toBeTruthy();
  expect(screen.getByText("SHA256:new")).toBeTruthy();
  const confirm = screen.getByRole("button", { name: "settings:sshManager.trust" });
  fireEvent.click(confirm);
  await waitFor(() => expect(trust).toHaveBeenCalledWith(route, observation));
  expect(screen.queryByRole("dialog")).toBeNull();
});
it("discards an inline trust decision when its route changes", async () => {
  const { trust, Owner, rerender } = setup();
  rerender(<Owner current={{ ...route, stamp: "changed" }} />);
  await waitFor(() =>
    expect(screen.queryByRole("button", { name: "settings:sshManager.trust" })).toBeNull(),
  );
  expect(trust).not.toHaveBeenCalled();
});
it("discards the pending decision on a phone boundary change", async () => {
  const { trust, Owner, rerender } = setup();
  viewport.isMobile = true;
  rerender(<Owner />);
  await waitFor(() =>
    expect(screen.queryByRole("button", { name: "settings:sshManager.trust" })).toBeNull(),
  );
  expect(trust).not.toHaveBeenCalled();
});
