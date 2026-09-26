"use client";

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { IconChevronRight } from "@tabler/icons-react";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@kandev/ui/collapsible";
import { CommandPrefixField } from "@/components/settings/command-prefix-field";
import type { ProfileFormData } from "@/components/settings/profile-form-fields";

export function ProfileAdvancedOptions({
  profile,
  baselineProfile,
  onChange,
}: {
  profile: ProfileFormData;
  baselineProfile?: ProfileFormData;
  onChange: (patch: Partial<ProfileFormData>) => void;
}) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);

  return (
    <Collapsible
      open={open}
      onOpenChange={setOpen}
      className="space-y-1"
      data-testid="profile-advanced-options"
    >
      <CollapsibleTrigger asChild>
        <button
          type="button"
          className="flex min-h-11 w-full cursor-pointer items-center gap-2 rounded-md px-1 py-0 text-left font-medium hover:bg-muted/40 md:min-h-9"
          data-testid="profile-advanced-options-trigger"
        >
          <IconChevronRight
            className={`size-4 shrink-0 text-muted-foreground transition-transform ${open ? "rotate-90" : ""}`}
            aria-hidden="true"
          />
          <span>{t(open ? "common:hideAdvancedOptions" : "common:showAdvancedOptions")}</span>
        </button>
      </CollapsibleTrigger>
      <CollapsibleContent data-testid="profile-advanced-options-content">
        <CommandPrefixField
          profile={profile}
          baselineProfile={baselineProfile}
          onChange={onChange}
        />
      </CollapsibleContent>
    </Collapsible>
  );
}
