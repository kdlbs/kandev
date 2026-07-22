import { IconBrandGithub, IconKey, IconTerminal2 } from "@tabler/icons-react";
import { Label } from "@kandev/ui/label";
import { RadioGroup, RadioGroupItem } from "@kandev/ui/radio-group";
import { cn } from "@/lib/utils";

export type GitHubAutomationMethod = "pat" | "cli" | "app";

const methods = [
  {
    value: "pat" as const,
    label: "Personal access token",
    description: "Simple and portable, but automation is attributed to your personal account.",
    icon: IconKey,
  },
  {
    value: "cli" as const,
    label: "GitHub CLI account",
    description: "Reuse one named account already authenticated on this Kandev host.",
    icon: IconTerminal2,
  },
  {
    value: "app" as const,
    label: "GitHub App",
    description: "Isolated, short-lived credentials with explicit repository access.",
    icon: IconBrandGithub,
  },
];

export function GitHubAuthMethodList({
  value,
  onChange,
}: {
  value: GitHubAutomationMethod;
  onChange: (value: GitHubAutomationMethod) => void;
}) {
  return (
    <RadioGroup
      value={value}
      onValueChange={(next) => onChange(next as GitHubAutomationMethod)}
      className="grid gap-2 sm:grid-cols-3"
      aria-label="Connection method"
    >
      {methods.map((method) => {
        const Icon = method.icon;
        return (
          <Label
            key={method.value}
            htmlFor={`github-method-${method.value}`}
            className={cn(
              "flex min-h-24 cursor-pointer items-start gap-3 rounded-md border p-3",
              "transition-colors hover:bg-muted/40",
              value === method.value && "border-primary bg-muted/30",
            )}
          >
            <RadioGroupItem
              id={`github-method-${method.value}`}
              value={method.value}
              className="mt-0.5"
            />
            <span className="min-w-0 space-y-1">
              <span className="flex items-center gap-2 text-sm font-medium">
                <Icon className="h-4 w-4 shrink-0" />
                {method.label}
              </span>
              <span className="block text-xs font-normal leading-5 text-muted-foreground">
                {method.description}
              </span>
            </span>
          </Label>
        );
      })}
    </RadioGroup>
  );
}
