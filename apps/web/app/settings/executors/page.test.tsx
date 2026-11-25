import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { StateProvider } from "@/components/state-provider";
import ExecutorsHubPage from "./page";

const access = vi.hoisted(() => ({ enabled: true, admin: true }));
vi.mock("@/hooks/domains/features/use-feature", () => ({ useFeature: () => access.enabled }));
vi.mock("@/hooks/domains/auth/use-is-admin", () => ({ useIsAdmin: () => access.admin }));
afterEach(cleanup);

// @covers AC-EXECUTORS-SSH-CONNECTIONS-001.5, AC-EXECUTORS-SSH-CONNECTIONS-004.6
it.each([
  [true, true, true],
  [false, true, false],
  [true, false, false],
])("gates the manager entry with no profiles (feature=%s, admin=%s)", (enabled, admin, visible) => {
  Object.assign(access, { enabled, admin });
  render(
    <StateProvider>
      <ExecutorsHubPage />
    </StateProvider>,
  );
  const link = screen.queryByRole("link", { name: "SSH connections" });
  expect(Boolean(link)).toBe(visible);
  if (link) expect(link.getAttribute("href")).toBe("/settings/executors/ssh-connections");
});
