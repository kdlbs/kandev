"use client";

import { useState } from "react";
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

type ModeComboboxProps = {
  value: string;
  onChange: (value: string) => void;
  modes: ModeEntry[];
  currentModeId: string | undefined;
};

/**
 * Searchable session-mode picker. Same Popover+Command pattern as
 * ModelCombobox so descriptions in the dropdown don't leak into the
 * trigger text (which happens with Radix Select).
 */
export function ModeCombobox({ value, onChange, modes, currentModeId }: ModeComboboxProps) {
  const [open, setOpen] = useState(false);
  const selected = value || currentModeId || modes[0]?.id || "";
  const activeMode = modes.find((m) => m.id === selected);
  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          variant="outline"
          role="combobox"
          aria-expanded={open}
          data-testid="profile-mode-select"
          className="w-full justify-between font-normal cursor-pointer"
        >
          <span className="flex items-center gap-2 truncate">
            {activeMode?.name ?? selected}
            {activeMode?.id === currentModeId && (
              <span className="text-muted-foreground">(default)</span>
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
          <CommandInput placeholder="Search modes..." />
          <CommandList
            className="max-h-[min(60vh,24rem)] overflow-y-auto overscroll-contain"
            onWheel={(e) => e.stopPropagation()}
          >
            <CommandEmpty>No mode found.</CommandEmpty>
            <CommandGroup>
              {modes.map((m) => (
                <CommandItem
                  key={m.id}
                  value={`${m.id} ${m.name}`}
                  onSelect={() => {
                    onChange(m.id);
                    setOpen(false);
                  }}
                  data-checked={selected === m.id}
                  className="cursor-pointer"
                >
                  <div className="flex-1 min-w-0">
                    <div className="flex items-center gap-2 truncate">
                      <span className="truncate">{m.name}</span>
                      {m.id === currentModeId && (
                        <span className="text-muted-foreground text-xs">(default)</span>
                      )}
                    </div>
                    {m.description && m.description !== m.name && (
                      <p className="text-xs text-muted-foreground truncate">{m.description}</p>
                    )}
                  </div>
                </CommandItem>
              ))}
            </CommandGroup>
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  );
}
