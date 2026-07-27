import { IconAlertTriangle, IconCheck } from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { Label } from "@kandev/ui/label";
import { RadioGroup, RadioGroupItem } from "@kandev/ui/radio-group";
import type { GitHubAppRegistrationCatalogItem } from "@/lib/types/github";
import { cn } from "@/lib/utils";

export function GitHubAppRegistrationList({
  registrations,
  value,
  onChange,
}: {
  registrations: GitHubAppRegistrationCatalogItem[];
  value: string;
  onChange: (registrationId: string) => void;
}) {
  if (!registrations.length) {
    return (
      <div className="rounded-md border border-dashed p-4 text-sm text-muted-foreground">
        No GitHub Apps are registered yet. Add an existing App or create one below.
      </div>
    );
  }
  return (
    <RadioGroup value={value} onValueChange={onChange} className="space-y-2">
      {registrations.map((registration) => (
        <Label
          key={registration.id}
          htmlFor={`github-app-${registration.id}`}
          className={cn(
            "flex min-h-20 cursor-pointer items-start gap-3 rounded-md border p-3",
            value === registration.id && "border-primary bg-muted/30",
          )}
        >
          <RadioGroupItem
            id={`github-app-${registration.id}`}
            value={registration.id}
            className="mt-1"
          />
          <span className="min-w-0 flex-1 space-y-1">
            <span className="flex min-w-0 flex-wrap items-center gap-2">
              <span className="break-words text-sm font-medium">{registration.display_name}</span>
              <Badge variant="outline" className="capitalize">
                {registration.visibility}
              </Badge>
              {registration.status === "invalid" && (
                <Badge variant="destructive">Needs attention</Badge>
              )}
              {registration.selected && (
                <Badge variant="secondary">
                  <IconCheck className="mr-1 h-3 w-3" /> In use
                </Badge>
              )}
            </span>
            <span className="block break-words text-xs font-normal text-muted-foreground">
              {registration.owner_login}/{registration.slug} ·{" "}
              {registration.source === "managed" ? "Created by Kandev" : "Imported"}
            </span>
            {registration.shared && (
              <span className="flex items-start gap-1.5 text-xs font-normal leading-5 text-amber-600 dark:text-amber-400">
                <IconAlertTriangle className="mt-0.5 h-3.5 w-3.5 shrink-0" />
                Used by {registration.workspace_binding_count} workspaces. Editing or deleting it
                affects all of them.
              </span>
            )}
            {registration.status === "invalid" && (
              <span className="block text-xs font-normal leading-5 text-destructive">
                {registration.last_error || "The stored App credentials are unavailable."}
              </span>
            )}
          </span>
        </Label>
      ))}
    </RadioGroup>
  );
}
