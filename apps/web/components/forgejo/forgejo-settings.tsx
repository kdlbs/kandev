"use client";

import { useCallback, useEffect, useState } from "react";
import { Button } from "@kandev/ui/button";
import { CardContent, CardDescription, CardHeader, CardTitle } from "@kandev/ui/card";
import { Input } from "@kandev/ui/input";
import { WorkspaceScopedSection } from "@/components/integrations/workspace-scoped-section";
import { SettingsCard } from "@/components/settings/settings-card";
import {
  listForgejoQueue,
  listForgejoRepositories,
  testForgejoConfig,
} from "@/lib/api/domains/forgejo-api";
import type {
  ForgejoActionPreset,
  ForgejoIssueWatch,
  ForgejoRepository,
  ForgejoReviewWatch,
} from "@/lib/types/forgejo";
import { useSettingsSaveContributor } from "@/components/settings/settings-save-provider";
import { useForgejoConfig } from "@/hooks/domains/forgejo/use-forgejo-config";
import { useForgejoIssueWatches } from "@/hooks/domains/forgejo/use-forgejo-issue-watches";
import { useForgejoReviewWatches } from "@/hooks/domains/forgejo/use-forgejo-review-watches";
import { useForgejoActionPresets } from "@/hooks/domains/forgejo/use-forgejo-action-presets";

// The remaining watch/preset sections are deliberately kept together until their
// dedicated components land; the connection state itself is now delegated to a hook.
// eslint-disable-next-line complexity
function ForgejoConnection({ workspaceId }: { workspaceId: string }) {
  const {
    config,
    error: configError,
    save: saveConfig,
    refresh: refreshConfig,
    disconnect: disconnectConfig,
  } = useForgejoConfig(workspaceId);
  const [origin, setOrigin] = useState("");
  const [savedOrigin, setSavedOrigin] = useState("");
  const [token, setToken] = useState("");
  const [webhookSecret, setWebhookSecret] = useState("");
  const [repositories, setRepositories] = useState<ForgejoRepository[]>([]);
  const [queue, setQueue] = useState<{
    issues: { repository: ForgejoRepository; issue: { number: number; title: string } }[];
    pull_requests: {
      repository: ForgejoRepository;
      pull_request: { number: number; title: string };
    }[];
  } | null>(null);
  const {
    watches,
    load: loadWatches,
    save: persistWatch,
    remove: removeWatch,
    poll: runWatchPoll,
  } = useForgejoIssueWatches(workspaceId);
  const [watchOwner, setWatchOwner] = useState("");
  const [watchRepo, setWatchRepo] = useState("");
  const [watchLabels, setWatchLabels] = useState("");
  const [watchWorkflow, setWatchWorkflow] = useState("");
  const [watchStep, setWatchStep] = useState("");
  const [watchRepository, setWatchRepository] = useState("");
  const [watchBaseBranch, setWatchBaseBranch] = useState("");
  const [watchPrompt, setWatchPrompt] = useState("");
  const [watchAgentProfile, setWatchAgentProfile] = useState("");
  const [watchExecutorProfile, setWatchExecutorProfile] = useState("");
  const [watchInterval, setWatchInterval] = useState("300");
  const [watchInflightLimit, setWatchInflightLimit] = useState("0");
  const {
    watches: reviewWatches,
    load: loadReviewWatches,
    save: persistReviewWatch,
    remove: removeReviewWatch,
    poll: runReviewWatchPoll,
  } = useForgejoReviewWatches(workspaceId);
  const [reviewOwner, setReviewOwner] = useState("");
  const [reviewRepo, setReviewRepo] = useState("");
  const [reviewWorkflow, setReviewWorkflow] = useState("");
  const {
    presets,
    load: loadPresets,
    save: persistPreset,
    remove: removePreset,
  } = useForgejoActionPresets(workspaceId);
  const [presetName, setPresetName] = useState("");
  const [presetInstructions, setPresetInstructions] = useState("");
  const [message, setMessage] = useState("");

  useEffect(() => {
    if (!config) return;
    setSavedOrigin(config.origin);
    setOrigin((current) => (current === savedOrigin ? config.origin : current));
  }, [config, savedOrigin]);
  const options = { workspaceId };
  const test = async () => {
    const result = await testForgejoConfig({ origin, token }, options);
    setMessage(
      result.ok ? `Connected as ${result.username}` : (result.error ?? "Connection failed"),
    );
  };
  const save = useCallback(async () => {
    const next = await saveConfig({
      origin,
      token: token || undefined,
      webhook_secret: webhookSecret || undefined,
    });
    setSavedOrigin(next.origin);
    setOrigin(next.origin);
    setToken("");
    setWebhookSecret("");
    setMessage(`Saved connection for ${next.username}`);
  }, [origin, saveConfig, token, webhookSecret]);
  const loadRepositories = async () => {
    const result = await listForgejoRepositories(options);
    setRepositories(result.repositories);
  };
  const loadQueue = async () => {
    setQueue(await listForgejoQueue(options));
  };
  const refresh = async () => {
    const next = await refreshConfig();
    setSavedOrigin(next.origin);
    setMessage(
      next.last_ok ? "Connection is healthy" : next.last_error || "Connection check failed",
    );
  };
  const disconnect = async () => {
    await disconnectConfig();
    setSavedOrigin("");
    setOrigin("");
    setToken("");
    setRepositories([]);
    setQueue(null);
    setMessage("Forgejo disconnected from this workspace");
  };
  const saveWatch = async () => {
    await persistWatch({
      owner: watchOwner,
      repo: watchRepo,
      labels: watchLabels,
      workflow_id: watchWorkflow,
      workflow_step_id: watchStep,
      repository_id: watchRepository,
      base_branch: watchBaseBranch,
      prompt: watchPrompt,
      agent_profile_id: watchAgentProfile,
      executor_profile_id: watchExecutorProfile,
      poll_interval_seconds: Number(watchInterval) || 300,
      inflight_limit: Number(watchInflightLimit) || 0,
      cleanup_policy: "auto",
      enabled: true,
    });
    setWatchOwner("");
    setWatchRepo("");
    setWatchLabels("");
    setWatchWorkflow("");
    setWatchStep("");
    setWatchRepository("");
    setWatchBaseBranch("");
    setWatchPrompt("");
    setWatchAgentProfile("");
    setWatchExecutorProfile("");
    setWatchInterval("300");
    setWatchInflightLimit("0");
  };
  const pollWatch = async (watch: ForgejoIssueWatch) => {
    const result = await runWatchPoll(watch.id);
    setMessage(
      `Watch found ${result.issues.length} matching issue${result.issues.length === 1 ? "" : "s"}`,
    );
  };
  const deleteWatch = async (watch: ForgejoIssueWatch) => {
    await removeWatch(watch.id);
  };
  const saveReviewWatch = async () => {
    await persistReviewWatch({
      owner: reviewOwner,
      repo: reviewRepo,
      workflow_id: reviewWorkflow,
      enabled: true,
    });
    setReviewOwner("");
    setReviewRepo("");
    setReviewWorkflow("");
  };
  const deleteReviewWatch = async (watch: ForgejoReviewWatch) => {
    await removeReviewWatch(watch.id);
  };
  const pollReviewWatch = async (watch: ForgejoReviewWatch) => {
    const result = await runReviewWatchPoll(watch.id);
    setMessage(
      `Review watch found ${result.pull_requests.length} open pull request${result.pull_requests.length === 1 ? "" : "s"}`,
    );
  };
  const savePreset = async () => {
    await persistPreset({ kind: "review", name: presetName, instructions: presetInstructions });
    setPresetName("");
    setPresetInstructions("");
  };
  const deletePreset = async (preset: ForgejoActionPreset) => {
    await removePreset(preset.id);
  };
  const useRepositoryForWatch = (repository: ForgejoRepository) => {
    setWatchOwner(repository.owner);
    setWatchRepo(repository.name);
    setWatchBaseBranch(repository.default_branch);
    setReviewOwner(repository.owner);
    setReviewRepo(repository.name);
    setMessage(`Selected ${repository.full_name} for a Forgejo watch.`);
  };

  const validOrigin = (() => {
    try {
      const url = new URL(origin.trim());
      return (
        (url.protocol === "http:" || url.protocol === "https:") && !url.username && !url.password
      );
    } catch {
      return false;
    }
  })();
  const connectionDirty = origin.trim() !== savedOrigin || Boolean(token) || Boolean(webhookSecret);
  useSettingsSaveContributor({
    id: `forgejo-connection:${workspaceId}`,
    revision: JSON.stringify({ origin: origin.trim(), token, webhookSecret }),
    isDirty: connectionDirty,
    canSave: validOrigin && (Boolean(config?.has_secret) || Boolean(token.trim())),
    invalidReason: !validOrigin
      ? "Enter a valid HTTP or HTTPS Forgejo origin."
      : "Enter a Forgejo token before connecting.",
    save,
    discard: () => {
      setOrigin(savedOrigin);
      setToken("");
      setWebhookSecret("");
    },
  });

  return (
    <SettingsCard>
      <CardHeader>
        <CardTitle>Forgejo</CardTitle>
        <CardDescription>
          Connect this workspace to a Forgejo server. The token is stored in Kandev’s secret store
          and is never made available to agents.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-3">
        <Input
          data-testid="forgejo-origin-input"
          type="url"
          placeholder="https://forgejo.example"
          value={origin}
          onChange={(event) => setOrigin(event.target.value)}
        />
        <Input
          data-testid="forgejo-token-input"
          type="password"
          placeholder={
            config?.has_secret
              ? "Token saved — enter a new value to replace it"
              : "Forgejo personal access token"
          }
          value={token}
          onChange={(event) => setToken(event.target.value)}
        />
        <Input
          data-testid="forgejo-webhook-secret-input"
          type="password"
          placeholder={
            config?.has_webhook_secret
              ? "Webhook secret saved — enter a new value to replace it"
              : "Webhook secret, optional"
          }
          value={webhookSecret}
          onChange={(event) => setWebhookSecret(event.target.value)}
        />
        <p className="text-sm text-muted-foreground">
          Set the same secret on the Forgejo webhook. It signs incoming issue events; polling still
          catches anything the webhook misses.
        </p>
        <div className="flex flex-wrap gap-2">
          <Button className="cursor-pointer" type="button" onClick={() => void test()}>
            Test connection
          </Button>
          <Button className="cursor-pointer" type="button" onClick={() => void save()}>
            Save connection
          </Button>
          {config?.has_secret ? (
            <Button
              className="cursor-pointer"
              variant="outline"
              type="button"
              onClick={() => void refresh()}
            >
              Refresh connection
            </Button>
          ) : null}
          {config?.has_secret ? (
            <Button
              className="cursor-pointer"
              variant="outline"
              type="button"
              onClick={() => void loadRepositories()}
            >
              Load repositories
            </Button>
          ) : null}
          {config?.has_secret ? (
            <Button
              className="cursor-pointer"
              variant="outline"
              type="button"
              onClick={() => void loadQueue()}
            >
              Refresh queue
            </Button>
          ) : null}
          {config?.has_secret ? (
            <Button
              className="cursor-pointer"
              variant="destructive"
              type="button"
              onClick={() => void disconnect()}
            >
              Disconnect
            </Button>
          ) : null}
        </div>
        {message || configError ? (
          <p role="status" className="text-sm text-muted-foreground">
            {message || configError}
          </p>
        ) : null}
        {repositories.length ? (
          <div className="space-y-2">
            <p className="font-medium text-sm">Accessible repositories</p>
            <ul className="space-y-1 text-sm">
              {repositories.map((repository) => (
                <li
                  className="flex flex-wrap items-center justify-between gap-2"
                  key={repository.full_name}
                >
                  <a
                    className="hover:underline"
                    href={repository.html_url}
                    target="_blank"
                    rel="noreferrer"
                  >
                    {repository.full_name}
                  </a>
                  <Button
                    className="cursor-pointer"
                    size="sm"
                    type="button"
                    variant="outline"
                    onClick={() => useRepositoryForWatch(repository)}
                  >
                    Use for watch
                  </Button>
                </li>
              ))}
            </ul>
          </div>
        ) : null}
        {queue ? (
          <div className="space-y-3 text-sm">
            <div>
              <p className="font-medium">Open issues</p>
              <ul className="space-y-1">
                {queue.issues.map(({ repository, issue }) => (
                  <li key={`${repository.full_name}-${issue.number}`}>
                    {repository.full_name} #{issue.number}: {issue.title}
                  </li>
                ))}
              </ul>
            </div>
            <div>
              <p className="font-medium">Open pull requests</p>
              <ul className="space-y-1">
                {queue.pull_requests.map(({ repository, pull_request: pull }) => (
                  <li key={`${repository.full_name}-${pull.number}`}>
                    {repository.full_name} #{pull.number}: {pull.title}
                  </li>
                ))}
              </ul>
            </div>
          </div>
        ) : null}
        {config?.has_secret ? (
          <div className="space-y-2 border-t pt-3">
            <p className="font-medium text-sm">Issue watches</p>
            <p className="text-sm text-muted-foreground">
              A matching issue creates a task in the workflow and step you choose. Use repository ID
              and agent profile ID when this workspace has multiple repositories or agents. An
              inflight limit of 0 means unlimited active tasks.
            </p>
            <div className="grid gap-2 sm:grid-cols-3">
              <Input
                placeholder="Owner"
                value={watchOwner}
                onChange={(event) => setWatchOwner(event.target.value)}
              />
              <Input
                placeholder="Repository"
                value={watchRepo}
                onChange={(event) => setWatchRepo(event.target.value)}
              />
              <Input
                placeholder="Labels, optional"
                value={watchLabels}
                onChange={(event) => setWatchLabels(event.target.value)}
              />
              <Input
                placeholder="Workflow ID"
                value={watchWorkflow}
                onChange={(event) => setWatchWorkflow(event.target.value)}
              />
              <Input
                placeholder="Workflow step ID, optional"
                value={watchStep}
                onChange={(event) => setWatchStep(event.target.value)}
              />
              <Input
                placeholder="Kandev repository ID, optional"
                value={watchRepository}
                onChange={(event) => setWatchRepository(event.target.value)}
              />
              <Input
                placeholder="Base branch, optional"
                value={watchBaseBranch}
                onChange={(event) => setWatchBaseBranch(event.target.value)}
              />
              <Input
                placeholder="Agent profile ID, optional"
                value={watchAgentProfile}
                onChange={(event) => setWatchAgentProfile(event.target.value)}
              />
              <Input
                placeholder="Executor profile ID, optional"
                value={watchExecutorProfile}
                onChange={(event) => setWatchExecutorProfile(event.target.value)}
              />
              <Input
                type="number"
                min="30"
                placeholder="Poll interval seconds"
                value={watchInterval}
                onChange={(event) => setWatchInterval(event.target.value)}
              />
              <Input
                type="number"
                min="0"
                placeholder="Inflight limit (0 unlimited)"
                value={watchInflightLimit}
                onChange={(event) => setWatchInflightLimit(event.target.value)}
              />
              <Input
                placeholder="Task instructions, optional"
                value={watchPrompt}
                onChange={(event) => setWatchPrompt(event.target.value)}
              />
            </div>
            <div className="flex flex-wrap gap-2">
              <Button
                className="cursor-pointer"
                type="button"
                disabled={!watchOwner || !watchRepo || !watchWorkflow}
                onClick={() => void saveWatch()}
              >
                Save watch
              </Button>
              <Button
                className="cursor-pointer"
                variant="outline"
                type="button"
                onClick={() => void loadWatches()}
              >
                Load watches
              </Button>
            </div>
            <ul className="space-y-1 text-sm">
              {watches.map((watch) => (
                <li className="flex flex-wrap items-center gap-2" key={watch.id}>
                  <span>
                    {watch.owner}/{watch.repo}
                    {watch.labels ? ` · ${watch.labels}` : ""}
                    {watch.workflow_id ? ` → ${watch.workflow_id}` : ""}
                  </span>
                  <Button
                    className="cursor-pointer"
                    size="sm"
                    variant="outline"
                    type="button"
                    onClick={() => void pollWatch(watch)}
                  >
                    Poll
                  </Button>
                  <Button
                    className="cursor-pointer"
                    size="sm"
                    variant="destructive"
                    type="button"
                    onClick={() => void deleteWatch(watch)}
                  >
                    Delete
                  </Button>
                </li>
              ))}
            </ul>
          </div>
        ) : null}
        {config?.has_secret ? (
          <div className="space-y-2 border-t pt-3">
            <p className="font-medium text-sm">Review watches</p>
            <p className="text-sm text-muted-foreground">
              Each open pull request in the selected Forgejo repository creates one Kandev task. Use
              this for a recurring review queue.
            </p>
            <div className="grid gap-2 sm:grid-cols-3">
              <Input
                placeholder="Owner"
                value={reviewOwner}
                onChange={(event) => setReviewOwner(event.target.value)}
              />
              <Input
                placeholder="Repository"
                value={reviewRepo}
                onChange={(event) => setReviewRepo(event.target.value)}
              />
              <Input
                placeholder="Workflow ID"
                value={reviewWorkflow}
                onChange={(event) => setReviewWorkflow(event.target.value)}
              />
            </div>
            <div className="flex flex-wrap gap-2">
              <Button
                className="cursor-pointer"
                type="button"
                disabled={!reviewOwner || !reviewRepo || !reviewWorkflow}
                onClick={() => void saveReviewWatch()}
              >
                Save review watch
              </Button>
              <Button
                className="cursor-pointer"
                variant="outline"
                type="button"
                onClick={() => void loadReviewWatches()}
              >
                Load review watches
              </Button>
            </div>
            <ul className="space-y-1 text-sm">
              {reviewWatches.map((watch) => (
                <li className="flex flex-wrap items-center gap-2" key={watch.id}>
                  <span>
                    {watch.owner}/{watch.repo} → {watch.workflow_id}
                  </span>
                  <Button
                    className="cursor-pointer"
                    size="sm"
                    variant="outline"
                    type="button"
                    onClick={() => void pollReviewWatch(watch)}
                  >
                    Poll
                  </Button>
                  <Button
                    className="cursor-pointer"
                    size="sm"
                    variant="destructive"
                    type="button"
                    onClick={() => void deleteReviewWatch(watch)}
                  >
                    Delete
                  </Button>
                </li>
              ))}
            </ul>
          </div>
        ) : null}
        {config?.has_secret ? (
          <div className="space-y-2 border-t pt-3">
            <p className="font-medium text-sm">Action presets</p>
            <p className="text-sm text-muted-foreground">
              Save reusable review instructions, then copy them into a review action or watch when
              needed.
            </p>
            <div className="grid gap-2 sm:grid-cols-2">
              <Input
                placeholder="Preset name"
                value={presetName}
                onChange={(event) => setPresetName(event.target.value)}
              />
              <Input
                placeholder="Instructions"
                value={presetInstructions}
                onChange={(event) => setPresetInstructions(event.target.value)}
              />
            </div>
            <div className="flex flex-wrap gap-2">
              <Button
                className="cursor-pointer"
                type="button"
                disabled={!presetName}
                onClick={() => void savePreset()}
              >
                Save preset
              </Button>
              <Button
                className="cursor-pointer"
                variant="outline"
                type="button"
                onClick={() => void loadPresets()}
              >
                Load presets
              </Button>
            </div>
            <ul className="space-y-1 text-sm">
              {presets.map((preset) => (
                <li className="flex flex-wrap items-center gap-2" key={preset.id}>
                  <span>
                    {preset.name}
                    {preset.instructions ? ` · ${preset.instructions}` : ""}
                  </span>
                  <Button
                    className="cursor-pointer"
                    size="sm"
                    variant="destructive"
                    type="button"
                    onClick={() => void deletePreset(preset)}
                  >
                    Delete
                  </Button>
                </li>
              ))}
            </ul>
          </div>
        ) : null}
      </CardContent>
    </SettingsCard>
  );
}

export function ForgejoIntegrationPage({ workspaceId }: { workspaceId?: string } = {}) {
  return (
    <WorkspaceScopedSection workspaceId={workspaceId}>
      {(ws) => <ForgejoConnection workspaceId={ws} />}
    </WorkspaceScopedSection>
  );
}
