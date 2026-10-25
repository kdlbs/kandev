export function ContextValue({ label, value }: { label: string; value: string }) {
  return (
    <div className="min-w-0 space-y-1">
      <div className="text-xs text-muted-foreground">{label}</div>
      <div className="break-all font-mono text-xs" title={value}>
        {value}
      </div>
    </div>
  );
}

export function CapabilityGroup({
  title,
  capabilities,
  selected,
  disabled,
  onToggle,
}: {
  title: string;
  capabilities: string[];
  selected: string[];
  disabled: boolean;
  onToggle: (capabilityId: string, checked: boolean) => void;
}) {
  if (capabilities.length === 0) return null;
  return (
    <fieldset className="space-y-1" disabled={disabled}>
      <legend className="mb-1 text-sm font-medium">{title}</legend>
      <div className="grid gap-1 sm:grid-cols-2">
        {capabilities.map((capability) => {
          const resource = capability.substring(capability.indexOf(":") + 1);
          const checked = selected.includes(capability);
          return (
            <label
              key={capability}
              data-testid={`plugin-capability-${capability.replaceAll(":", "-")}`}
              className="flex min-h-8 cursor-pointer items-center gap-3 rounded-md px-3 py-2 hover:bg-muted/40 max-md:min-h-11 [@media(pointer:coarse)]:min-h-11"
            >
              <input
                type="checkbox"
                checked={checked}
                onChange={(event) => onToggle(capability, event.target.checked)}
                className="size-4 shrink-0 accent-primary"
              />
              <span className="text-sm">{resource}</span>
            </label>
          );
        })}
      </div>
    </fieldset>
  );
}
