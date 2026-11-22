import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import type { SSHManager } from "@/hooks/domains/settings/use-ssh-connections";
import type { SSHRoute } from "@/lib/types/http-ssh-connections";
import { EnrollmentPanel, EnrollmentStatus } from "./enrollment-panel";
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (key: string) => key }) }));
vi.mock("./confirmation", () => ({
  ManagerConfirmation: ({
    label,
    onConfirm,
    busy,
  }: {
    label: string;
    onConfirm: () => Promise<void>;
    busy: boolean;
  }) => (
    <button disabled={busy} onClick={() => void onConfirm().catch(() => undefined)}>
      {label}
    </button>
  ),
}));
afterEach(cleanup);
const passwordLabel = "settings:sshManager.enrollment.password";
const routeStamp = "route-stamp";
it("clears the bootstrap password after failure and submits the exact route and public key", async () => {
  const enroll = vi.fn().mockRejectedValue(new Error("network failed"));
  const route = {
    destination: { id: "target", identity_id: "key", fingerprint: "host-pin" },
    stamp: routeStamp,
  } as SSHRoute;
  const manager = {
    busy: false,
    enroll,
    identities: [
      { id: "key", label: "key", public_key: "ssh-ed25519 public", fingerprint: "key-pin" },
    ],
  } as unknown as SSHManager;
  render(<EnrollmentPanel route={route} manager={manager} />);
  fireEvent.click(
    screen.getByRole("radio", { name: "settings:sshManager.experience.methodPassword" }),
  );
  fireEvent.change(screen.getByLabelText(passwordLabel), {
    target: { value: "one-time-secret" },
  });
  fireEvent.click(
    screen.getByRole("button", { name: "settings:sshManager.experience.reviewInstallation" }),
  );
  fireEvent.click(screen.getByRole("button", { name: "settings:sshManager.enrollment.install" }));
  await waitFor(() => expect(enroll).toHaveBeenCalledTimes(1));
  await waitFor(() => expect(screen.queryByLabelText(passwordLabel)).toBeNull());
  expect(enroll.mock.calls[0][1]).toMatchObject({
    expected_stamp: routeStamp,
    key_fingerprint: "key-pin",
    password: "one-time-secret",
    mode: "password",
  });
});

// @covers AC-EXECUTORS-SSH-CONNECTIONS-002.7, 003.5
it.each(["bastion", "destination"] as const)(
  "identifies the %s in a persisted enrollment failure",
  (hop) => {
    const route = {
      destination: { id: "target", identity_id: "key", fingerprint: "host-pin" },
      stamp: routeStamp,
      latest_enrollment: {
        request_id: "attempt",
        connection_id: "target",
        stamp: routeStamp,
        key_fingerprint: "key-pin",
        created_at: "2026-10-20T00:00:00Z",
        state: "failed",
        error_code: "ssh_host_key_changed",
        hop,
      },
    } as SSHRoute;
    render(<EnrollmentStatus latest={route.latest_enrollment} />);
    expect(
      screen.getByText(
        `settings:sshManager.${hop}: settings:sshManager.errors.ssh_host_key_changed`,
      ),
    ).toBeTruthy();
  },
);

it("clears credentials and obsolete outcomes when the approved route changes", () => {
  const route = {
    destination: { id: "target", identity_id: "key", fingerprint: "host-pin" },
    stamp: "old-route",
    latest_enrollment: {
      request_id: "attempt",
      connection_id: "target",
      stamp: "old-route",
      key_fingerprint: "key-pin",
      created_at: "2026-10-20T00:00:00Z",
      state: "verified",
    },
  } as SSHRoute;
  const manager = {
    busy: false,
    identities: [{ id: "key", public_key: "ssh-ed25519 public", fingerprint: "key-pin" }],
  } as unknown as SSHManager;
  const { rerender } = render(<EnrollmentPanel route={route} manager={manager} />);
  expect(screen.queryByText("settings:sshManager.enrollment.states.verified")).toBeNull();
  fireEvent.click(
    screen.getByRole("radio", { name: "settings:sshManager.experience.methodPassword" }),
  );
  fireEvent.change(screen.getByLabelText(passwordLabel), {
    target: { value: "old-route-password" },
  });
  rerender(<EnrollmentPanel route={{ ...route, stamp: "new-route" }} manager={manager} />);
  expect(screen.queryByLabelText(passwordLabel)).toBeNull();
  expect(screen.queryByText("settings:sshManager.enrollment.states.verified")).toBeNull();
});
