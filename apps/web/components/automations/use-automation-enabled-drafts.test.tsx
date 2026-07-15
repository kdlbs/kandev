import { act, fireEvent, render, renderHook, screen, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { SettingsSaveProvider } from "@/components/settings/settings-save-provider";
import type { Automation } from "@/lib/types/automation";
import { useAutomationEnabledDrafts } from "./use-automation-enabled-drafts";

const automation = { id: "automation-1", enabled: true } as Automation;

function Harness({ disable }: { disable: (id: string) => Promise<unknown> }) {
  const draft = useAutomationEnabledDrafts({
    automations: [automation],
    enable: vi.fn(),
    disable,
  });
  return (
    <button onClick={() => draft.setEnabled(automation.id, !draft.automations[0].enabled)}>
      Toggle
    </button>
  );
}

describe("useAutomationEnabledDrafts", () => {
  it("persists list toggles only through the shared save action", async () => {
    const disable = vi.fn().mockResolvedValue(undefined);
    render(
      <SettingsSaveProvider>
        <Harness disable={disable} />
      </SettingsSaveProvider>,
    );

    fireEvent.click(screen.getByRole("button", { name: "Toggle" }));
    expect(disable).not.toHaveBeenCalled();

    fireEvent.click(screen.getByRole("button", { name: "Save changes" }));
    await waitFor(() => expect(disable).toHaveBeenCalledWith(automation.id));
  });

  it("does not retry a toggle that succeeded before another toggle failed", async () => {
    const enable = vi.fn().mockRejectedValue(new Error("temporary failure"));
    const disable = vi.fn().mockResolvedValue(undefined);
    const second = { ...automation, id: "automation-2", enabled: false };
    const { result } = renderHook(
      () => useAutomationEnabledDrafts({ automations: [automation, second], enable, disable }),
      { wrapper: SettingsSaveProvider },
    );

    act(() => {
      result.current.setEnabled(automation.id, false);
      result.current.setEnabled(second.id, true);
    });
    fireEvent.click(screen.getByRole("button", { name: "Save changes" }));
    await waitFor(() => expect(disable).toHaveBeenCalledTimes(1));
    expect(enable).toHaveBeenCalledTimes(1);

    fireEvent.click(screen.getByRole("button", { name: "Retry save" }));
    await waitFor(() => expect(enable).toHaveBeenCalledTimes(2));
    expect(disable).toHaveBeenCalledTimes(1);
  });
});
