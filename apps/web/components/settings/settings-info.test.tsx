import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
const mode = vi.hoisted(() => ({ touch: false }));
vi.mock("@/hooks/use-compact-task-chrome", () => ({ useTouchDrawer: () => mode.touch }));
vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (_: string, values?: { setting: string }) => (values ? `About ${values.setting}` : "Close"),
  }),
}));
import { SettingsInfo } from "./settings-info";
afterEach(cleanup);
describe("SettingsInfo", () => {
  it("reveals optional details on keyboard focus", async () => {
    mode.touch = false;
    render(<SettingsInfo label="Limit">Technical details</SettingsInfo>);
    expect(screen.queryByRole("tooltip")).toBeNull();
    fireEvent.focus(screen.getByRole("button", { name: "About Limit" }));
    expect((await screen.findByRole("tooltip")).textContent).toContain("Technical details");
  });
  it("opens a touch dialog without changing its surrounding setting", async () => {
    mode.touch = true;
    render(<SettingsInfo label="Limit">Technical details</SettingsInfo>);
    fireEvent.click(screen.getByRole("button", { name: "About Limit" }));
    expect((await screen.findByRole("dialog")).textContent).toContain("Technical details");
  });
});
