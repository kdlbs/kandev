import { useId } from "react";
import { useTranslation } from "react-i18next";
import type { NotificationFormatterSlotProps } from "@kdlbs/plugin-sdk";
import { Button } from "@kandev/ui/button";
import { PluginSlot } from "@/components/plugins/plugin-slot";
import {
  formatterChoiceId,
  formatterBindingId,
  useNotificationFormatter,
} from "@/hooks/domains/settings/use-notification-formatter";
import { SettingsGroup } from "./settings-group";
import { SettingsSaveDeferredError, useSettingsSaveContributor } from "./settings-save-provider";

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
    selectionChanged: state.selected !== formatterBindingId(state.settings?.binding),
    activateConfiguration: state.activate,
    reload: () => state.reload(),
    previewConfiguration: state.preview,
    readActiveBinding: state.readActiveBinding,
  };
}

async function saveFormatterSelection(
  state: ReturnType<typeof useNotificationFormatter>,
  clearing: boolean,
) {
  if (clearing) return state.clear();
  const active = await state.readActiveBinding();
  if (formatterBindingId(active) !== state.selected) throw new SettingsSaveDeferredError();
}

function FormatterStatus({
  state,
  unavailable,
}: {
  state: ReturnType<typeof useNotificationFormatter>;
  unavailable: boolean;
}) {
  const { t } = useTranslation();
  return (
    <>
      {state.loading && <p role="status">{t("common:loading")}</p>}
      {(state.error || unavailable) && (
        <p role="status">{t("settings:notificationFormatterUnavailable")}</p>
      )}
      {state.error && (
        <Button
          type="button"
          variant="outline"
          className="cursor-pointer [@media(pointer:coarse)]:min-h-11"
          onClick={() => void state.reload()}
        >
          {t("plugins:retry")}
        </Button>
      )}
    </>
  );
}

function NotificationFormatterProvider({ provider }: { provider: FormatterProvider }) {
  const { t } = useTranslation();
  const id = useId();
  const state = useNotificationFormatter(provider.id);
  const binding = state.settings?.binding ?? null;
  const clearing = state.selected === "" && Boolean(binding?.plugin_id);
  const selectionChanged = state.selected !== formatterBindingId(binding);
  useSettingsSaveContributor({
    id: `notification-formatter:${provider.id}`,
    order: 50,
    revision: JSON.stringify([state.selected, binding?.revision]),
    isDirty: selectionChanged,
    canSave: !state.loading && !state.error && (state.selected === "" || Boolean(state.choice)),
    save: () => saveFormatterSelection(state, clearing),
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
      <FormatterStatus state={state} unavailable={unavailable} />
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
