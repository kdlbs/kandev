import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import {
  MobileNavigationGuardProvider,
  useMobileNavigationGuard,
  useRegisterMobileNavigationGuard,
} from "./mobile-navigation-guard";

beforeEach(cleanup);

function RequestButton({ action = () => undefined }: { action?: () => void }) {
  const request = useMobileNavigationGuard();
  return <button onClick={() => request(action)}>Request navigation</button>;
}

function GuardRegistration({ onRequest }: { onRequest: (action: () => void) => void }) {
  useRegisterMobileNavigationGuard(onRequest);
  return null;
}

describe("MobileNavigationGuardProvider", () => {
  it("runs an action directly when no layout guard is registered", () => {
    const action = vi.fn();
    function RequestingButton() {
      const request = useMobileNavigationGuard();
      return <button onClick={() => request(action)}>Request navigation</button>;
    }

    render(
      <MobileNavigationGuardProvider>
        <RequestingButton />
      </MobileNavigationGuardProvider>,
    );

    fireEvent.click(screen.getByRole("button", { name: "Request navigation" }));
    expect(action).toHaveBeenCalledOnce();
  });

  it("routes picker actions through the registered layout guard and unregisters on cleanup", () => {
    const guard = vi.fn((action: () => void) => action());
    const action = vi.fn();
    const view = render(
      <MobileNavigationGuardProvider>
        <GuardRegistration onRequest={guard} />
        <RequestButton action={action} />
      </MobileNavigationGuardProvider>,
    );

    fireEvent.click(screen.getByRole("button", { name: "Request navigation" }));
    expect(guard).toHaveBeenCalledOnce();
    expect(action).toHaveBeenCalledOnce();

    view.unmount();
  });
});
