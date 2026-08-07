"use client";

import { useCallback, useEffect, useState } from "react";
import {
  IconPlus,
  IconTrash,
  IconBrandTelegram,
  IconBrandSlack,
  IconBrandDiscord,
  IconWebhook,
} from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Badge } from "@kandev/ui/badge";
import { Input } from "@kandev/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter } from "@kandev/ui/dialog";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { toast } from "@/lib/toast/sonner";
import type { AgentProfile } from "@/lib/state/slices/office/types";
import * as officeApi from "@/lib/api/domains/office-api";
import { useTranslation } from "react-i18next";

type Channel = {
  id: string;
  platform: string;
  config: string;
  status: string;
  task_id: string;
  created_at: string;
};

type AgentChannelsTabProps = {
  agent: AgentProfile;
};

const PLATFORM_ICONS: Record<string, typeof IconBrandTelegram> = {
  telegram: IconBrandTelegram,
  slack: IconBrandSlack,
  discord: IconBrandDiscord,
  webhook: IconWebhook,
};

export function AgentChannelsTab({ agent }: AgentChannelsTabProps) {
  const { t } = useTranslation();
  const [channels, setChannels] = useState<Channel[]>([]);
  const [addDialogOpen, setAddDialogOpen] = useState(false);

  useEffect(() => {
    let cancelled = false;
    officeApi
      .listChannels(agent.id)
      .then((res) => {
        if (!cancelled) {
          setChannels((res as { channels?: Channel[] }).channels ?? []);
        }
      })
      .catch((err) => {
        toast.error(err instanceof Error ? err.message : t("office:failedToLoadChannels"));
      });
    return () => {
      cancelled = true;
    };
  }, [agent.id]);

  const handleDelete = useCallback(
    async (channelId: string) => {
      try {
        await officeApi.deleteChannel(agent.id, channelId);
        setChannels((prev) => prev.filter((c) => c.id !== channelId));
        toast.success(t("office:channelDeleted"));
      } catch (err) {
        toast.error(err instanceof Error ? err.message : t("office:failedToDeleteChannel"));
      }
    },
    [agent.id],
  );

  const handleCreated = useCallback((channel: Channel) => {
    setChannels((prev) => [...prev, channel]);
    setAddDialogOpen(false);
  }, []);

  return (
    <div className="mt-4 space-y-4">
      <div className="flex items-center">
        <Button
          variant="outline"
          size="sm"
          onClick={() => setAddDialogOpen(true)}
          className="cursor-pointer"
        >
          <IconPlus className="h-4 w-4 mr-1" />
          {t("office:addChannel")}
        </Button>
      </div>

      {channels.length === 0 ? (
        <div className="flex items-center justify-center py-12">
          <p className="text-sm text-muted-foreground">
            {t("office:noChannelsConfiguredAddAChannel")}
          </p>
        </div>
      ) : (
        <div className="border border-border rounded-lg divide-y divide-border">
          {channels.map((channel) => (
            <ChannelRow
              key={channel.id}
              channel={channel}
              onDelete={() => handleDelete(channel.id)}
            />
          ))}
        </div>
      )}

      <AddChannelDialog
        open={addDialogOpen}
        onOpenChange={setAddDialogOpen}
        agent={agent}
        onCreated={handleCreated}
      />
    </div>
  );
}

function ChannelRow({ channel, onDelete }: { channel: Channel; onDelete: () => void }) {
  const { t } = useTranslation();
  const PlatformIcon = PLATFORM_ICONS[channel.platform] ?? IconWebhook;
  const statusColor =
    channel.status === "active"
      ? "bg-green-100 text-green-700 dark:bg-green-900/50 dark:text-green-300"
      : "bg-neutral-100 text-neutral-700 dark:bg-neutral-900/50 dark:text-neutral-300";

  return (
    <div className="flex items-center gap-3 px-4 py-3">
      <PlatformIcon className="h-5 w-5 text-muted-foreground shrink-0" />
      <div className="flex-1 min-w-0">
        <p className="text-sm font-medium capitalize">{channel.platform}</p>
        <p className="text-xs text-muted-foreground">
          {t("office:sortCreated")} {new Date(channel.created_at).toLocaleDateString()}
        </p>
      </div>
      <Badge className={statusColor}>{channel.status}</Badge>
      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            variant="ghost"
            size="icon"
            className="h-7 w-7 shrink-0 cursor-pointer"
            onClick={onDelete}
          >
            <IconTrash className="h-4 w-4" />
          </Button>
        </TooltipTrigger>
        <TooltipContent>{t("office:deleteChannel")}</TooltipContent>
      </Tooltip>
    </div>
  );
}

function AddChannelDialog({
  open,
  onOpenChange,
  agent,
  onCreated,
}: {
  open: boolean;
  onOpenChange: (v: boolean) => void;
  agent: AgentProfile;
  onCreated: (channel: Channel) => void;
}) {
  const { t } = useTranslation();
  const [platform, setPlatform] = useState("telegram");
  const [config, setConfig] = useState("");
  const [submitting, setSubmitting] = useState(false);

  const handleSubmit = useCallback(async () => {
    setSubmitting(true);
    try {
      const res = await officeApi.setupChannel(agent.id, {
        workspace_id: agent.workspaceId,
        platform,
        config: config || "{}",
        status: "active",
      });
      onCreated((res as { channel: Channel }).channel);
      toast.success(t("office:channelAdded"));
    } catch (err) {
      toast.error(err instanceof Error ? err.message : t("office:failedToAddChannel"));
    } finally {
      setSubmitting(false);
    }
  }, [agent.id, agent.workspaceId, platform, config, onCreated]);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("office:addChannel")}</DialogTitle>
        </DialogHeader>
        <div className="space-y-4">
          <div>
            <label className="text-sm font-medium">{t("office:platform")}</label>
            <Select value={platform} onValueChange={setPlatform}>
              <SelectTrigger className="mt-1 cursor-pointer">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="telegram" className="cursor-pointer">
                  {t("office:telegram")}
                </SelectItem>
                <SelectItem value="slack" className="cursor-pointer">
                  Slack
                </SelectItem>
                <SelectItem value="discord" className="cursor-pointer">
                  {t("office:discord")}
                </SelectItem>
                <SelectItem value="webhook" className="cursor-pointer">
                  {t("office:webhook")}
                </SelectItem>
              </SelectContent>
            </Select>
          </div>
          <div>
            <label className="text-sm font-medium">{t("office:configJson")}</label>
            <Input
              placeholder={t("office:botTokenChatId")}
              value={config}
              onChange={(e) => setConfig(e.target.value)}
              className="mt-1 font-mono text-xs"
            />
          </div>
        </div>
        <DialogFooter>
          <Button variant="ghost" onClick={() => onOpenChange(false)} className="cursor-pointer">
            {t("common:cancel")}
          </Button>
          <Button onClick={handleSubmit} disabled={submitting} className="cursor-pointer">
            {submitting ? t("office:creating") : t("office:addChannel")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
