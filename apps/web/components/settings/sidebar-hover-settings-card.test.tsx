import { useState } from "react";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { defaultState } from "@/lib/state/default-state";
import { createAppearanceSavedState } from "./appearance-settings-state";
import { SidebarHoverSettingsCard } from "./sidebar-hover-settings-card";

afterEach(cleanup);

function Editor() {
  const saved = createAppearanceSavedState("dark", "flat", true, defaultState.userSettings);
  const [draft, setDraft] = useState(saved);
  return (
    <SidebarHoverSettingsCard
      draft={draft}
      saved={saved}
      updateDraft={(patch) => setDraft((current) => ({ ...current, ...patch }))}
    />
  );
}

it("retains the last valid delay when disabling an invalid draft", () => {
  render(<Editor />);
  const delay = screen.getByRole("spinbutton", { name: "Hover delay (ms)" });
  const toggle = screen.getByRole("switch", { name: "Show sidebar on hover" });
  fireEvent.change(delay, { target: { value: "1250" } });
  fireEvent.change(delay, { target: { value: "" } });
  expect(delay.getAttribute("aria-invalid")).toBe("true");
  fireEvent.click(toggle);
  expect((delay as HTMLInputElement).disabled).toBe(true);
  expect((delay as HTMLInputElement).value).toBe("1250");
  fireEvent.click(toggle);
  expect((delay as HTMLInputElement).disabled).toBe(false);
  expect((delay as HTMLInputElement).value).toBe("1250");
});
