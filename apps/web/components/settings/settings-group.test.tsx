import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { SettingsGroup, SettingsRow } from "./settings-group";
import { SettingsSaveProvider, useSettingsSaveContributor } from "./settings-save-provider";

afterEach(cleanup);

function DirtyContributor({ id }: { id: string }) {
  useSettingsSaveContributor({
    id,
    revision: 1,
    isDirty: true,
    canSave: true,
    save: async () => undefined,
    discard: () => undefined,
  });
  return null;
}

describe("SettingsGroup structure", () => {
  it("keeps one group card and mounts collapsed content for discovery and saving", () => {
    render(
      <SettingsGroup
        title="Runtime and limits"
        summary="Automatic sessions: 5"
        collapsible
        defaultOpen={false}
        data-testid="runtime-group"
      >
        <SettingsRow
          label="Maximum automatic sessions"
          description="Manual starts can exceed this limit."
          controlId="maximum-sessions"
          control={<input id="maximum-sessions" />}
        />
      </SettingsGroup>,
    );

    const group = screen.getByTestId("runtime-group");
    const details = group.querySelector("details");
    expect(details?.open).toBe(false);
    expect(screen.getByLabelText("Maximum automatic sessions")).toBeTruthy();
    expect(group.querySelectorAll('[data-slot="card"]')).toHaveLength(1);
    expect(group.querySelectorAll("h3")).toHaveLength(1);

    fireEvent.click(group.querySelector("summary")!);
    expect(details?.open).toBe(true);
  });

  it("associates row labels and descriptions with the row control", () => {
    render(
      <SettingsGroup title="Preferences">
        <SettingsRow
          label="Open new tasks automatically"
          description="Open a new task after you create it."
          controlId="auto-open"
          control={<input id="auto-open" aria-describedby="existing-help" />}
          descriptionId="auto-open-help"
        />
      </SettingsGroup>,
    );

    expect(
      screen.getByLabelText("Open new tasks automatically").getAttribute("aria-describedby"),
    ).toBe("existing-help auto-open-help");
    expect(screen.getByText("Open a new task after you create it.")).toBeTruthy();
  });

  it("generates a description association for a production-shaped row", () => {
    const { container } = render(
      <SettingsGroup title="Preferences">
        <SettingsRow
          label="Open new tasks automatically"
          description="Open a new task after you create it."
          controlId="auto-open"
          control={<input id="auto-open" />}
        />
      </SettingsGroup>,
    );

    const control = container.querySelector<HTMLInputElement>("#auto-open")!;
    const descriptionId = control.getAttribute("aria-describedby");
    expect(descriptionId).toMatch(/^settings-row-description-/);
    expect(document.getElementById(descriptionId!)?.textContent).toBe(
      "Open a new task after you create it.",
    );
  });
});

describe("SettingsGroup interactions", () => {
  it("reveals a collapsed group on a new attention state and permits manual collapse", () => {
    const { container, rerender } = render(
      <SettingsGroup title="Runtime" collapsible defaultOpen={false} revealOn={false}>
        <div>Runtime content</div>
      </SettingsGroup>,
    );

    const details = container.querySelector<HTMLDetailsElement>("details")!;
    expect(details.open).toBe(false);

    rerender(
      <SettingsGroup title="Runtime" collapsible defaultOpen={false} revealOn>
        <div>Runtime content</div>
      </SettingsGroup>,
    );
    expect(details.open).toBe(true);

    fireEvent.click(details.querySelector("summary")!);
    expect(details.open).toBe(false);

    rerender(
      <SettingsGroup title="Runtime" collapsible defaultOpen={false} revealOn>
        <div>Runtime content</div>
      </SettingsGroup>,
    );
    expect(details.open).toBe(false);

    rerender(
      <SettingsGroup title="Runtime" collapsible defaultOpen={false} revealOn={false}>
        <div>Runtime content</div>
      </SettingsGroup>,
    );
    rerender(
      <SettingsGroup title="Runtime" collapsible defaultOpen={false} revealOn>
        <div>Runtime content</div>
      </SettingsGroup>,
    );
    expect(details.open).toBe(true);
  });

  it("makes the mobile switch wrapper a real activation target", () => {
    render(
      <SettingsGroup title="Preferences">
        <SettingsRow
          label="Open new tasks automatically"
          controlId="auto-open"
          touchTarget="switch"
          controlWrapperTestId="auto-open-touch-target"
          control={<input id="auto-open" type="checkbox" />}
        />
      </SettingsGroup>,
    );

    fireEvent.click(screen.getByTestId("auto-open-touch-target"));
    expect((screen.getByRole("checkbox") as HTMLInputElement).checked).toBe(true);
  });

  it("includes descendant save contributors in the group dirty state", () => {
    const { container } = render(
      <SettingsSaveProvider>
        <SettingsGroup title="Runtime" isDirty={false}>
          <DirtyContributor id="sleep-inhibition" />
        </SettingsGroup>
      </SettingsSaveProvider>,
    );

    const group = container.querySelector('[data-settings-group="true"]')!;
    expect(group.getAttribute("data-settings-dirty")).toBe("true");
    expect(screen.getByRole("status").textContent).toBe("Unsaved changes");
  });
});

describe("SettingsGroup heading", () => {
  it("keeps title accessories outside the semantic group heading", () => {
    render(
      <SettingsGroup
        title="Repository Scope"
        titleAccessory={<button type="button" aria-label="Explain repository scope" />}
      >
        <div>Content</div>
      </SettingsGroup>,
    );

    expect(screen.getByRole("heading", { name: "Repository Scope", level: 3 })).toBeTruthy();
    expect(screen.getByRole("button", { name: "Explain repository scope" })).toBeTruthy();
  });
});
