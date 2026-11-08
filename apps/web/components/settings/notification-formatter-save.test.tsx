import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { useState } from "react";
import { afterEach, expect, it, vi } from "vitest";
import {
  getNotificationFormatter,
  selectNotificationFormatter,
} from "@/lib/api/domains/notification-formatters-api";
import { NotificationFormatters } from "./notification-formatters";
import {
  SettingsSaveProvider,
  useSettingsSaveContributor,
  useSettingsSaveCoordinator,
} from "./settings-save-provider";

vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (key: string) => key }) }));
vi.mock("@/lib/api/domains/notification-formatters-api", () => ({
  getNotificationFormatter: vi.fn(),
  selectNotificationFormatter: vi.fn(),
}));
afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

const choice = {
  plugin_id: "formatter",
  installation_id: "installation",
  key: "text",
  version: "1",
};
const selected = JSON.stringify([choice.plugin_id, choice.installation_id, choice.key]);
const providers = [{ id: "mail", name: "Mail", type: "apprise", events: [] }];

function OtherSettings({ save }: { save: (id: string) => void }) {
  const [saved, setSaved] = useState(false);
  useSettingsSaveContributor({
    id: "provider-settings",
    revision: 1,
    isDirty: !saved,
    save: () => {
      save("provider-settings");
      setSaved(true);
    },
    discard: () => setSaved(true),
  });
  useSettingsSaveContributor({
    id: "notification-sound",
    revision: 1,
    isDirty: !saved,
    save: () => {
      save("notification-sound");
      setSaved(true);
    },
    discard: () => setSaved(true),
  });
  return null;
}

function SaveProbe({ result }: { result: (value: unknown) => void }) {
  const coordinator = useSettingsSaveCoordinator();
  return (
    <button type="button" onClick={async () => result(await coordinator.saveAll())}>
      Save all
    </button>
  );
}

it.each([false, true])(
  "saves sibling settings while an explicit formatter activation remains pending (existing=%s)",
  async (existing) => {
    vi.mocked(getNotificationFormatter).mockResolvedValue({
      binding: existing
        ? {
            provider_id: "mail",
            provider_type: "apprise",
            plugin_id: "previous",
            installation_id: "previous-installation",
            formatter_key: "text",
            configuration_revision: "configuration-1",
            digest: "digest-1",
            revision: "binding-1",
          }
        : null,
      formatters: [choice],
      events: [],
    });
    const save = vi.fn();
    const result = vi.fn();
    render(
      <SettingsSaveProvider>
        <NotificationFormatters providers={providers} />
        <OtherSettings save={save} />
        <SaveProbe result={result} />
      </SettingsSaveProvider>,
    );
    const select = screen.getByRole("combobox");
    await waitFor(() => expect(select.querySelector(`option[value='${selected}']`)).toBeTruthy());
    fireEvent.change(select, { target: { value: selected } });
    await act(async () => fireEvent.click(screen.getByRole("button", { name: "Save all" })));

    expect(save.mock.calls).toEqual([["provider-settings"], ["notification-sound"]]);
    expect(result).toHaveBeenCalledWith({ canLeave: false, failedIds: new Set() });
    expect(selectNotificationFormatter).not.toHaveBeenCalled();
    expect((select as HTMLSelectElement).value).toBe(selected);
    expect(screen.getByTestId("settings-floating-save").dataset.status).toBe("dirty");
    expect(screen.getByTestId("settings-floating-save").dataset.dirtyContributors).toBe(
      "notification-formatter:mail",
    );

    fireEvent.click(screen.getByRole("button", { name: "settings:reset" }));
    await waitFor(() => expect(screen.queryByTestId("settings-floating-save")).toBeNull());
    expect((select as HTMLSelectElement).value).toBe(
      existing ? JSON.stringify(["previous", "previous-installation", "text"]) : "",
    );
    expect(selectNotificationFormatter).not.toHaveBeenCalled();
  },
);
