"use client";

import { useState } from "react";
import { Button } from "@kandev/ui/button";
import { Drawer, DrawerClose, DrawerFooter } from "@kandev/ui/drawer";
import { ChangeRequestStatusDrawerContent } from "./change-request-status-chrome";
import {
  IntegrationChangeRequestMultiStatusContent,
  IntegrationChangeRequestStatusContent,
} from "./integration-change-request-status-content";
import { IntegrationChangeRequestStatusTrigger } from "./integration-change-request-status-trigger";
import type { IntegrationChangeRequestStatusItem } from "./integration-change-request-status-types";
import { useRefreshItemsWhenOpen } from "./use-integration-status-hover";

export function MobileIntegrationChangeRequestStatus({
  items,
  surface,
}: {
  items: readonly IntegrationChangeRequestStatusItem[];
  surface: "topbar" | "composer";
}) {
  const [open, setOpen] = useState(false);
  const [single] = items;
  useRefreshItemsWhenOpen(open, items);
  return (
    <Drawer open={open} onOpenChange={setOpen}>
      <IntegrationChangeRequestStatusTrigger
        items={items}
        mobile={false}
        surface={surface}
        onClick={() => setOpen(true)}
      />
      <ChangeRequestStatusDrawerContent
        testId="integration-change-request-status-drawer"
        headerTestId="integration-change-request-status-drawer-header"
        closeTestId="integration-change-request-status-drawer-close"
        bodyTestId="integration-change-request-status-scroll-body"
        title={items.length === 1 ? `PR #${single.number}` : `${items.length} pull requests`}
        description="Pull request CI status, reviews, and checks summary."
        footer={
          items.length === 1 ? (
            <DrawerFooter className="shrink-0 border-t pb-[max(1rem,env(safe-area-inset-bottom))]">
              <DrawerClose asChild>
                <Button type="button" className="h-11 cursor-pointer" onClick={single.onOpenReview}>
                  Open review
                </Button>
              </DrawerClose>
            </DrawerFooter>
          ) : undefined
        }
      >
        {items.length === 1 ? (
          <IntegrationChangeRequestStatusContent item={single} mobile contained={false} />
        ) : (
          <IntegrationChangeRequestMultiStatusContent items={items} mobile contained={false} />
        )}
      </ChangeRequestStatusDrawerContent>
    </Drawer>
  );
}
