"use client";

import { useTranslation } from "react-i18next";

export function FileBrowserStatus({ loading }: { loading: boolean }) {
  const { t } = useTranslation();
  return (
    <span role="status" className="sr-only">
      {loading ? t("editors:openingFolder") : ""}
    </span>
  );
}
