import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import type { SSHManager } from "@/hooks/domains/settings/use-ssh-connections";
import type { SSHEnrollment, SSHRoute } from "@/lib/types/http-ssh-connections";
import type { ManagerNavigation } from "./use-manager-navigation";
import { ManagerDetail } from "./manager-detail";
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (key: string) => key }) }));
vi.mock("./confirmation", () => ({
  ManagerConfirmation: ({
    label,
    onConfirm,
    busy,
  }: {
    label: string;
    onConfirm: () => Promise<unknown>;
    busy: boolean;
  }) => (
    <button
      disabled={busy}
      onClick={() => {
        void onConfirm().catch(() => undefined);
      }}
    >
      {label}
    </button>
  ),
}));
afterEach(cleanup);
const passwordLabel = "settings:sshManager.enrollment.password";
const installLabel = "settings:sshManager.ui.installKey";
function setup() {
  const route: SSHRoute = {
    destination: {
      id: "target",
      name: "Target",
      host: "target.invalid",
      port: 22,
      user: "deploy",
      identity_id: "key",
      fingerprint: "host-pin",
      public_key: "host-public",
      version: 1,
      revision: 1,
      verified_route: "stamp",
    },
    stamp: "stamp",
  };
  const manager = {
    busy: false,
    routes: [route],
    identities: [
      {
        id: "key",
        label: "Key",
        kind: "managed",
        public_key: "ssh-ed25519 public",
        fingerprint: "key-pin",
      },
    ],
    test: vi.fn(),
    observe: vi.fn(),
    preflight: vi.fn(),
    enroll: vi.fn(),
  } as unknown as SSHManager;
  let section = "access";
  const n = () =>
    ({
      selected: "target",
      identities: false,
      creating: false,
      editing: false,
      section,
      setSection: vi.fn(),
      edit: vi.fn(),
    }) as unknown as ManagerNavigation;
  const mounted = render(<ManagerDetail manager={manager} navigation={n()} />);
  return {
    route,
    manager,
    ...mounted,
    section: (next: string) => {
      section = next;
      mounted.rerender(<ManagerDetail manager={manager} navigation={n()} />);
    },
    refresh: () => mounted.rerender(<ManagerDetail manager={manager} navigation={n()} />),
  };
}
// @covers AC-EXECUTORS-SSH-CONNECTIONS-004.10, 004.11
it("keeps credentials absent until installation is explicitly requested", () => {
  const { manager } = setup();
  expect(screen.queryByLabelText(passwordLabel)).toBeNull();
  expect(manager.test).not.toHaveBeenCalled();
  expect(manager.observe).not.toHaveBeenCalled();
  expect(manager.preflight).not.toHaveBeenCalled();
  expect(manager.enroll).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", { name: installLabel }));
  expect(screen.getByLabelText(passwordLabel)).toBeTruthy();
});
it("clears secrets when leaving and reopening a previously visited Access view", () => {
  const result = setup();
  fireEvent.click(screen.getByRole("button", { name: installLabel }));
  fireEvent.change(screen.getByLabelText(passwordLabel), {
    target: { value: "fixture-only-secret" },
  });
  result.section("overview");
  expect(screen.queryByLabelText(passwordLabel)).toBeNull();
  result.section("access");
  expect(screen.queryByLabelText(passwordLabel)).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: installLabel }));
  expect((screen.getByLabelText(passwordLabel) as HTMLInputElement).value).toBe("");
});
it("replaces verified enrollment with its result and permits deliberate fresh setup", async () => {
  const result = setup();
  vi.mocked(result.manager.enroll).mockImplementation(async () => {
    const completed: SSHEnrollment = {
      request_id: "attempt",
      connection_id: "target",
      stamp: "stamp",
      key_fingerprint: "key-pin",
      created_at: "2026-10-26T00:00:00Z",
      state: "verified",
    };
    result.route.latest_enrollment = completed;
    return completed;
  });
  fireEvent.click(screen.getByRole("button", { name: installLabel }));
  fireEvent.change(screen.getByLabelText(passwordLabel), {
    target: { value: "fixture-only-secret" },
  });
  fireEvent.click(screen.getByRole("button", { name: "settings:sshManager.enrollment.install" }));
  await waitFor(() => expect(screen.queryByLabelText(passwordLabel)).toBeNull());
  expect(screen.getByText("settings:sshManager.enrollment.states.verified")).toBeTruthy();
  await waitFor(() =>
    expect(document.activeElement).toBe(
      screen.getByRole("button", { name: "settings:sshManager.test" }),
    ),
  );
  fireEvent.click(screen.getByRole("button", { name: installLabel }));
  expect((screen.getByLabelText(passwordLabel) as HTMLInputElement).value).toBe("");
});
it("does not reopen credentials when a dismissed attempt completes later", async () => {
  const result = setup();
  let finish!: (value: SSHEnrollment) => void;
  vi.mocked(result.manager.enroll).mockReturnValue(
    new Promise((resolve) => {
      finish = resolve;
    }),
  );
  fireEvent.click(screen.getByRole("button", { name: installLabel }));
  fireEvent.change(screen.getByLabelText(passwordLabel), {
    target: { value: "fixture-only-secret" },
  });
  fireEvent.click(screen.getByRole("button", { name: "settings:sshManager.enrollment.install" }));
  result.section("overview");
  await act(async () =>
    finish({
      request_id: "late",
      connection_id: "target",
      stamp: "stamp",
      key_fingerprint: "key-pin",
      created_at: "2026-10-26T00:00:00Z",
      state: "verified",
    }),
  );
  result.section("access");
  expect(screen.queryByLabelText(passwordLabel)).toBeNull();
});
