import type { ComponentType } from "react";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { EntityReference, EntityReferenceSearchGroup } from "@/lib/types/entity-reference";
import * as entityReferenceMenus from "./entity-reference-menu";

afterEach(cleanup);

describe("EntityReferenceMenu", () => {
  it("provides a menu distinct from @ context mentions", () => {
    expect(typeof (entityReferenceMenus as Record<string, unknown>).EntityReferenceMenu).toBe(
      "function",
    );
  });

  it("renders descriptor-driven groups with a generic fallback and 44px touch rows", () => {
    const reference: EntityReference = {
      version: 1,
      ref: "mention:v1:plugin:acme:incident:scope:incident-9",
      provider: "plugin:acme:tracker",
      kind: "incident",
      id: "incident-9",
      key: "INC-9",
      title: "Authentication outage",
      url: "https://tracker.example.test/incidents/9",
      scope: "tracker.example.test/team-a",
    };
    const groups: EntityReferenceSearchGroup[] = [
      {
        source: "plugin:acme:incidents",
        provider: reference.provider,
        kind: reference.kind,
        display_name: "Acme tracker",
        kind_label: "Incident",
        status: "ok",
        results: [reference],
      },
    ];
    const onSelect = vi.fn();
    const Menu = entityReferenceMenus.EntityReferenceMenu as unknown as ComponentType<{
      isOpen: boolean;
      clientRect: () => DOMRect;
      groups: EntityReferenceSearchGroup[];
      query: string;
      selectedIndex: number;
      isSearching: boolean;
      error: null;
      onRetry: () => void;
      onSelect: (reference: EntityReference) => void;
      onClose: () => void;
      setSelectedIndex: (index: number) => void;
    }>;

    render(
      <Menu
        isOpen
        clientRect={() => new DOMRect(16, 240, 1, 20)}
        groups={groups}
        query="auth"
        selectedIndex={0}
        isSearching={false}
        error={null}
        onRetry={vi.fn()}
        onSelect={onSelect}
        onClose={vi.fn()}
        setSelectedIndex={vi.fn()}
      />,
    );

    expect(screen.getByText("Acme tracker")).toBeTruthy();
    expect(screen.getByTestId("entity-reference-menu")).toBeTruthy();
    expect(screen.getByText("Incident")).toBeTruthy();
    expect(screen.getByTestId("entity-reference-generic-icon")).toBeTruthy();
    const row = screen.getByRole("option", { name: /#INC-9.*Authentication outage/ });
    expect(row.className).toContain("min-h-11");
    fireEvent.click(row);
    expect(onSelect).toHaveBeenCalledWith(reference);
  });
});
