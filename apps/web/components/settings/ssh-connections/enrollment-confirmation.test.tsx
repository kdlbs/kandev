import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import type { SSHManager } from "@/hooks/domains/settings/use-ssh-connections";
import type { SSHRoute } from "@/lib/types/http-ssh-connections";
import { EnrollmentPanel } from "./enrollment-panel";

const viewport = vi.hoisted(() => ({ isMobile: false, isFinePointer: true }));
vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => viewport,
  useTouchDrawer: () => !viewport.isFinePointer,
}));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (key: string) => key }) }));
afterEach(cleanup);
const route: SSHRoute = {
  destination: {
    id: "target",
    name: "Target",
    host: "target.invalid",
    port: 22,
    user: "deploy",
    identity_id: "key",
    fingerprint: "SHA256:host",
    public_key: "ssh-ed25519 host",
    version: 1,
    revision: 1,
  },
  stamp: "route-stamp",
};
function setup() {
  const enroll = vi.fn().mockResolvedValue({});
  const manager = {
    busy: false,
    enroll,
    identities: [
      { id: "key", label: "key", public_key: "ssh-ed25519 public", fingerprint: "SHA256:key" },
    ],
  } as unknown as SSHManager;
  render(<EnrollmentPanel route={route} manager={manager} />);
  fireEvent.click(
    screen.getByRole("radio", { name: "settings:sshManager.experience.methodPassword" }),
  );
  const password = screen.getByLabelText("settings:sshManager.enrollment.password");
  fireEvent.change(password, { target: { value: "one-time-fixture-password" } });
  fireEvent.click(
    screen.getByRole("button", { name: "settings:sshManager.experience.reviewInstallation" }),
  );
  return { enroll };
}
for (const mobile of [false, true]) {
  // @covers AC-EXECUTORS-SSH-CONNECTIONS-004.3
  it(`clears the bootstrap password when ${mobile ? "phone" : "desktop"} confirmation is cancelled`, async () => {
    viewport.isMobile = mobile;
    viewport.isFinePointer = !mobile;
    const { enroll } = setup();
    fireEvent.click(screen.getByRole("button", { name: "common:back" }));
    await waitFor(() =>
      expect(
        (screen.getByLabelText("settings:sshManager.enrollment.password") as HTMLInputElement)
          .value,
      ).toBe(""),
    );
    expect(enroll).not.toHaveBeenCalled();
  });
  it(`submits the explicit password once from ${mobile ? "phone" : "desktop"} confirmation`, async () => {
    viewport.isMobile = mobile;
    viewport.isFinePointer = !mobile;
    const { enroll } = setup();
    fireEvent.click(screen.getByRole("button", { name: "settings:sshManager.enrollment.install" }));
    await waitFor(() => expect(enroll).toHaveBeenCalledTimes(1));
    expect(enroll.mock.calls[0][1].password).toBe("one-time-fixture-password");
    expect(screen.queryByLabelText("settings:sshManager.enrollment.password")).toBeNull();
  });
}
