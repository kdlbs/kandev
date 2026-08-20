import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { i18n } from "@/lib/i18n";
import { RichOutputMotionSettingsCard } from "./rich-output-motion-settings-card";

afterEach(async () => {
  cleanup();
  await i18n.changeLanguage("en");
});

describe("RichOutputMotionSettingsCard", () => {
  it("renders a translated, controlled, touch-sized device preference", async () => {
    await i18n.changeLanguage("pseudo");
    const onChange = vi.fn();
    render(<RichOutputMotionSettingsCard enabled isDirty onChange={onChange} />);

    const toggle = screen.getByRole("switch", { name: "Àńĩḿàţē ŕĩćĥ-ōũţƥũţ ćĥàŕţś" });
    expect(toggle.getAttribute("data-state")).toBe("checked");
    expect(toggle.getAttribute("data-settings-dirty")).toBe("true");
    expect(screen.getByTestId("rich-output-motion-settings-card").dataset.settingsDirty).toBe(
      "true",
    );
    expect(screen.getByTestId("rich-output-motion-toggle-row").className).toContain("min-h-11");
    expect(toggle.className).toContain("data-[size=default]:h-11");
    expect(toggle.className).toContain("data-[size=default]:w-11");
    expect(toggle.className).toContain("data-[state=checked]:before:bg-primary");
    expect(
      screen.getByText(
        "Àńĩḿàţē ĺĩńē àńď ƀàŕ ćĥàŕţś ŵĥēń àĝēńţś ƥŕēśēńţ ţĥēḿ. Ţũŕń ţĥĩś ōƒƒ ţō ŕēďũćē ƀŕōŵśēŕ ŵōŕķ; ćĥàŕţ ďàţà àńď ćōńţŕōĺś śţàŷ àvàĩĺàƀĺē. Ŷōũŕ ōƥēŕàţĩńĝ śŷśţēḿ'ś ŕēďũćēď-ḿōţĩōń śēţţĩńĝ àĺŵàŷś ţàķēś ƥŕĩōŕĩţŷ. Śàvēď ōń ţĥĩś ďēvĩćē.",
      ),
    ).toBeTruthy();

    fireEvent.click(toggle);
    expect(onChange).toHaveBeenCalledWith(false);
    expect(onChange).toHaveBeenCalledOnce();
  });
});
