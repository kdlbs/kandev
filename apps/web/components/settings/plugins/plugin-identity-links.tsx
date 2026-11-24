"use client";

import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { controlSizingClassName } from "@kandev/ui/control-sizing";
import { SettingsGroup } from "@/components/settings/settings-group";
import { useIsAdmin } from "@/hooks/domains/auth/use-is-admin";
import type { PluginIdentityLink, PluginRecord } from "@/lib/types/plugins";
import { usePluginIdentityLinks } from "./use-plugin-identity-links";

const HUMAN_RESPONSES_RESOURCE = "human_responses";
const TOUCH_TARGET = "cursor-pointer max-md:min-h-11 [@media(pointer:coarse)]:min-h-11";

export function pluginSupportsIdentityLinks(plugin: PluginRecord): boolean {
  return plugin.capabilities.api_write?.includes(HUMAN_RESPONSES_RESOURCE) ?? false;
}

/**
 * Linked chat accounts: a Kandev user generates a short-lived code here and
 * gives it to the plugin's external surface, which links that account to the
 * user. Approvals relayed by the plugin then act as this Kandev user.
 */
export function PluginIdentityLinks({ plugin }: { plugin: PluginRecord }) {
  const { t } = useTranslation();
  const isAdmin = useIsAdmin();
  const state = usePluginIdentityLinks(plugin.id, isAdmin);

  return (
    <SettingsGroup
      title={t("plugins:identityLinks")}
      description={t("plugins:identityLinksDescription")}
      data-testid="plugin-identity-links"
      contentClassName="space-y-4"
    >
      <div className="space-y-3">
        <Button
          variant="outline"
          data-testid="plugin-identity-link-code-create"
          className={controlSizingClassName("standard", TOUCH_TARGET)}
          disabled={state.busy}
          onClick={() => void state.createCode()}
        >
          {t("plugins:identityLinkCodeCreate")}
        </Button>
        {state.code && (
          <div
            className="space-y-1 rounded-md border border-border/70 bg-muted/20 p-3"
            data-testid="plugin-identity-link-code"
          >
            <p className="font-mono text-lg tracking-wider select-all">{state.code.code}</p>
            <p className="text-sm text-muted-foreground">
              {t("plugins:identityLinkCodeInstructions")}
            </p>
          </div>
        )}
      </div>
      {state.errorKey && (
        <p className="text-sm text-destructive" role="alert">
          {t(state.errorKey)}
        </p>
      )}
      <IdentityLinkList
        links={state.links}
        loading={state.loading}
        showUser={isAdmin}
        busy={state.busy}
        onRevoke={(id) => void state.revoke(id)}
      />
    </SettingsGroup>
  );
}

function IdentityLinkList({
  links,
  loading,
  showUser,
  busy,
  onRevoke,
}: {
  links: PluginIdentityLink[];
  loading: boolean;
  showUser: boolean;
  busy: boolean;
  onRevoke: (id: string) => void;
}) {
  const { t } = useTranslation();
  if (loading) {
    return (
      <p className="text-sm text-muted-foreground" role="status">
        {t("plugins:identityLinksLoading")}
      </p>
    );
  }
  if (links.length === 0) {
    return <p className="text-sm text-muted-foreground">{t("plugins:identityLinksEmpty")}</p>;
  }
  return (
    <ul className="divide-y rounded-md border" data-testid="plugin-identity-link-list">
      {links.map((link) => (
        <li
          key={link.id}
          className="flex flex-col gap-2 p-3 sm:flex-row sm:items-center sm:justify-between"
          data-testid={`plugin-identity-link-${link.id}`}
        >
          <div className="min-w-0 text-sm">
            <p className="truncate font-medium">
              {t("plugins:identityLinkAccount", {
                provider: link.provider,
                name: link.display_name || link.subject,
              })}
            </p>
            {showUser && (
              <p className="truncate text-muted-foreground">
                {t("plugins:identityLinkUser", { user: link.user_display_name || link.user_id })}
              </p>
            )}
          </div>
          <Button
            variant="outline"
            data-testid={`plugin-identity-link-revoke-${link.id}`}
            className={controlSizingClassName("standard", TOUCH_TARGET)}
            disabled={busy}
            onClick={() => onRevoke(link.id)}
          >
            {t("plugins:identityLinkRevoke")}
          </Button>
        </li>
      ))}
    </ul>
  );
}
