import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { ModelFallbackSettingsShell } from "./model-fallback-settings-shell";

afterEach(cleanup);

function renderShell({ autoFallback = false, fallbackModel = "", isDirty = false } = {}) {
  return render(
    <ModelFallbackSettingsShell
      autoFallback={autoFallback}
      fallbackModel={fallbackModel}
      isDirty={isDirty}
      automaticOption={<span>Automatic option</span>}
      explicitOption={<span>Explicit option</span>}
    />,
  );
}

describe("ModelFallbackSettingsShell", () => {
  it("starts collapsed with a state summary", () => {
    renderShell({ fallbackModel: "gpt-5", isDirty: true });

    expect(
      screen.getByTestId("profile-fallback-settings-trigger").getAttribute("aria-expanded"),
    ).toBe("false");
    expect(screen.getByTestId("profile-fallback-settings-summary").textContent).toContain(
      "Explicit fallback: gpt-5",
    );
    expect(
      screen
        .getByTestId("profile-fallback-settings")
        .firstElementChild?.getAttribute("data-settings-dirty"),
    ).toBe("true");
    expect(screen.queryByTestId("profile-fallback-settings-grid")).toBeNull();
  });

  it("reveals both fallback options in the responsive grid", () => {
    renderShell({ autoFallback: true });

    fireEvent.click(screen.getByTestId("profile-fallback-settings-trigger"));

    expect(
      screen.getByTestId("profile-fallback-settings-trigger").getAttribute("aria-expanded"),
    ).toBe("true");
    expect(screen.getByTestId("profile-fallback-settings-grid").className).toContain(
      "md:grid-cols-2",
    );
    expect(screen.getByTestId("profile-auto-fallback-option").textContent).toContain(
      "Automatic option",
    );
    expect(screen.getByTestId("profile-explicit-fallback-option").textContent).toContain(
      "Explicit option",
    );
  });
});
