"use client";

import { Checkbox } from "@kandev/ui/checkbox";
import { Label } from "@kandev/ui/label";
import { HelpTip } from "@/components/settings/workflow-pipeline-editor-helpers";

export function PolicyCheckbox({
  id,
  checked,
  dirty,
  label,
  help,
  disabled,
  onChange,
  rowTestId,
  labelTestId,
  helpTestId,
}: {
  id: string;
  checked: boolean;
  dirty: boolean;
  label: string;
  help: string;
  disabled: boolean;
  onChange: (checked: boolean) => void;
  rowTestId?: string;
  labelTestId?: string;
  helpTestId?: string;
}) {
  return (
    <div className="flex items-start gap-3" data-testid={rowTestId}>
      <Checkbox
        id={id}
        className="mt-1"
        checked={checked}
        onCheckedChange={(value) => onChange(value === true)}
        disabled={disabled}
        data-settings-dirty={dirty}
      />
      <div className="flex min-w-0 items-center gap-1">
        <Label
          htmlFor={id}
          className="flex min-h-11 min-w-0 cursor-pointer items-center text-sm md:min-h-0 [@media(pointer:coarse)]:min-h-11"
          data-testid={labelTestId}
        >
          {label}
        </Label>
        <HelpTip testId={helpTestId} text={help} />
      </div>
    </div>
  );
}
