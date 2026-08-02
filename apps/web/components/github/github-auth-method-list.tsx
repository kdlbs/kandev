import { IconBrandGithub, IconKey, IconTerminal2 } from "@tabler/icons-react";
import { Label } from "@kandev/ui/label";
import { RadioGroup, RadioGroupItem } from "@kandev/ui/radio-group";
import { cn } from "@/lib/utils";
import { useTranslation } from "react-i18next";

export type GitHubAutomationMethod = "pat" | "cli" | "app";

// `value` is the wire enum the connection form switches on — never translated.
// The copy travels as catalog keys resolved at render: a `t()` call here would
// be module scope and freeze at the boot locale (see docs/i18n.md). Every entry
// carries both keys, so `t()` never receives `string | undefined`.
type AuthMethodOption = {
  value: GitHubAutomationMethod;
  labelKey: string;
  descriptionKey: string;
  icon: typeof IconTerminal2;
};

const methods: AuthMethodOption[] = [
  {
    value: "cli",
    labelKey: "github:githubCliAccount",
    descriptionKey: "github:resolvedFromOneNamedHostAccount",
    icon: IconTerminal2,
  },
  {
    value: "pat",
    labelKey: "github:personalAccessToken",
    descriptionKey: "github:storedEncryptedByKandevWorkspaceAutomation",
    icon: IconKey,
  },
  {
    value: "app",
    labelKey: "github:githubApp",
    descriptionKey: "github:storedAppCredentialsMintShortLived",
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
  const { t } = useTranslation();
  return (
    <RadioGroup
      value={value}
      onValueChange={(next) => onChange(next as GitHubAutomationMethod)}
      className="grid gap-2 sm:grid-cols-3"
      aria-label={t("github:connectionMethod")}
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
                {t(method.labelKey)}
              </span>
              <span className="block text-xs font-normal leading-5 text-muted-foreground">
                {t(method.descriptionKey)}
              </span>
            </span>
          </Label>
        );
      })}
    </RadioGroup>
  );
}
