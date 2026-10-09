import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

const { listManagedConversationDestinations } = vi.hoisted(() => ({
  listManagedConversationDestinations: vi.fn(),
}));

vi.mock("@/lib/api/domains/plugins-api", () => ({
  listManagedConversationDestinations,
}));

import { ManagedConversationDestinationSelector } from "./managed-conversation-destination-selector";

afterEach(() => {
  cleanup();
  listManagedConversationDestinations.mockReset();
});

describe("ManagedConversationDestinationSelector", () => {
  it("selects an available workspace conversation and carries its current revision", async () => {
    listManagedConversationDestinations.mockResolvedValue([
      {
        plugin_id: "kandev-plugin-coordinator",
        plugin_name: "Coordinator",
        instance_key: "daily-brief",
        revision: 7,
        paused: false,
      },
    ]);
    const onChange = vi.fn();
    render(
      <ManagedConversationDestinationSelector
        workspaceId="workspace-1"
        isDirty={false}
        onChange={onChange}
      />,
    );

    await waitFor(() =>
      expect(listManagedConversationDestinations).toHaveBeenCalledWith("workspace-1"),
    );
    fireEvent.click(screen.getByTestId("managed-conversation-destination-trigger"));
    fireEvent.click(await screen.findByText("Coordinator / daily-brief"));

    expect(onChange).toHaveBeenCalledWith({
      plugin_id: "kandev-plugin-coordinator",
      instance_key: "daily-brief",
      revision: 7,
    });
  });

  it("keeps a missing destination visible and offers repair", async () => {
    listManagedConversationDestinations.mockResolvedValue([]);
    render(
      <ManagedConversationDestinationSelector
        workspaceId="workspace-1"
        value={{ plugin_id: "kandev-plugin-removed", instance_key: "old-instance", revision: 2 }}
        isDirty={false}
        onChange={vi.fn()}
      />,
    );

    expect(await screen.findByTestId("managed-conversation-destination-unavailable")).toBeTruthy();
    expect(screen.getByTestId("managed-conversation-destination-repair")).toBeTruthy();
  });
});
