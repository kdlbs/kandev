import { cleanup, render, waitFor } from "@testing-library/react";
import { createRef } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LOCATION_CHANGE_EVENT } from "@/lib/routing/navigation-event";
import { emitSettingsTargetRequest } from "@/lib/settings-discovery/target";
import { SettingsTargetProvider } from "./settings-target-provider";
import { SettingsTarget } from "./settings-target";

afterEach(() => {
  cleanup();
  window.history.replaceState({}, "", "/");
  vi.restoreAllMocks();
});

describe("SettingsTargetProvider", () => {
  it("preserves a caller-supplied ref while registering the discovery target", () => {
    const callerRef = createRef<HTMLDivElement>();

    const view = render(
      <SettingsTargetProvider>
        <SettingsTarget ref={callerRef} targetId="font-size">
          Control
        </SettingsTarget>
      </SettingsTargetProvider>,
    );

    expect(callerRef.current).toBe(view.getByText("Control"));
  });

  it("reveals an initial target after descendants register", async () => {
    window.history.replaceState({}, "", "/settings/general/terminal#font-size");
    const reveal = vi.fn();

    render(
      <SettingsTargetProvider revealTarget={reveal}>
        <SettingsTarget targetId="font-size">Control</SettingsTarget>
      </SettingsTargetProvider>,
    );

    await waitFor(() => expect(reveal).toHaveBeenCalledTimes(1));
    expect((reveal.mock.calls[0][0] as HTMLElement).getAttribute("data-settings-target")).toBe(
      "font-size",
    );
  });

  it("retains a route request until asynchronous content registers", async () => {
    const reveal = vi.fn();
    const view = render(
      <SettingsTargetProvider revealTarget={reveal}>
        <span>Loading</span>
      </SettingsTargetProvider>,
    );
    window.history.replaceState({}, "", "/settings/general/terminal#late-control");
    window.dispatchEvent(new Event(LOCATION_CHANGE_EVENT));

    view.rerender(
      <SettingsTargetProvider revealTarget={reveal}>
        <SettingsTarget targetId="late-control">Loaded</SettingsTarget>
      </SettingsTargetProvider>,
    );

    await waitFor(() => expect(reveal).toHaveBeenCalledTimes(1));
  });

  it("handles repeated explicit requests and browser history", async () => {
    const reveal = vi.fn();
    render(
      <SettingsTargetProvider revealTarget={reveal}>
        <SettingsTarget targetId="font-size">Control</SettingsTarget>
      </SettingsTargetProvider>,
    );

    emitSettingsTargetRequest("font-size");
    emitSettingsTargetRequest("font-size");
    window.history.replaceState({}, "", "/settings/general/terminal#font-size");
    window.dispatchEvent(new PopStateEvent("popstate"));

    await waitFor(() => expect(reveal).toHaveBeenCalledTimes(3));
  });
});
