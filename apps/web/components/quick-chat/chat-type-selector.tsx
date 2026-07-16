"use client";

import type { QuickChatSessionKind } from "@/lib/state/slices/ui/types";

type ChatTypeSelectorProps = {
  value: QuickChatSessionKind;
  disabled?: boolean;
  onChange: (kind: QuickChatSessionKind) => void;
};

const options: Array<{ value: QuickChatSessionKind; label: string }> = [
  { value: "chat", label: "Quick chat" },
  { value: "config", label: "Configuration chat" },
];

export function ChatTypeSelector({ value, disabled, onChange }: ChatTypeSelectorProps) {
  return (
    <section className="space-y-2" aria-labelledby="chat-type-label">
      <div>
        <h3 id="chat-type-label" className="text-sm font-medium">
          Chat type
        </h3>
        <p className="text-xs text-muted-foreground">
          Configuration chats can update Kandev settings and workflows.
        </p>
      </div>
      <div
        role="radiogroup"
        aria-labelledby="chat-type-label"
        className="grid grid-cols-2 rounded-md border bg-muted/30 p-1"
      >
        {options.map((option) => {
          const selected = value === option.value;
          return (
            <button
              key={option.value}
              type="button"
              role="radio"
              aria-checked={selected}
              disabled={disabled}
              onClick={() => onChange(option.value)}
              className={`min-h-11 cursor-pointer rounded-sm px-3 py-2 text-sm transition-colors ${
                selected
                  ? "bg-background font-medium text-foreground shadow-sm"
                  : "text-muted-foreground hover:bg-background/60 hover:text-foreground"
              }`}
            >
              {option.label}
            </button>
          );
        })}
      </div>
    </section>
  );
}
