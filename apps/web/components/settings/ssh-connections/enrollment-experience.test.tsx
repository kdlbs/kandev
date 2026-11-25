import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import type { SSHManager } from "@/hooks/domains/settings/use-ssh-connections";
import type { SSHEnrollment, SSHRoute } from "@/lib/types/http-ssh-connections";
import { EnrollmentPanel } from "./enrollment-panel";
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
    fingerprint: "SHA256:host",
    public_key: "public",
    version: 1,
    revision: 1,
  },
  stamp: "revision",
};
const passwordLabel = "settings:sshManager.enrollment.password";
const reviewLabel = "settings:sshManager.experience.reviewInstallation";
const finalLabel = "settings:sshManager.enrollment.install";
function setup() {
  const enroll = vi
    .fn()
    .mockResolvedValue({ state: "verified", stamp: route.stamp } as SSHEnrollment);
  const manager = {
    busy: false,
    enroll,
    identities: [
      {
        id: "key",
        label: "Key",
        kind: "managed",
        public_key: "ssh-ed25519 public",
        fingerprint: "SHA256:key",
      },
    ],
  } as unknown as SSHManager;
  return { enroll, manager, ...render(<EnrollmentPanel route={route} manager={manager} />) };
}
function choosePassword() {
  fireEvent.click(
    screen.getByRole("radio", { name: "settings:sshManager.experience.methodPassword" }),
  );
}
function reviewPassword() {
  choosePassword();
  fireEvent.change(screen.getByLabelText(passwordLabel), { target: { value: "fixture-password" } });
  fireEvent.click(screen.getByRole("button", { name: reviewLabel }));
}
// @covers AC-EXECUTORS-SSH-CONNECTIONS-004.15, 004.16
it("requires a method and a separate review before submitting", async () => {
  const { enroll } = setup();
  expect(screen.queryByLabelText(passwordLabel)).toBeNull();
  reviewPassword();
  expect(enroll).not.toHaveBeenCalled();
  expect(screen.queryByLabelText(passwordLabel)).toBeNull();
  expect(screen.getByText("deploy@host.invalid:22")).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: finalLabel }));
  await waitFor(() => expect(enroll).toHaveBeenCalledTimes(1));
  expect(enroll.mock.calls[0][1]).toMatchObject({
    expected_stamp: "revision",
    key_fingerprint: "SHA256:key",
    mode: "password",
    password: "fixture-password",
  });
});
it("discards credentials on review Back and responsive invalidation", async () => {
  const { enroll, manager, rerender } = setup();
  reviewPassword();
  fireEvent.click(screen.getByRole("button", { name: "common:back" }));
  expect((screen.getByLabelText(passwordLabel) as HTMLInputElement).value).toBe("");
  fireEvent.change(screen.getByLabelText(passwordLabel), { target: { value: "second-fixture" } });
  fireEvent.click(screen.getByRole("button", { name: reviewLabel }));
  viewport.isMobile = true;
  rerender(<EnrollmentPanel route={route} manager={manager} />);
  await waitFor(() =>
    expect((screen.getByLabelText(passwordLabel) as HTMLInputElement).value).toBe(""),
  );
  expect(enroll).not.toHaveBeenCalled();
});
it("admits only one install and clears credential controls while it is pending", async () => {
  const { enroll } = setup();
  let finish!: (value: SSHEnrollment) => void;
  enroll.mockReturnValue(
    new Promise((resolve) => {
      finish = resolve;
    }),
  );
  reviewPassword();
  const install = screen.getByRole("button", { name: finalLabel });
  fireEvent.click(install);
  fireEvent.click(install);
  expect(enroll).toHaveBeenCalledTimes(1);
  expect(screen.getByRole("button", { name: finalLabel })).toBe(install);
  expect(screen.queryByLabelText(passwordLabel)).toBeNull();
  await act(async () => {
    finish({ state: "outcome_unknown", stamp: route.stamp } as SSHEnrollment);
  });
  expect(screen.getByText("settings:sshManager.enrollment.states.outcome_unknown")).toBeTruthy();
});
it("manual installation never invokes enrollment or claims success", () => {
  const { enroll } = setup();
  fireEvent.click(
    screen.getByRole("radio", { name: "settings:sshManager.experience.methodManual" }),
  );
  expect(screen.getByRole("button", { name: "settings:sshManager.copyPublicKey" })).toBeTruthy();
  expect(screen.queryByRole("button", { name: finalLabel })).toBeNull();
  expect(enroll).not.toHaveBeenCalled();
});
it("clears the password on method change and sends only the chosen identity", async () => {
  const { enroll } = setup();
  choosePassword();
  fireEvent.change(screen.getByLabelText(passwordLabel), { target: { value: "discarded" } });
  fireEvent.click(
    screen.getByRole("radio", { name: "settings:sshManager.experience.methodIdentity" }),
  );
  fireEvent.change(screen.getByLabelText("settings:sshManager.enrollment.existingIdentity"), {
    target: { value: "key" },
  });
  fireEvent.click(screen.getByRole("button", { name: reviewLabel }));
  fireEvent.click(screen.getByRole("button", { name: finalLabel }));
  await waitFor(() => expect(enroll).toHaveBeenCalledTimes(1));
  expect(enroll.mock.calls[0][1]).toMatchObject({ mode: "identity", bootstrap_identity_id: "key" });
  expect(enroll.mock.calls[0][1]).not.toHaveProperty("password");
});
it("form submission opens review and Escape clears the password", () => {
  const { enroll } = setup();
  choosePassword();
  const password = screen.getByLabelText(passwordLabel);
  fireEvent.change(password, { target: { value: "discarded" } });
  fireEvent.submit(password.closest("form")!);
  expect(enroll).not.toHaveBeenCalled();
  fireEvent.keyDown(screen.getByRole("heading", { name: reviewLabel }), { key: "Escape" });
  expect((screen.getByLabelText(passwordLabel) as HTMLInputElement).value).toBe("");
});
it.each(["failed", "installed_unverified", "outcome_unknown", "started"] as const)(
  "preserves the %s result without announcing verified access",
  async (state) => {
    const { enroll } = setup();
    enroll.mockResolvedValue({ state, stamp: route.stamp });
    reviewPassword();
    fireEvent.click(screen.getByRole("button", { name: finalLabel }));
    await screen.findByText(`settings:sshManager.enrollment.states.${state}`);
    expect(screen.queryByText("settings:sshManager.enrollment.states.verified")).toBeNull();
    expect(screen.queryByLabelText(passwordLabel)).toBeNull();
  },
);
it("drops the reviewed password when the selected key fingerprint changes", () => {
  const { enroll, manager, rerender } = setup();
  reviewPassword();
  manager.identities[0] = { ...manager.identities[0], fingerprint: "SHA256:replacement" };
  rerender(<EnrollmentPanel route={route} manager={manager} />);
  expect(screen.queryByRole("button", { name: finalLabel })).toBeNull();
  choosePassword();
  expect((screen.getByLabelText(passwordLabel) as HTMLInputElement).value).toBe("");
  expect(enroll).not.toHaveBeenCalled();
});

it("keeps a focused password draft across a phone layout change before review", () => {
  const { enroll, manager, rerender } = setup();
  choosePassword();
  const password = screen.getByLabelText(passwordLabel);
  fireEvent.change(password, { target: { value: "draft-password" } });
  password.focus();
  viewport.isMobile = true;
  rerender(<EnrollmentPanel route={route} manager={manager} />);
  expect(screen.getByLabelText(passwordLabel)).toBe(password);
  expect((password as HTMLInputElement).value).toBe("draft-password");
  expect(document.activeElement).toBe(password);
  expect(enroll).not.toHaveBeenCalled();
});
