import { Label } from "@kandev/ui/label";
import { RadioGroup, RadioGroupItem } from "@kandev/ui/radio-group";
import type { GitHubAppVisibility } from "@/lib/types/github";

export function GitHubAppVisibilityField({
  value,
  onChange,
}: {
  value: GitHubAppVisibility;
  onChange: (value: GitHubAppVisibility) => void;
}) {
  return (
    <fieldset className="space-y-2">
      <legend className="text-sm font-medium">Who can install this App?</legend>
      <RadioGroup
        value={value}
        onValueChange={(next) => onChange(next as GitHubAppVisibility)}
        className="grid gap-2 sm:grid-cols-2"
      >
        <Label className="flex min-h-20 cursor-pointer items-start gap-3 rounded-md border p-3">
          <RadioGroupItem value="private" className="mt-0.5" />
          <span>
            <span className="block text-sm font-medium">Only the App owner</span>
            <span className="block text-xs font-normal leading-5 text-muted-foreground">
              Best for personal or company-internal automation.
            </span>
          </span>
        </Label>
        <Label className="flex min-h-20 cursor-pointer items-start gap-3 rounded-md border p-3">
          <RadioGroupItem value="public" className="mt-0.5" />
          <span>
            <span className="block text-sm font-medium">Other GitHub accounts</span>
            <span className="block text-xs font-normal leading-5 text-muted-foreground">
              Allows installation elsewhere. It does not publish to Marketplace or grant repos.
            </span>
          </span>
        </Label>
      </RadioGroup>
    </fieldset>
  );
}
