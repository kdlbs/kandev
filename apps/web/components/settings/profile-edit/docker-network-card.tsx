"use client";

import { IconInfoCircle, IconPlus, IconTrash } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { CardContent } from "@kandev/ui/card";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { SettingsCard } from "@/components/settings/settings-card";
import { SettingsCardHeader } from "@/components/settings/settings-card-header";
import type { AdditionalNetworkRow } from "@/components/settings/profile-edit/use-docker-networks-form-state";
import { useTranslation } from "react-i18next";

type DockerNetworkCardProps = {
  primaryNetwork: string;
  onPrimaryNetworkChange: (value: string) => void;
  primaryGwPriority: string;
  onPrimaryGwPriorityChange: (value: string) => void;
  additionalNetworks: AdditionalNetworkRow[];
  onAddAdditionalNetwork: () => void;
  onUpdateAdditionalNetwork: (index: number, patch: Partial<AdditionalNetworkRow>) => void;
  onRemoveAdditionalNetwork: (index: number) => void;
  baselinePrimaryNetwork?: string;
  baselinePrimaryGwPriority?: string;
  baselineAdditionalNetworks?: AdditionalNetworkRow[];
};

export function DockerNetworkCard({
  primaryNetwork,
  onPrimaryNetworkChange,
  primaryGwPriority,
  onPrimaryGwPriorityChange,
  additionalNetworks,
  onAddAdditionalNetwork,
  onUpdateAdditionalNetwork,
  onRemoveAdditionalNetwork,
  baselinePrimaryNetwork = "",
  baselinePrimaryGwPriority = "",
  baselineAdditionalNetworks = [],
}: DockerNetworkCardProps) {
  const { t } = useTranslation();

  const primaryDirty = primaryNetwork !== baselinePrimaryNetwork;
  const priorityDirty = primaryGwPriority !== baselinePrimaryGwPriority;
  const additionalDirty =
    JSON.stringify(additionalNetworks) !== JSON.stringify(baselineAdditionalNetworks);

  return (
    <SettingsCard isDirty={primaryDirty || priorityDirty || additionalDirty}>
      <SettingsCardHeader
        title={t("executors:containerNetworksTitle")}
        description={t("executors:containerNetworksDescription")}
      />
      <CardContent className="space-y-4">
        <div className="flex flex-col gap-4 md:flex-row md:items-start">
          <div className="flex-1 space-y-2">
            <Label htmlFor="docker-primary-network">{t("executors:primaryNetwork")}</Label>
            <Input
              id="docker-primary-network"
              value={primaryNetwork}
              onChange={(e) => onPrimaryNetworkChange(e.target.value)}
              className="font-mono text-sm"
              data-settings-dirty={primaryDirty}
            />
          </div>
          <div className="space-y-2 md:w-40">
            <Label htmlFor="docker-primary-gw-priority">{t("executors:gatewayPriority")}</Label>
            <Input
              id="docker-primary-gw-priority"
              type="number"
              step={1}
              value={primaryGwPriority}
              onChange={(e) => onPrimaryGwPriorityChange(e.target.value)}
              className="font-mono text-sm"
              data-settings-dirty={priorityDirty}
            />
          </div>
        </div>

        <NetworkNote>{t("executors:primaryNetworkEmptyDaemonDefault")}</NetworkNote>
        <NetworkNote>{t("executors:primaryNetworkMustPublishPorts")}</NetworkNote>

        <div className="space-y-2">
          <Label>{t("executors:additionalNetworks")}</Label>
          {additionalNetworks.map((row, index) => (
            <AdditionalNetworkRowFields
              key={index}
              index={index}
              row={row}
              onUpdate={onUpdateAdditionalNetwork}
              onRemove={onRemoveAdditionalNetwork}
            />
          ))}
          <Button
            variant="outline"
            size="sm"
            onClick={onAddAdditionalNetwork}
            className="min-h-11 cursor-pointer md:min-h-9"
          >
            <IconPlus className="mr-1 h-4 w-4" />
            {t("executors:addNetwork")}
          </Button>
        </div>

        <NetworkNote>{t("executors:gatewayPriorityNote")}</NetworkNote>
      </CardContent>
    </SettingsCard>
  );
}

/** An informational line under the network fields. */
function NetworkNote({ children }: { children: React.ReactNode }) {
  return (
    <p className="flex items-start gap-2 text-sm text-muted-foreground">
      <IconInfoCircle className="mt-0.5 h-4 w-4 shrink-0" />
      <span>{children}</span>
    </p>
  );
}

/** One additional-network row: name, optional priority, remove. */
function AdditionalNetworkRowFields({
  index,
  row,
  onUpdate,
  onRemove,
}: {
  index: number;
  row: AdditionalNetworkRow;
  onUpdate: (index: number, patch: Partial<AdditionalNetworkRow>) => void;
  onRemove: (index: number) => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="flex flex-col gap-2 rounded-md border p-3 md:flex-row md:items-end md:border-0 md:p-0">
      <div className="flex-1 space-y-2">
        <Label className="md:sr-only" htmlFor={`docker-additional-network-${index}`}>
          {t("executors:networkName")}
        </Label>
        <Input
          id={`docker-additional-network-${index}`}
          value={row.name}
          onChange={(e) => onUpdate(index, { name: e.target.value })}
          className="font-mono text-sm"
        />
      </div>
      <div className="space-y-2 md:w-40">
        <Label className="md:sr-only" htmlFor={`docker-additional-priority-${index}`}>
          {t("executors:gatewayPriority")}
        </Label>
        <Input
          id={`docker-additional-priority-${index}`}
          type="number"
          step={1}
          value={row.gwPriority}
          onChange={(e) => onUpdate(index, { gwPriority: e.target.value })}
          className="font-mono text-sm"
        />
      </div>
      <Button
        variant="ghost"
        size="icon"
        aria-label={t("executors:removeAdditionalNetwork")}
        onClick={() => onRemove(index)}
        className="min-h-11 min-w-11 cursor-pointer text-muted-foreground md:min-h-9 md:min-w-9"
      >
        <IconTrash className="h-4 w-4" />
      </Button>
    </div>
  );
}
