"use client";

import { useMemo, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import {
  SettingsTabs,
  SettingsTabsList,
  SettingsTabsPanel,
} from "@/components/settings/settings-tabs";
import { useCoordinatorSection } from "@/hooks/domains/coordinator/use-coordinator-section";

export type CoordinatorSectionEntry = {
  slug: string;
  label: string;
  help: string;
  render: () => ReactNode;
};

type SectionsRowProps = {
  entries: readonly CoordinatorSectionEntry[];
};

/**
 * The coordinator page's Sections row. Entries render in the order given;
 * sections owned by other work orders register by adding an entry.
 */
export function SectionsRow({ entries }: SectionsRowProps) {
  const { t } = useTranslation();
  const slugKey = entries.map((entry) => entry.slug).join(",");
  const slugs = useMemo(() => slugKey.split(","), [slugKey]);
  const { value, selectSection } = useCoordinatorSection(slugs);
  const tabs = entries.map(({ slug, label }) => ({ id: slug, label }));
  const activeHelp = entries.find((entry) => entry.slug === value)?.help;

  return (
    <SettingsTabs tabs={tabs} value={value} onValueChange={selectSection}>
      <SettingsTabsList
        ariaLabel={t("coordinator:sectionsRowLabel")}
        className="md:flex-wrap md:overflow-x-visible"
      />
      {activeHelp && (
        <p className="mt-3 text-sm text-muted-foreground" data-testid="coordinator-section-help">
          {activeHelp}
        </p>
      )}
      <div className="mt-4">
        {entries.map((entry) => (
          <SettingsTabsPanel
            key={entry.slug}
            value={entry.slug}
            testId={`coordinator-section-${entry.slug}`}
          >
            {entry.render()}
          </SettingsTabsPanel>
        ))}
      </div>
    </SettingsTabs>
  );
}
