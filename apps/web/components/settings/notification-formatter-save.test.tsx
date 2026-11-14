import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { useState, type ReactNode } from "react";
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

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string) => key }),
  Trans: ({ children }: { children?: ReactNode }) => children,
}));
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
const providerSettingsId = "provider-settings";
const notificationSoundId = "notification-sound";
const floatingSaveId = "settings-floating-save";

function OtherSettings({ save }: { save: (id: string) => void }) {
  const [saved, setSaved] = useState(false);
  useSettingsSaveContributor({
    id: providerSettingsId,
    revision: 1,
    isDirty: !saved,
    save: () => {
      save(providerSettingsId);
      setSaved(true);
    },
    discard: () => setSaved(true),
  });
  useSettingsSaveContributor({
    id: notificationSoundId,
    revision: 1,
    isDirty: !saved,
    save: () => {
      save(notificationSoundId);
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

function UnavailableTemplateEditor() {
  const [dirty, setDirty] = useState(true);
  useSettingsSaveContributor({
    id: "plugin:formatter:templates:mail",
    order: 40,
    revision: 1,
    isDirty: dirty,
    canSave: true,
    save: async () => {
      throw new Error("template editor is not ready");
    },
    discard: () => setDirty(false),
  });
  return null;
}

it("keeps a failed template editor dirty while saving independent provider and sound settings", async () => {
  const save = vi.fn();
  const result = vi.fn();
  render(
    <SettingsSaveProvider>
      <UnavailableTemplateEditor />
      <OtherSettings save={save} />
      <SaveProbe result={result} />
    </SettingsSaveProvider>,
  );
  await act(async () => fireEvent.click(screen.getByRole("button", { name: "Save all" })));
  expect(save.mock.calls).toEqual([[providerSettingsId], [notificationSoundId]]);
  expect(result).toHaveBeenCalledWith({
    canLeave: false,
    failedIds: new Set(["plugin:formatter:templates:mail"]),
  });
  expect(screen.getByTestId(floatingSaveId).dataset.dirtyContributors).toBe(
    "plugin:formatter:templates:mail",
  );
  fireEvent.click(screen.getByRole("button", { name: "settings:reset" }));
  await waitFor(() => expect(screen.queryByTestId(floatingSaveId)).toBeNull());
});

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

    expect(save.mock.calls).toEqual([[providerSettingsId], [notificationSoundId]]);
    expect(result).toHaveBeenCalledWith({ canLeave: false, failedIds: new Set() });
    expect(selectNotificationFormatter).not.toHaveBeenCalled();
    expect((select as HTMLSelectElement).value).toBe(selected);
    expect(screen.getByTestId(floatingSaveId).dataset.status).toBe("dirty");
    expect(screen.getByTestId(floatingSaveId).dataset.dirtyContributors).toBe(
      "notification-formatter:mail",
    );

    fireEvent.click(screen.getByRole("button", { name: "settings:reset" }));
    await waitFor(() => expect(screen.queryByTestId(floatingSaveId)).toBeNull());
    expect((select as HTMLSelectElement).value).toBe(
      existing ? JSON.stringify(["previous", "previous-installation", "text"]) : "",
    );
    expect(selectNotificationFormatter).not.toHaveBeenCalled();
  },
);
