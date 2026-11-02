"use client";

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { IconSelector } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from "@kandev/ui/command";
import { Popover, PopoverContent, PopoverTrigger } from "@kandev/ui/popover";
import type { ModeEntry } from "@/lib/types/http";
import { settingsControlClassName } from "@/components/settings/settings-control";

type ModeComboboxProps = {
  value: string;
  onChange: (value: string) => void;
  modes: ModeEntry[];
  currentModeId: string | undefined;
  disabled?: boolean;
};

/** True when the id is set but absent from the advertised mode list. */
function isCustomMode(id: string, modes: ModeEntry[]): boolean {
  return id.length > 0 && !modes.some((m) => m.id === id);
}

/** The typed filter when it names a new id, or "" when it matches nothing selectable. */
function customEntryId(filterText: string, storedCustom: string, modes: ModeEntry[]): string {
  const trimmed = filterText.trim();
  if (trimmed.length === 0 || trimmed === storedCustom) {
    return "";
  }
  return modes.some((m) => m.id === trimmed) ? "" : trimmed;
}

type ModeRowProps = {
  onSelect: (id: string) => void;
};

function StoredCustomModeRow({ id, onSelect }: ModeRowProps & { id: string }) {
  const { t } = useTranslation();
  return (
    <CommandItem
      value={id}
      data-testid="profile-mode-custom-row"
      onSelect={() => onSelect(id)}
      data-checked
      className="cursor-pointer"
    >
      <div className="flex-1 min-w-0">
        <div className="flex items-center gap-2 truncate">
          <span className="truncate">{id}</span>
          <span className="text-muted-foreground text-xs">{t("agents:modeSourceCustom")}</span>
        </div>
        <p className="text-xs text-muted-foreground truncate">
          {t("agents:customModeDescription")}
        </p>
      </div>
    </CommandItem>
  );
}

function CustomModeEntryRow({ id, onSelect }: ModeRowProps & { id: string }) {
  const { t } = useTranslation();
  return (
    <CommandItem
      value={id}
      data-testid="profile-mode-custom-entry"
      onSelect={() => onSelect(id)}
      className="cursor-pointer"
    >
      <div className="min-w-0 flex-1 truncate">{t("agents:useCustomModeId", { mode: id })}</div>
    </CommandItem>
  );
}

function AdvertisedModeRow({
  mode,
  isDefault,
  isSelected,
  onSelect,
}: ModeRowProps & {
  mode: ModeEntry;
  isDefault: boolean;
  isSelected: boolean;
}) {
  const { t } = useTranslation();
  return (
    <CommandItem
      value={`${mode.id} ${mode.name}`}
      onSelect={() => onSelect(mode.id)}
      data-checked={isSelected}
      className="cursor-pointer"
    >
      <div className="flex-1 min-w-0">
        <div className="flex items-center gap-2 truncate">
          <span className="truncate">{mode.name}</span>
          {isDefault && (
            <span className="text-muted-foreground text-xs">{t("agents:defaultSuffix")}</span>
          )}
        </div>
        {mode.description && mode.description !== mode.name && (
          <p className="text-xs text-muted-foreground truncate">{mode.description}</p>
        )}
      </div>
    </CommandItem>
  );
}

/**
 * Searchable session-mode picker. Same Popover+Command pattern as
 * ModelCombobox so descriptions in the dropdown don't leak into the
 * trigger text (which happens with Radix Select).
 *
 * Modes can be repository-local (for example an agent defined in a
 * project checkout) and never appear in the advertised list, so stored
 * and typed ids without a match stay selectable instead of dead-ending.
 * The typed-entry group renders first because cmdk activates the first
 * DOM item, so Enter must land on the entry the user typed, not on a
 * coincidental advertised match below it.
 */
export function ModeCombobox({
  value,
  onChange,
  modes,
  currentModeId,
  disabled = false,
}: ModeComboboxProps) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const [filterText, setFilterText] = useState("");
  const selected = value || currentModeId || modes[0]?.id || "";
  const activeMode = modes.find((m) => m.id === selected);
  const storedCustom = isCustomMode(selected, modes) ? selected : "";
  const customEntry = customEntryId(filterText, storedCustom, modes);
  const close = () => {
    setOpen(false);
    setFilterText("");
  };
  const handleOpenChange = (next: boolean) => {
    if (next) {
      setOpen(true);
    } else {
      close();
    }
  };
  const select = (id: string) => {
    onChange(id);
    close();
  };
  return (
    <Popover open={open} onOpenChange={handleOpenChange}>
      <PopoverTrigger asChild>
        <Button
          variant="outline"
          role="combobox"
          aria-expanded={open}
          data-testid="profile-mode-select"
          disabled={disabled}
          className={settingsControlClassName("w-full justify-between font-normal cursor-pointer")}
        >
          <span className="flex items-center gap-2 truncate">
            {activeMode?.name ?? selected}
            {activeMode?.id === currentModeId && (
              <span className="text-muted-foreground">{t("agents:defaultSuffix")}</span>
            )}
          </span>
          <IconSelector className="ml-2 h-4 w-4 shrink-0 opacity-50" />
        </Button>
      </PopoverTrigger>
      <PopoverContent
        className="min-w-[--radix-popover-trigger-width] w-[min(32rem,calc(100vw-2rem))] p-0"
        align="start"
        onWheel={(e) => e.stopPropagation()}
      >
        <Command>
          <CommandInput
            placeholder={t("agents:searchModes")}
            value={filterText}
            onValueChange={setFilterText}
          />
          <CommandList
            className="max-h-[min(60vh,24rem)] overflow-y-auto overscroll-contain"
            onWheel={(e) => e.stopPropagation()}
          >
            <CommandEmpty>{t("agents:noModeFound")}</CommandEmpty>
            {customEntry && (
              <CommandGroup>
                <CustomModeEntryRow id={customEntry} onSelect={select} />
              </CommandGroup>
            )}
            {storedCustom && (
              <CommandGroup>
                <StoredCustomModeRow id={storedCustom} onSelect={select} />
              </CommandGroup>
            )}
            {modes.length > 0 && (
              <CommandGroup>
                {modes.map((m) => (
                  <AdvertisedModeRow
                    key={m.id}
                    mode={m}
                    isDefault={m.id === currentModeId}
                    isSelected={selected === m.id}
                    onSelect={select}
                  />
                ))}
              </CommandGroup>
            )}
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  );
}
