import { useId } from "react";
import { useTranslation } from "react-i18next";
import { Switch } from "@kandev/ui/switch";

/** The label supplies the touch target while the switch retains its compact track. */
export function BuiltinPackageSwitch({
  label,
  checked,
  onCheckedChange,
  disabled,
  testId,
  dirty,
  describedBy,
  hideLabel = false,
}: {
  label: string;
  checked: boolean;
  onCheckedChange: (checked: boolean) => void;
  disabled: boolean;
  testId: string;
  dirty?: boolean;
  describedBy?: string;
  hideLabel?: boolean;
}) {
  const { t } = useTranslation();
  const id = useId();
  return (
    <label
      htmlFor={id}
      data-testid={testId}
      data-settings-dirty={dirty}
      className="flex min-h-7 cursor-pointer items-center justify-between gap-4 text-sm max-md:min-h-11 [@media(pointer:coarse)]:min-h-11 has-disabled:cursor-default"
    >
      <span className={hideLabel ? "sr-only" : "min-w-0 font-medium"}>{label}</span>
      <span className="flex shrink-0 items-center gap-3">
        <span aria-hidden className="text-muted-foreground">
          {t(checked ? "settings:builtinMcp.choices.on" : "settings:builtinMcp.choices.off")}
        </span>
        <Switch
          id={id}
          aria-label={label}
          aria-describedby={describedBy}
          checked={checked}
          onCheckedChange={onCheckedChange}
          disabled={disabled}
          className="transition-colors motion-reduce:transition-none [&_[data-slot=switch-thumb]]:motion-reduce:transition-none"
        />
      </span>
    </label>
  );
}
