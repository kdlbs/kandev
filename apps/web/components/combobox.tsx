"use client"

import { memo, useState } from "react"
import { IconCheck, IconChevronDown } from "@tabler/icons-react"

import { cn } from "@/lib/utils"
import { Button } from "@kandev/ui/button"
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from "@kandev/ui/command"
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@kandev/ui/popover"

export type ComboboxOption = {
  value: string
  label: string
  description?: string
  renderLabel?: () => React.ReactNode
}

interface ComboboxProps {
  options: ComboboxOption[]
  value: string
  onValueChange: (value: string) => void
  dropdownLabel?: string
  placeholder?: string
  searchPlaceholder?: string
  emptyMessage?: string
  disabled?: boolean
  className?: string
  triggerClassName?: string
  showSearch?: boolean
}

export const Combobox = memo(function Combobox({
  options,
  value,
  onValueChange,
  dropdownLabel,
  placeholder = "Select option...",
  searchPlaceholder = "Search...",
  emptyMessage = "No option found.",
  disabled = false,
  className,
  triggerClassName,
  showSearch = true,
}: ComboboxProps) {
  const [open, setOpen] = useState(false)

  const selectedOption = options.find((option) => option.value === value)

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          variant="ghost"
          role="combobox"
          aria-expanded={open}
          className={cn("w-full justify-between", !disabled && "cursor-pointer", triggerClassName)}
          disabled={disabled}
        >
          <div className="flex min-w-0 flex-1 items-center">
            {selectedOption?.renderLabel ? (
              selectedOption.renderLabel()
            ) : (
              <span className="truncate">{selectedOption?.label || placeholder}</span>
            )}
          </div>
          <IconChevronDown className="ml-2 h-4 w-4 shrink-0 opacity-50" />
        </Button>
      </PopoverTrigger>
      <PopoverContent className={cn("w-[var(--radix-popover-trigger-width)] min-w-[300px] max-w-none p-0", className)} align="start">
        <Command>
          {dropdownLabel ? (
            <div className="text-muted-foreground px-2 py-1.5 text-xs border-b">
              {dropdownLabel}
            </div>
          ) : null}
          {showSearch && <CommandInput placeholder={searchPlaceholder} className="h-9" />}
          <CommandList>
            <CommandEmpty>{emptyMessage}</CommandEmpty>
            <CommandGroup>
              {options.map((option) => (
                <CommandItem
                  key={option.value}
                  value={option.value}
                  onSelect={() => {
                    onValueChange(option.value === value ? "" : option.value)
                    setOpen(false)
                  }}
                  className="relative pr-7"
                >
                  <div className="flex min-w-0 flex-1 items-center">
                    {option.renderLabel ? option.renderLabel() : option.label}
                  </div>
                  <IconCheck
                    className={cn(
                      "absolute right-2 h-4 w-4",
                      value === option.value ? "opacity-100" : "opacity-0"
                    )}
                  />
                </CommandItem>
              ))}
            </CommandGroup>
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  )
});
