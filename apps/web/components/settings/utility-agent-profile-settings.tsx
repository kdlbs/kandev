"use client";

import { useEffect, useRef, useState } from "react";
import { CardContent, CardDescription, CardHeader, CardTitle } from "@kandev/ui/card";
import { Label } from "@kandev/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { updateUserSettings } from "@/lib/api";
import { useHealthyAgentProfiles } from "@/hooks/domains/settings/use-healthy-agent-profiles";
import { SettingsCard } from "./settings-card";
import { useSettingsSaveContributor } from "./settings-save-provider";

const NONE_VALUE = "none";

export function UtilityAgentProfileSettings() {
  const preference = useAppStore((state) => state.userSettings.utilityAgentProfileId) ?? "";
  const setUserSettings = useAppStore((state) => state.setUserSettings);
  const storeApi = useAppStoreApi();
  const healthyProfiles = useHealthyAgentProfiles(preference || undefined);
  const [saved, setSaved] = useState(preference);
  const [draft, setDraft] = useState(preference);
  const draftRef = useRef(draft);
  draftRef.current = draft;
  const isDirty = draft !== saved;

  useEffect(() => {
    setSaved((previous) => {
      if (draftRef.current === previous) setDraft(preference);
      return preference;
    });
  }, [preference]);

  useSettingsSaveContributor({
    id: "general-utility-agent-profile",
    order: 20,
    revision: draft,
    isDirty,
    save: async (revision) => {
      const submitted = revision as string;
      await updateUserSettings({ utility_agent_profile_id: submitted });
      setSaved(submitted);
      setUserSettings({
        ...storeApi.getState().userSettings,
        utilityAgentProfileId: submitted || null,
      });
    },
    discard: () => setDraft(saved),
  });

  return (
    <SettingsCard isDirty={isDirty} data-testid="utility-agent-profile-card">
      <CardHeader>
        <CardTitle className="text-base">
          <h3>Utility agent</h3>
        </CardTitle>
        <CardDescription>
          Agent profile used for lightweight one-shot LLM calls that plugins delegate to (e.g.
          summaries). Plugins with the <code>agent_invoke</code> capability call this agent so they
          need no API key of their own. Choose &quot;None&quot; to leave delegated calls unavailable
          until an agent profile is selected.
        </CardDescription>
      </CardHeader>
      <CardContent>
        <div className="w-full max-w-sm space-y-1.5">
          <Label htmlFor="utility-agent-profile-select">Agent profile</Label>
          <Select
            value={draft || NONE_VALUE}
            onValueChange={(value) => setDraft(value === NONE_VALUE ? "" : value)}
          >
            <SelectTrigger
              id="utility-agent-profile-select"
              className="w-full cursor-pointer"
              data-testid="utility-agent-profile-select"
              data-settings-dirty={isDirty}
            >
              <SelectValue placeholder="None" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value={NONE_VALUE} className="cursor-pointer">
                None
              </SelectItem>
              {healthyProfiles.map((profile) => (
                <SelectItem key={profile.id} value={profile.id} className="cursor-pointer">
                  {profile.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
      </CardContent>
    </SettingsCard>
  );
}
