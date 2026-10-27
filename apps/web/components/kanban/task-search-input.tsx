"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { Input } from "@kandev/ui/input";
import { IconSearch, IconX, IconLoader2 } from "@tabler/icons-react";
import { cn } from "@/lib/utils";
import { useTranslation } from "react-i18next";

interface TaskSearchInputProps {
  value: string;
  onChange: (value: string) => void;
  placeholder?: string;
  debounceMs?: number;
  isLoading?: boolean;
  className?: string;
  autoFocus?: boolean;
  inputTestId?: string;
  clearButtonLabel?: string;
  clearButtonClassName?: string;
}

export function TaskSearchInput({
  value,
  onChange,
  placeholder,
  debounceMs = 300,
  isLoading = false,
  className,
  autoFocus = false,
  inputTestId,
  clearButtonLabel,
  clearButtonClassName,
}: TaskSearchInputProps) {
  const { t } = useTranslation();
  const searchPlaceholder = placeholder ?? t("kanban:searchTasksPlaceholder");
  const [localValue, setLocalValue] = useState(value);
  const timeoutRef = useRef<NodeJS.Timeout | null>(null);
  const inputRef = useRef<HTMLInputElement | null>(null);

  // Sync local value when external value changes
  useEffect(() => {
    if (timeoutRef.current) {
      clearTimeout(timeoutRef.current);
      timeoutRef.current = null;
    }
    setLocalValue(value);
  }, [value]);

  // Focus on mount when requested (e.g. when the mobile search bar expands)
  useEffect(() => {
    if (autoFocus) {
      inputRef.current?.focus();
    }
  }, [autoFocus]);

  const handleChange = useCallback(
    (e: React.ChangeEvent<HTMLInputElement>) => {
      const newValue = e.target.value;
      setLocalValue(newValue);

      // Clear existing timeout
      if (timeoutRef.current) {
        clearTimeout(timeoutRef.current);
      }

      // Debounce the onChange callback
      timeoutRef.current = setTimeout(() => {
        onChange(newValue);
      }, debounceMs);
    },
    [onChange, debounceMs],
  );

  const handleClear = useCallback(() => {
    setLocalValue("");
    if (timeoutRef.current) {
      clearTimeout(timeoutRef.current);
    }
    onChange("");
  }, [onChange]);

  // Cleanup timeout on unmount
  useEffect(() => {
    return () => {
      if (timeoutRef.current) {
        clearTimeout(timeoutRef.current);
      }
    };
  }, []);

  return (
    <div className={cn("relative", className)}>
      {isLoading ? (
        <IconLoader2 className="absolute left-2.5 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground pointer-events-none animate-spin" />
      ) : (
        <IconSearch className="absolute left-2.5 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground pointer-events-none" />
      )}
      <Input
        ref={inputRef}
        type="text"
        value={localValue}
        onChange={handleChange}
        placeholder={searchPlaceholder}
        aria-label={searchPlaceholder}
        data-testid={inputTestId}
        className="pl-8 pr-8 w-full border border-border text-[16px] md:text-[16px] lg:text-xs/relaxed"
      />
      {localValue && !isLoading && (
        <button
          type="button"
          onClick={handleClear}
          aria-label={clearButtonLabel ?? t("tasks:clearSearch")}
          className={cn(
            "absolute right-2 top-1/2 inline-flex -translate-y-1/2 cursor-pointer items-center justify-center text-muted-foreground hover:text-foreground",
            clearButtonClassName,
          )}
        >
          <IconX className="h-4 w-4" />
        </button>
      )}
    </div>
  );
}
