import { useId } from "react";
import { useTranslation } from "react-i18next";
import type { NotificationFormatterSlotProps } from "@kdlbs/plugin-sdk";
import { PluginSlot } from "@/components/plugins/plugin-slot";
import {
  formatterChoiceId,
  useNotificationFormatter,
} from "@/hooks/domains/settings/use-notification-formatter";
import { SettingsGroup } from "./settings-group";
import { useSettingsSaveContributor } from "./settings-save-provider";

type FormatterProvider = NotificationFormatterSlotProps["provider"];

function FormatterOptions({
  state,
  unavailable,
}: {
  state: ReturnType<typeof useNotificationFormatter>;
  unavailable: boolean;
}) {
  const { t } = useTranslation();
  return (
    <>
      <option value="">{t("settings:notificationFormatterDefault")}</option>
      {unavailable && (
        <option value={state.selected} disabled>
          {t("settings:notificationFormatterUnavailable")}
        </option>
      )}
      {state.settings?.formatters.map((choice) => (
        <option key={formatterChoiceId(choice)} value={formatterChoiceId(choice)}>
          {choice.plugin_id} / {choice.key} ({choice.version})
        </option>
      ))}
    </>
  );
}

function formatterSlotProps(
  provider: FormatterProvider,
  state: ReturnType<typeof useNotificationFormatter>,
): NotificationFormatterSlotProps | null {
  if (!state.choice) return null;
  return {
    provider: {
      id: provider.id,
      name: provider.name,
      type: provider.type,
      events: provider.events,
    },
    events: state.settings?.events ?? [],
    formatter: state.choice,
    binding: state.settings?.binding ?? null,
    activateConfiguration: state.activate,
    reload: () => state.reload(),
    previewConfiguration: state.preview,
    readActiveBinding: state.readActiveBinding,
  };
}

function NotificationFormatterProvider({ provider }: { provider: FormatterProvider }) {
  const { t } = useTranslation();
  const id = useId();
  const state = useNotificationFormatter(provider.id);
  const binding = state.settings?.binding ?? null;
  const clearing = state.selected === "" && Boolean(binding?.plugin_id);
  useSettingsSaveContributor({
    id: `notification-formatter:${provider.id}`,
    order: 30,
    revision: JSON.stringify([state.selected, binding?.revision]),
    isDirty: clearing,
    canSave: !state.loading && !state.error,
    save: state.clear,
    discard: state.discard,
  });
  const slotProps = formatterSlotProps(provider, state);
  const unavailable = !state.choice && state.selected !== "";
  return (
    <section className="min-w-0 space-y-3" data-testid={`notification-formatter-${provider.id}`}>
      <label htmlFor={id} className="block text-sm font-medium">
        {provider.name}
      </label>
      <select
        id={id}
        aria-label={t("settings:notificationFormatterLabel")}
        className="min-h-7 w-full max-w-full cursor-pointer rounded-md border bg-background px-2 text-sm [@media(pointer:coarse)]:min-h-11"
        value={state.selected}
        disabled={state.loading || state.error}
        onChange={(event) => state.select(event.target.value)}
      >
        <FormatterOptions state={state} unavailable={unavailable} />
      </select>
      {state.loading && <p role="status">{t("common:loading")}</p>}
      {(state.error || unavailable) && (
        <p role="status">{t("settings:notificationFormatterUnavailable")}</p>
      )}
      {slotProps && (
        <PluginSlot
          name="notification-formatter"
          ownerPluginId={slotProps.formatter.plugin_id}
          slotProps={slotProps}
          key={`${provider.id}:${state.selected}`}
        />
      )}
      {clearing && <p role="status">{t("settings:notificationFormatterDefault")}</p>}
    </section>
  );
}

export function NotificationFormatters({
  providers,
}: {
  providers: readonly (Omit<FormatterProvider, "events"> & { events?: readonly string[] })[];
}) {
  const { t } = useTranslation();
  if (!providers.length) return null;
  return (
    <SettingsGroup
      title={t("settings:notificationFormatterLabel")}
      description={t("settings:notificationFormatterDescription")}
    >
      <div className="space-y-5">
        {providers.map((provider) => (
          <NotificationFormatterProvider
            key={provider.id}
            provider={{
              id: provider.id,
              name: provider.name,
              type: provider.type,
              events: provider.events ?? [],
            }}
          />
        ))}
      </div>
    </SettingsGroup>
  );
}
