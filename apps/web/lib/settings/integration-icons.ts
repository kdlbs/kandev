import type { ComponentType } from "react";
import {
  IconBrandGithub,
  IconBrandGitlab,
  IconBrandSentry,
  IconHexagon,
  IconTicket,
} from "@tabler/icons-react";

import { AzureDevOpsIcon } from "@/components/icons/azure-devops-icon";
import { WORKSPACE_INTEGRATIONS } from "@/lib/settings-discovery/catalog/integrations";

type IntegrationSlug = (typeof WORKSPACE_INTEGRATIONS)[number][0];

/**
 * The product mark per integration slug.
 *
 * Typed as a `Record` over the catalog's own slug union rather than a loose
 * lookup with a fallback: adding an integration to `WORKSPACE_INTEGRATIONS`
 * then fails to compile until it has a mark, instead of silently rendering a
 * generic one. The catalog owns slug → brand name; this owns slug → icon.
 */
export const INTEGRATION_ICONS: Record<
  IntegrationSlug,
  ComponentType<{ className?: string }>
> = {
  "azure-devops": AzureDevOpsIcon,
  github: IconBrandGithub,
  gitlab: IconBrandGitlab,
  jira: IconTicket,
  linear: IconHexagon,
  sentry: IconBrandSentry,
};
