"use client";

import type { RefObject } from "react";
import { useTranslation } from "react-i18next";
import { IconRefresh } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { settingsActionClassName } from "./settings-control";

/**
 * The persistent runtime entry. Desktop places it beside Install agents; the
 * phone layout gives it a full-width row below the page heading so the entry
 * stays visible regardless of page scroll position.
 */
export function RuntimeUpdatesButton({
  count,
  isMobile,
  onClick,
  buttonRef,
}: {
  count: number;
  isMobile: boolean;
  onClick: (opener: HTMLElement) => void;
  buttonRef?: RefObject<HTMLButtonElement | null>;
}) {
  const { t } = useTranslation();
  const label = t("agents:runtimeUpdatesOpen", { count });
  if (isMobile) {
    return (
      <Button
        ref={buttonRef}
        type="button"
        variant="outline"
        className="min-h-12 w-full cursor-pointer justify-center"
        onClick={(event) => onClick(event.currentTarget)}
        aria-label={label}
        data-testid="runtime-updates-button"
      >
        <IconRefresh className="mr-2 h-4 w-4" aria-hidden="true" />
        {t("agents:runtimeUpdatesTitle")}
        {count > 0 && <RuntimeCountBadge count={count} />}
      </Button>
    );
  }
  return (
    <Button
      ref={buttonRef}
      type="button"
      variant="outline"
      className={settingsActionClassName("cursor-pointer")}
      onClick={(event) => onClick(event.currentTarget)}
      aria-label={label}
      data-testid="runtime-updates-button"
    >
      <IconRefresh className="mr-2 h-4 w-4" aria-hidden="true" />
      {t("agents:runtimeUpdatesTitle")}
      {count > 0 && <RuntimeCountBadge count={count} />}
    </Button>
  );
}

function RuntimeCountBadge({ count }: { count: number }) {
  return (
    <span
      className="ml-2 inline-flex min-w-5 items-center justify-center rounded-full bg-sky-500 px-1.5 text-xs font-semibold text-white"
      data-testid="runtime-updates-count"
      aria-hidden="true"
    >
      {count}
    </span>
  );
}
