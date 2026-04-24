"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { Input } from "@kandev/ui/input";
import { Button } from "@kandev/ui/button";
import {
  IconSearch,
  IconX,
  IconChevronUp,
  IconChevronDown,
  IconLetterCase,
  IconRegex,
  IconLoader2,
} from "@tabler/icons-react";
import { cn } from "@/lib/utils";

export type MatchInfo = {
  current: number;
  total: number;
};

export type ToggleState = {
  value: boolean;
  onChange: (value: boolean) => void;
};

export type SearchToggles = {
  caseSensitive?: ToggleState;
  regex?: ToggleState;
};

type PanelSearchBarProps = {
  value: string;
  onChange: (value: string) => void;
  onNext: () => void;
  onPrev: () => void;
  onClose: () => void;
  matchInfo?: MatchInfo;
  isLoading?: boolean;
  placeholder?: string;
  toggles?: SearchToggles;
  hasError?: boolean;
  errorText?: string;
  debounceMs?: number;
  className?: string;
};

function ToggleButton({
  state,
  title,
  children,
}: {
  state: ToggleState;
  title: string;
  children: React.ReactNode;
}) {
  return (
    <Button
      type="button"
      variant={state.value ? "secondary" : "ghost"}
      size="icon-sm"
      onClick={() => state.onChange(!state.value)}
      title={title}
      aria-pressed={state.value}
      className="cursor-pointer"
    >
      {children}
    </Button>
  );
}

function MatchCounter({ info }: { info: MatchInfo }) {
  return (
    <span
      className="min-w-[3.5rem] text-center text-[0.6875rem] text-muted-foreground tabular-nums"
      aria-live="polite"
    >
      {info.total === 0 ? "0 / 0" : `${info.current} / ${info.total}`}
    </span>
  );
}

function ActionButtons({
  onNext,
  onPrev,
  onClose,
}: {
  onNext: () => void;
  onPrev: () => void;
  onClose: () => void;
}) {
  return (
    <>
      <Button
        type="button"
        variant="ghost"
        size="icon-sm"
        onClick={onPrev}
        title="Previous (Shift+Enter)"
        className="cursor-pointer"
      >
        <IconChevronUp />
      </Button>
      <Button
        type="button"
        variant="ghost"
        size="icon-sm"
        onClick={onNext}
        title="Next (Enter)"
        className="cursor-pointer"
      >
        <IconChevronDown />
      </Button>
      <Button
        type="button"
        variant="ghost"
        size="icon-sm"
        onClick={onClose}
        title="Close (Esc)"
        className="cursor-pointer"
      >
        <IconX />
      </Button>
    </>
  );
}

function SearchInputField({
  localValue,
  onChange,
  onKeyDown,
  placeholder,
  isLoading,
  hasError,
  inputRef,
}: {
  localValue: string;
  onChange: (e: React.ChangeEvent<HTMLInputElement>) => void;
  onKeyDown: (e: React.KeyboardEvent<HTMLInputElement>) => void;
  placeholder: string;
  isLoading: boolean;
  hasError: boolean;
  inputRef: React.RefObject<HTMLInputElement | null>;
}) {
  return (
    <div className="relative w-56">
      {isLoading ? (
        <IconLoader2 className="absolute left-2 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground pointer-events-none animate-spin" />
      ) : (
        <IconSearch className="absolute left-2 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground pointer-events-none" />
      )}
      <Input
        ref={inputRef}
        type="text"
        value={localValue}
        onChange={onChange}
        onKeyDown={onKeyDown}
        placeholder={placeholder}
        aria-invalid={hasError || undefined}
        className="pl-7 pr-2 w-full"
      />
    </div>
  );
}

function useDebouncedChange(onChange: (v: string) => void, debounceMs: number) {
  const debounceRef = useRef<NodeJS.Timeout | null>(null);
  useEffect(
    () => () => {
      if (debounceRef.current) clearTimeout(debounceRef.current);
    },
    [],
  );
  const emit = useCallback(
    (next: string) => {
      if (debounceRef.current) clearTimeout(debounceRef.current);
      debounceRef.current = setTimeout(() => onChange(next), debounceMs);
    },
    [onChange, debounceMs],
  );
  const flush = useCallback(
    (value: string) => {
      if (!debounceRef.current) return;
      clearTimeout(debounceRef.current);
      debounceRef.current = null;
      onChange(value);
    },
    [onChange],
  );
  return { emit, flush };
}

export function PanelSearchBar({
  value,
  onChange,
  onNext,
  onPrev,
  onClose,
  matchInfo,
  isLoading = false,
  placeholder = "Search…",
  toggles,
  hasError = false,
  errorText,
  debounceMs = 150,
  className,
}: PanelSearchBarProps) {
  const [localValue, setLocalValue] = useState(value);
  const inputRef = useRef<HTMLInputElement | null>(null);
  const { emit, flush } = useDebouncedChange(onChange, debounceMs);

  useEffect(() => {
    setLocalValue(value);
  }, [value]);

  useEffect(() => {
    inputRef.current?.focus();
    inputRef.current?.select();
  }, []);

  const handleChange = useCallback(
    (e: React.ChangeEvent<HTMLInputElement>) => {
      setLocalValue(e.target.value);
      emit(e.target.value);
    },
    [emit],
  );

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent<HTMLInputElement>) => {
      if (e.key === "Enter") {
        e.preventDefault();
        flush(localValue);
        if (e.shiftKey) onPrev();
        else onNext();
      } else if (e.key === "Escape") {
        e.preventDefault();
        e.stopPropagation();
        onClose();
      }
    },
    [localValue, flush, onNext, onPrev, onClose],
  );

  return (
    <div
      className={cn("absolute top-2 right-2 z-20 flex flex-col items-end gap-1", className)}
      data-panel-search-bar
    >
      <div
        className={cn(
          "flex items-center gap-1 rounded-md border border-border bg-background p-1 shadow-md",
          hasError && "border-destructive",
        )}
      >
        <SearchInputField
          localValue={localValue}
          onChange={handleChange}
          onKeyDown={handleKeyDown}
          placeholder={placeholder}
          isLoading={isLoading}
          hasError={hasError}
          inputRef={inputRef}
        />
        {matchInfo && <MatchCounter info={matchInfo} />}
        {toggles?.caseSensitive && (
          <ToggleButton state={toggles.caseSensitive} title="Match case">
            <IconLetterCase />
          </ToggleButton>
        )}
        {toggles?.regex && (
          <ToggleButton state={toggles.regex} title="Regular expression">
            <IconRegex />
          </ToggleButton>
        )}
        <ActionButtons onNext={onNext} onPrev={onPrev} onClose={onClose} />
      </div>
      {hasError && errorText && (
        <span className="text-[0.6875rem] text-destructive bg-background px-2 py-0.5 rounded border border-destructive">
          {errorText}
        </span>
      )}
    </div>
  );
}
