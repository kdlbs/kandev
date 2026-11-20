import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { NotificationFormatters } from "./notification-formatters";
import { SettingsSaveDeferredError } from "./settings-save-provider";

const mocks = vi.hoisted(() => ({ hook: vi.fn(), contributor: vi.fn() }));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (key: string) => key }) }));
vi.mock("@/components/plugins/plugin-slot", () => ({ PluginSlot: () => null }));
vi.mock("@/hooks/domains/settings/use-notification-formatter", async (original) => ({
  ...(await original<typeof import("@/hooks/domains/settings/use-notification-formatter")>()),
  useNotificationFormatter: mocks.hook,
}));
vi.mock("./settings-save-provider", async (original) => ({
  ...(await original<typeof import("./settings-save-provider")>()),
  useSettingsSaveContributor: mocks.contributor,
}));

const choice = {
  plugin_id: "formatter",
  installation_id: "installation",
  key: "text",
  version: "1",
};
const selected = JSON.stringify([choice.plugin_id, choice.installation_id, choice.key]);
const providers = [{ id: "mail", name: "Mail", type: "apprise", events: [] }];

function state() {
  return {
    settings: { binding: null, formatters: [choice], events: [] },
    selected,
    choice,
    error: false,
    loading: false,
    readActiveBinding: vi.fn().mockResolvedValue(null),
    clear: vi.fn(),
    discard: vi.fn(),
    reload: vi.fn(),
    preview: vi.fn(),
    activate: vi.fn(),
    select: vi.fn(),
  };
}

beforeEach(() => {
  vi.clearAllMocks();
});
afterEach(cleanup);

it("registers a selection-only draft and refuses to imply first activation on Save", async () => {
  const value = state();
  mocks.hook.mockReturnValue(value);
  render(<NotificationFormatters providers={providers} />);
  const contribution = mocks.contributor.mock.lastCall?.[0];
  expect(contribution.isDirty).toBe(true);
  await expect(contribution.save()).rejects.toBeInstanceOf(SettingsSaveDeferredError);
  expect(value.readActiveBinding).toHaveBeenCalledOnce();
  expect(value.activate).not.toHaveBeenCalled();
});

it("exposes Retry when the initial formatter settings request fails", () => {
  const value = { ...state(), error: true };
  mocks.hook.mockReturnValue(value);
  render(<NotificationFormatters providers={providers} />);
  fireEvent.click(screen.getByRole("button", { name: "plugins:retry" }));
  expect(value.reload).toHaveBeenCalledOnce();
});
