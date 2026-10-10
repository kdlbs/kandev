"use client";

import { useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { IconAlertTriangle, IconChevronDown } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Command, CommandEmpty, CommandItem, CommandList } from "@kandev/ui/command";
import { Drawer, DrawerContent, DrawerHeader, DrawerTitle } from "@kandev/ui/drawer";
import { Popover, PopoverContent, PopoverTrigger } from "@kandev/ui/popover";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { listManagedConversationDestinations } from "@/lib/api/domains/plugins-api";
import type {
  ManagedConversationDestination,
  ManagedConversationDestinationOption,
} from "@/lib/types/automation";

function destinationId(option: Pick<ManagedConversationDestination, "plugin_id" | "instance_key">) {
  return `${option.plugin_id}\u0000${option.instance_key}`;
}

function availabilityMessageKey(reason: string | undefined) {
  if (!reason) return "automations:managedDestinationUnavailable";
  if (reason.includes("approval") || reason.includes("capability")) {
    return "automations:managedDestinationApprovalRequired";
  }
  if (reason === "plugin_inactive" || reason.includes("unavailable")) {
    return "automations:managedDestinationPluginUnavailable";
  }
  return "automations:managedDestinationUnavailable";
}

function optionLabel(option: ManagedConversationDestinationOption) {
  return `${option.plugin_name} / ${option.instance_key}`;
}

function triggerLabel(
  unavailable: boolean,
  selected: ManagedConversationDestinationOption | undefined,
  t: (key: string) => string,
) {
  if (unavailable) return t("automations:managedDestinationUnavailable");
  if (selected) return optionLabel(selected);
  return t("automations:managedDestinationSelect");
}

function destinationOptionStatus(
  option: ManagedConversationDestinationOption,
  t: (key: string) => string,
) {
  if (option.unavailable_reason) {
    return (
      <span className="ml-2 text-xs text-muted-foreground">
        {t(availabilityMessageKey(option.unavailable_reason))}
      </span>
    );
  }
  if (option.paused) {
    return (
      <span className="ml-2 text-xs text-muted-foreground">
        {t("automations:managedDestinationPaused")}
      </span>
    );
  }
  return null;
}

function selectedDestinationNotice({
  unavailable,
  selected,
  hasValue,
  onRepair,
  t,
}: {
  unavailable: boolean;
  selected?: ManagedConversationDestinationOption;
  hasValue: boolean;
  onRepair: () => void;
  t: (key: string) => string;
}) {
  if (unavailable && hasValue) {
    return (
      <p
        className="flex items-start gap-2 text-sm text-destructive"
        role="alert"
        data-testid="managed-conversation-destination-unavailable"
      >
        <IconAlertTriangle className="mt-0.5 size-4 shrink-0" aria-hidden />
        <span>{t(availabilityMessageKey(selected?.unavailable_reason))}</span>
        <button
          type="button"
          className="min-h-11 shrink-0 underline underline-offset-4"
          onClick={onRepair}
          data-testid="managed-conversation-destination-repair"
        >
          {t("automations:managedDestinationRepair")}
        </button>
      </p>
    );
  }
  if (selected?.paused) {
    return (
      <p
        className="text-sm text-muted-foreground"
        data-testid="managed-conversation-destination-paused"
      >
        {t("automations:managedDestinationPausedHelp")}
      </p>
    );
  }
  return null;
}

// eslint-disable-next-line max-lines-per-function -- The selector binds the same destination state to native mobile and desktop pickers.
export function ManagedConversationDestinationSelector({
  workspaceId,
  value,
  onChange,
  isDirty,
}: {
  workspaceId: string;
  value?: ManagedConversationDestination;
  onChange: (value: ManagedConversationDestination) => void;
  isDirty: boolean;
}) {
  const { t } = useTranslation();
  const { isMobile } = useResponsiveBreakpoint();
  const [options, setOptions] = useState<ManagedConversationDestinationOption[]>([]);
  const [open, setOpen] = useState(false);
  const selectedId = value ? destinationId(value) : "";
  const selected = useMemo(
    () => options.find((option) => destinationId(option) === selectedId),
    [options, selectedId],
  );
  const unavailable = Boolean(value && (!selected || selected.unavailable_reason));

  useEffect(() => {
    let active = true;
    listManagedConversationDestinations(workspaceId)
      .then((items) => {
        if (active) setOptions(items);
      })
      .catch(() => {
        if (active) setOptions([]);
      });
    return () => {
      active = false;
    };
  }, [workspaceId]);

  const choose = (option: ManagedConversationDestinationOption) => {
    if (option.unavailable_reason) return;
    onChange({
      plugin_id: option.plugin_id,
      instance_key: option.instance_key,
      revision: option.revision,
    });
    setOpen(false);
  };

  const label = triggerLabel(unavailable, selected, t);

  const optionRows = options.map((option) => (
    <CommandItem
      key={destinationId(option)}
      value={destinationId(option)}
      disabled={Boolean(option.unavailable_reason)}
      onSelect={() => choose(option)}
      className="min-h-11"
    >
      <span className="min-w-0 flex-1 truncate">{optionLabel(option)}</span>
      {destinationOptionStatus(option, t)}
    </CommandItem>
  ));

  const picker = (
    <Command>
      <CommandList className="max-h-[50dvh]">
        <CommandEmpty>{t("automations:managedDestinationEmpty")}</CommandEmpty>
        {optionRows}
      </CommandList>
    </Command>
  );

  const trigger = (
    <Button
      type="button"
      variant="outline"
      id="managed-conversation-destination"
      aria-label={t("automations:managedDestinationSelect")}
      aria-haspopup={isMobile ? "dialog" : "listbox"}
      aria-expanded={open}
      data-testid="managed-conversation-destination-trigger"
      data-settings-dirty={isDirty}
      className="min-h-11 w-full justify-between"
    >
      <span className="truncate">{label}</span>
      <IconChevronDown className="ml-2 size-4 shrink-0" aria-hidden />
    </Button>
  );

  return (
    <div className="space-y-2" data-testid="managed-conversation-destination-selector">
      <label className="text-sm font-medium" htmlFor="managed-conversation-destination">
        {t("automations:managedDestinationLabel")}
      </label>
      {isMobile ? (
        <>
          <Button
            type="button"
            variant="outline"
            id="managed-conversation-destination"
            aria-haspopup="dialog"
            aria-expanded={open}
            data-testid="managed-conversation-destination-trigger"
            data-settings-dirty={isDirty}
            onClick={() => setOpen(true)}
            className="min-h-11 w-full justify-between"
          >
            <span className="truncate">{label}</span>
            <IconChevronDown className="ml-2 size-4 shrink-0" aria-hidden />
          </Button>
          <Drawer open={open} onOpenChange={setOpen}>
            <DrawerContent
              className="max-h-[80dvh]"
              data-testid="managed-conversation-destination-drawer"
            >
              <DrawerHeader className="text-left">
                <DrawerTitle>{t("automations:managedDestinationSelect")}</DrawerTitle>
              </DrawerHeader>
              <div className="min-h-0 overflow-y-auto px-4 pb-[calc(1rem+env(safe-area-inset-bottom))]">
                {picker}
              </div>
            </DrawerContent>
          </Drawer>
        </>
      ) : (
        <Popover open={open} onOpenChange={setOpen}>
          <PopoverTrigger asChild>{trigger}</PopoverTrigger>
          <PopoverContent className="w-[min(28rem,calc(100vw-2rem))] p-0" align="start">
            {picker}
          </PopoverContent>
        </Popover>
      )}
      {selectedDestinationNotice({
        unavailable,
        selected,
        hasValue: Boolean(value),
        onRepair: () => setOpen(true),
        t,
      })}
      {!value && options.length === 0 ? (
        <p className="text-xs text-muted-foreground">{t("automations:managedDestinationEmpty")}</p>
      ) : null}
    </div>
  );
}
