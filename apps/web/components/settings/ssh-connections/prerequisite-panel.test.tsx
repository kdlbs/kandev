import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import type { SSHManager } from "@/hooks/domains/settings/use-ssh-connections";
import type { SSHPreflight, SSHRoute } from "@/lib/types/http-ssh-connections";
import { PrerequisitePanel } from "./prerequisite-panel";
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (key: string) => key }) }));
afterEach(cleanup);
const checkLabel = "settings:sshManager.preflight";
const emptyLabel = "settings:sshManager.experience.noSetupIssues";
const route = { destination: { id: "dest", identity_id: "key" }, stamp: "current" } as SSHRoute;
function setup() {
  const preflight = vi.fn().mockResolvedValue({ stamp: "current", checks: [], ready: true });
  const test = vi.fn().mockResolvedValue(undefined);
  const manager = { preflight, test, busy: false } as unknown as SSHManager;
  const onRemedy = vi.fn();
  return {
    preflight,
    test,
    manager,
    onRemedy,
    ...render(<PrerequisitePanel route={route} manager={manager} onRemedy={onRemedy} />),
  };
}
// @covers AC-EXECUTORS-SSH-CONNECTIONS-004.18
it("does not claim live access from an empty local setup report", async () => {
  const { test } = setup();
  fireEvent.click(screen.getByRole("button", { name: checkLabel }));
  await screen.findByText(emptyLabel);
  expect(screen.queryByText("settings:sshManager.ready")).toBeNull();
  expect(test).not.toHaveBeenCalled();
  expect(screen.queryByRole("button", { name: "settings:sshManager.test" })).toBeNull();
});
it("discards a late report from a previous route", async () => {
  const { preflight, manager, onRemedy, rerender } = setup();
  let resolve!: (value: SSHPreflight) => void;
  preflight.mockReturnValue(
    new Promise((done) => {
      resolve = done;
    }),
  );
  fireEvent.click(screen.getByRole("button", { name: checkLabel }));
  rerender(
    <PrerequisitePanel route={{ ...route, stamp: "new" }} manager={manager} onRemedy={onRemedy} />,
  );
  await act(async () => {
    resolve({ stamp: "current", checks: [], ready: true });
  });
  expect(screen.queryByText(emptyLabel)).toBeNull();
});
it("groups issues by hop and dispatches an explicit non-mutating remedy", async () => {
  const { preflight, onRemedy } = setup();
  preflight.mockResolvedValue({
    stamp: "current",
    checks: [
      { code: "ssh_trust_required", hop: "bastion" },
      { code: "ssh_agent_empty", hop: "destination" },
      { code: "untrusted raw output\nsecret", hop: undefined },
    ],
    ready: false,
  });
  fireEvent.click(screen.getByRole("button", { name: checkLabel }));
  await screen.findByText("settings:sshManager.errors.ssh_agent_empty");
  fireEvent.click(
    screen.getByRole("button", { name: "settings:sshManager.experience.remedyAccess" }),
  );
  expect(onRemedy).toHaveBeenCalledWith("access", "bastion");
  expect(screen.queryByText(/untrusted raw output/)).toBeNull();
});
it("clears an earlier report when a new request fails", async () => {
  const { preflight } = setup();
  const check = screen.getByRole("button", { name: checkLabel });
  fireEvent.click(check);
  await screen.findByText(emptyLabel);
  preflight.mockRejectedValue(new Error("private transport content"));
  fireEvent.click(check);
  await screen.findByText("settings:sshManager.experience.failed.preflight");
  expect(screen.queryByText(emptyLabel)).toBeNull();
  expect(screen.queryByText("private transport content")).toBeNull();
});
