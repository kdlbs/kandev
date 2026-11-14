import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { pluginRegistry } from "@/lib/plugins/registry";
import { getNotificationFormatter } from "@/lib/api/domains/notification-formatters-api";
import { NotificationFormatters } from "./notification-formatters";
import { SettingsSaveProvider } from "./settings-save-provider";

vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (key: string) => key }) }));
vi.mock("@/lib/api/domains/notification-formatters-api", () => ({
  getNotificationFormatter: vi.fn(),
  selectNotificationFormatter: vi.fn(),
}));
afterEach(() => {
  cleanup();
  pluginRegistry.unregisterPlugin("formatter-a");
  pluginRegistry.unregisterPlugin("formatter-b");
});

it("mounts only the chosen owner and passes no provider credentials to its editor", async () => {
  const choices = ["a", "b"].map((key) => ({
    plugin_id: `formatter-${key}`,
    installation_id: `install-${key}`,
    key: "text",
    version: "1.0.0",
  }));
  vi.mocked(getNotificationFormatter).mockResolvedValue({
    binding: null,
    formatters: choices,
    events: ["session.failed"],
  });
  pluginRegistry
    .forPlugin("formatter-a")
    .registerComponent("notification-formatter", () => <div data-testid="editor-a" />);
  pluginRegistry
    .forPlugin("formatter-b")
    .registerComponent("notification-formatter", () => <div data-testid="editor-b" />);
  render(
    <SettingsSaveProvider>
      <NotificationFormatters
        providers={[{ id: "mail", name: "Mail", type: "apprise", events: [] }]}
      />
    </SettingsSaveProvider>,
  );
  const select = await screen.findByRole("combobox");
  await waitFor(() => expect(select.querySelectorAll("option")).toHaveLength(3));
  fireEvent.change(select, {
    target: { value: JSON.stringify(["formatter-b", "install-b", "text"]) },
  });
  expect(await screen.findByTestId("editor-b")).toBeTruthy();
  expect(screen.queryByTestId("editor-a")).toBeNull();
});
