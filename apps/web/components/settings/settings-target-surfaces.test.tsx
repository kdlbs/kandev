import { cleanup, render, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { emitSettingsTargetRequest } from "@/lib/settings-discovery/target";
import { SettingsCard } from "./settings-card";
import { SettingsSection } from "./settings-section";
import { SettingsTargetProvider } from "./settings-target-provider";

afterEach(cleanup);

describe("settings target surfaces", () => {
  it("registers a card without adding a layout wrapper", async () => {
    const reveal = vi.fn();
    const { container } = render(
      <SettingsTargetProvider revealTarget={reveal}>
        <SettingsCard discoveryTargetId="card-control">Card</SettingsCard>
      </SettingsTargetProvider>,
    );

    emitSettingsTargetRequest("card-control");

    await waitFor(() => expect(reveal).toHaveBeenCalledTimes(1));
    expect((reveal.mock.calls[0][0] as HTMLElement).getAttribute("data-slot")).toBe("card");
    expect(container.children).toHaveLength(1);
    expect(container.firstElementChild?.getAttribute("data-slot")).toBe("card");
  });

  it("registers a section as a stable target", async () => {
    const reveal = vi.fn();
    render(
      <SettingsTargetProvider revealTarget={reveal}>
        <SettingsSection discoveryTargetId="section-control" title="Section">
          Content
        </SettingsSection>
      </SettingsTargetProvider>,
    );

    emitSettingsTargetRequest("section-control");

    await waitFor(() => expect(reveal).toHaveBeenCalledTimes(1));
    expect((reveal.mock.calls[0][0] as HTMLElement).tagName).toBe("SECTION");
  });
});
