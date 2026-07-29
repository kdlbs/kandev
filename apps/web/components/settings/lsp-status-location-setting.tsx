import { Label } from "@kandev/ui/label";
import { RadioGroup, RadioGroupItem } from "@kandev/ui/radio-group";
import type { LspStatusLocation } from "@/lib/types/http";

type LspStatusLocationSettingProps = {
  value: LspStatusLocation;
  baseline: LspStatusLocation;
  onChange: (value: LspStatusLocation) => void;
};

const OPTIONS = [
  {
    value: "toolbar",
    label: "Editor toolbar",
    description: "Show LSP status beside the active file's editor actions.",
  },
  {
    value: "status_bar",
    label: "Application status bar",
    description:
      "Show LSP status in the reorderable application status bar. If that bar is disabled or a touch-oriented layout is active, the editor toolbar is used instead.",
  },
] as const satisfies ReadonlyArray<{
  value: LspStatusLocation;
  label: string;
  description: string;
}>;

export function LspStatusLocationSetting({
  value,
  baseline,
  onChange,
}: LspStatusLocationSettingProps) {
  const isDirty = value !== baseline;
  return (
    <div className="space-y-3" data-settings-dirty={isDirty}>
      <div>
        <div className="text-sm font-medium text-foreground">Status location</div>
        <div className="text-xs text-muted-foreground">
          Choose where LSP startup, indexing, and connection details appear.
        </div>
      </div>
      <RadioGroup
        aria-label="LSP status location"
        value={value}
        onValueChange={(next) => onChange(next as LspStatusLocation)}
        className="grid gap-3 sm:grid-cols-2"
      >
        {OPTIONS.map((option) => (
          <Label
            key={option.value}
            htmlFor={`lsp-status-location-${option.value}`}
            className="flex min-h-11 cursor-pointer items-start gap-3 rounded-md border p-3 hover:bg-muted/30"
            data-settings-dirty={isDirty && value === option.value}
          >
            <RadioGroupItem
              id={`lsp-status-location-${option.value}`}
              value={option.value}
              className="mt-0.5"
            />
            <span className="min-w-0 space-y-1">
              <span className="block text-sm font-medium">{option.label}</span>
              <span className="block text-xs leading-relaxed text-muted-foreground">
                {option.description}
              </span>
            </span>
          </Label>
        ))}
      </RadioGroup>
    </div>
  );
}
