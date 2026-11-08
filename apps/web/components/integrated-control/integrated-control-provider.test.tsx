import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { IntegratedControlProvider, useIntegratedControl } from "./integrated-control-provider";
const scope = vi.hoisted(() => ({ id: "owner-one" }));
vi.mock("@/hooks/domains/features/use-feature", () => ({ useFeature: () => true }));
vi.mock("@/lib/routing/client-router", () => ({ usePathname: () => "/control/history" }));
vi.mock("@/lib/state/shared-resource-reads", () => ({ stateReadScopeIdentity: () => scope.id }));
vi.mock("@/components/state-provider", () => ({
  useAppStore: (select: (state: unknown) => unknown) => select({ auth: { mode: "disabled" } }),
}));
vi.mock("@/hooks/domains/coordinator/use-control-aggregate", () => ({
  useControlAggregate: () => ({ status: "loading" }),
}));
vi.mock("@/hooks/domains/coordinator/use-integrated-control-data", () => ({
  useIntegratedControlData: () => ({ loading: false, error: false, refresh: vi.fn() }),
}));
function Selection() {
  const control = useIntegratedControl()!;
  return (
    <button onClick={() => control.selectSession("task", "exact-session")}>
      {control.selectedSessions.task ?? "none"}
    </button>
  );
}
afterEach(cleanup);
describe("control session selection scope", () => {
  it("preserves selection on same-owner refresh and discards it synchronously on scope change", () => {
    scope.id = "owner-one";
    const view = render(
      <IntegratedControlProvider>
        <Selection />
      </IntegratedControlProvider>,
    );
    fireEvent.click(screen.getByRole("button"));
    view.rerender(
      <IntegratedControlProvider>
        <Selection />
      </IntegratedControlProvider>,
    );
    expect(screen.getByRole("button").textContent).toBe("exact-session");
    scope.id = "owner-two";
    view.rerender(
      <IntegratedControlProvider>
        <Selection />
      </IntegratedControlProvider>,
    );
    expect(screen.getByRole("button").textContent).toBe("none");
  });
});
