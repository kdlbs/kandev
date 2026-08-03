"use client";

import { Suspense, useState } from "react";
import { useRouter, useSearchParams } from "@/lib/routing/client-router";
import { IconCloud, IconServer } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@kandev/ui/card";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { Separator } from "@kandev/ui/separator";
import { createExecutorAction } from "@/app/actions/executors";
import { getWebSocketClient } from "@/lib/ws/connection";
import { useAppStore } from "@/components/state-provider";
import type { Executor } from "@/lib/types/http";
import { useTranslation } from "react-i18next";

const EXECUTOR_TYPES = ["local_docker", "remote_docker"] as const;
type ExecutorType = (typeof EXECUTOR_TYPES)[number];

export default function ExecutorCreatePage() {
  const { t } = useTranslation();
  return (
    <Suspense fallback={<div className="p-4">{t("executors:loading")}</div>}>
      <ExecutorCreatePageContent />
    </Suspense>
  );
}

type RemoteDockerFieldsProps = {
  dockerTlsVerify: string;
  onDockerTlsVerifyChange: (value: string) => void;
  dockerCertPath: string;
  onDockerCertPathChange: (value: string) => void;
  gitToken: string;
  onGitTokenChange: (value: string) => void;
};

function RemoteDockerFields({
  dockerTlsVerify,
  onDockerTlsVerifyChange,
  dockerCertPath,
  onDockerCertPathChange,
  gitToken,
  onGitTokenChange,
}: RemoteDockerFieldsProps) {
  const { t } = useTranslation();
  return (
    <>
      <div className="space-y-2">
        <Label htmlFor="docker-tls-verify">{t("executors:tlsVerify")}</Label>
        <Select value={dockerTlsVerify} onValueChange={onDockerTlsVerifyChange}>
          <SelectTrigger id="docker-tls-verify">
            <SelectValue placeholder={t("executors:defaultNoTls")} />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="1">{t("executors:enabled")}</SelectItem>
            <SelectItem value="0">{t("executors:disabled")}</SelectItem>
          </SelectContent>
        </Select>
      </div>
      <div className="space-y-2">
        <Label htmlFor="docker-cert-path">{t("executors:tlsCertificatePath")}</Label>
        <Input
          id="docker-cert-path"
          value={dockerCertPath}
          onChange={(event) => onDockerCertPathChange(event.target.value)}
          placeholder="/path/to/certs"
        />
        <p className="text-xs text-muted-foreground">
          {t("executors:pathToTlsCertificatesForThe")}
        </p>
      </div>
      <div className="space-y-2">
        <Label htmlFor="git-token">{t("executors:gitTokenOptional")}</Label>
        <Input
          id="git-token"
          type="password"
          value={gitToken}
          onChange={(event) => onGitTokenChange(event.target.value)}
          placeholder="ghp_..."
        />
        <p className="text-xs text-muted-foreground">
          {t("executors:personalAccessTokenForCloningRepositories")}
        </p>
      </div>
    </>
  );
}

type ExecutorFormCardProps = {
  type: ExecutorType;
  name: string;
  dockerHost: string;
  dockerTlsVerify: string;
  dockerCertPath: string;
  gitToken: string;
  onTypeChange: (value: ExecutorType) => void;
  onNameChange: (value: string) => void;
  onDockerHostChange: (value: string) => void;
  onDockerTlsVerifyChange: (value: string) => void;
  onDockerCertPathChange: (value: string) => void;
  onGitTokenChange: (value: string) => void;
};

function ExecutorFormCard({
  type,
  name,
  dockerHost,
  dockerTlsVerify,
  dockerCertPath,
  gitToken,
  onTypeChange,
  onNameChange,
  onDockerHostChange,
  onDockerTlsVerifyChange,
  onDockerCertPathChange,
  onGitTokenChange,
}: ExecutorFormCardProps) {
  const { t } = useTranslation();
  const isRemoteDocker = type === "remote_docker";
  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          {isRemoteDocker ? <IconCloud className="h-4 w-4" /> : <IconServer className="h-4 w-4" />}
          {isRemoteDocker
            ? t("executors:remoteDockerExecutor")
            : t("executors:localDockerExecutor")}
        </CardTitle>
        <CardDescription>
          {isRemoteDocker
            ? t("executors:connectsToARemoteDockerHost")
            : t("executors:usesTheLocalDockerDaemonOn")}
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="space-y-2">
          <Label htmlFor="executor-type">{t("executors:executorType")}</Label>
          <Select value={type} onValueChange={(value) => onTypeChange(value as ExecutorType)}>
            <SelectTrigger id="executor-type">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="local_docker">{t("executors:localDocker")}</SelectItem>
              <SelectItem value="remote_docker">{t("executors:remoteDocker")}</SelectItem>
            </SelectContent>
          </Select>
        </div>
        <div className="space-y-2">
          <Label htmlFor="executor-name">{t("executors:executorName")}</Label>
          <Input
            id="executor-name"
            value={name}
            onChange={(event) => onNameChange(event.target.value)}
          />
        </div>
        <div className="space-y-2">
          <Label htmlFor="docker-host">{t("executors:dockerHost")}</Label>
          <Input
            id="docker-host"
            value={dockerHost}
            onChange={(event) => onDockerHostChange(event.target.value)}
            placeholder={
              isRemoteDocker
                ? "tcp://remote:2376 or ssh://user@host"
                : "unix:///var/run/docker.sock"
            }
          />
          <p className="text-xs text-muted-foreground">
            {isRemoteDocker
              ? t("executors:theRemoteDockerHostUrlTcp")
              : t("executors:repositoriesWillBeMountedAsVolumes")}
          </p>
        </div>
        {isRemoteDocker && (
          <RemoteDockerFields
            dockerTlsVerify={dockerTlsVerify}
            onDockerTlsVerifyChange={onDockerTlsVerifyChange}
            dockerCertPath={dockerCertPath}
            onDockerCertPathChange={onDockerCertPathChange}
            gitToken={gitToken}
            onGitTokenChange={onGitTokenChange}
          />
        )}
      </CardContent>
    </Card>
  );
}

function buildExecutorConfig(
  type: ExecutorType,
  dockerHost: string,
  dockerTlsVerify: string,
  dockerCertPath: string,
  gitToken: string,
): Record<string, string> {
  const config: Record<string, string> = { docker_host: dockerHost };
  if (type === "remote_docker") {
    if (dockerTlsVerify) config.docker_tls_verify = dockerTlsVerify;
    if (dockerCertPath) config.docker_cert_path = dockerCertPath;
    if (gitToken) config.git_token = gitToken;
  }
  return config;
}

function ExecutorCreatePageContent() {
  const { t } = useTranslation();
  const router = useRouter();
  const searchParams = useSearchParams();
  const initialType = searchParams.get("type");
  const [type, setType] = useState<ExecutorType>(() => {
    if (EXECUTOR_TYPES.includes(initialType as ExecutorType)) return initialType as ExecutorType;
    return "local_docker";
  });
  // The seeded executor *name* is PERSISTED user data, not copy: it is sent as
  // `payload.name`, stored, and rendered afterwards on surfaces this PR does not
  // own. Translating it would make an executor's stored name depend on the
  // locale it happened to be created in. The Select labels beside it are copy
  // and do go through `t()`.
  const [name, setName] = useState(() =>
    initialType === "remote_docker" ? "Remote Docker" : "Local Docker",
  );
  const [dockerHost, setDockerHost] = useState(() =>
    initialType === "remote_docker" ? "tcp://" : "unix:///var/run/docker.sock",
  );
  const [dockerTlsVerify, setDockerTlsVerify] = useState("");
  const [dockerCertPath, setDockerCertPath] = useState("");
  const [gitToken, setGitToken] = useState("");
  const [isCreating, setIsCreating] = useState(false);
  const executors = useAppStore((state) => state.executors.items);
  const setExecutors = useAppStore((state) => state.setExecutors);

  const handleTypeChange = (value: ExecutorType) => {
    setType(value);
    if (value === "local_docker") {
      setName("Local Docker");
      setDockerHost("unix:///var/run/docker.sock");
    } else if (value === "remote_docker") {
      setName("Remote Docker");
      setDockerHost("tcp://");
    }
  };

  const handleCreate = async () => {
    setIsCreating(true);
    try {
      const config = buildExecutorConfig(
        type,
        dockerHost,
        dockerTlsVerify,
        dockerCertPath,
        gitToken,
      );
      const payload = { name, type, status: "active", config };
      const client = getWebSocketClient();
      const created = client
        ? await client.request<Executor>("executor.create", payload)
        : await createExecutorAction(payload);
      setExecutors([...executors.filter((item: Executor) => item.id !== created.id), created]);
      router.push("/settings/executors");
    } finally {
      setIsCreating(false);
    }
  };

  return (
    <div className="space-y-8">
      <div>
        <h2 className="text-2xl font-bold">{t("executors:createExecutor")}</h2>
        <p className="text-sm text-muted-foreground mt-1">
          {t("executors:chooseAnExecutorTypeToRun")}
        </p>
      </div>
      <Separator />
      <ExecutorFormCard
        type={type}
        name={name}
        dockerHost={dockerHost}
        dockerTlsVerify={dockerTlsVerify}
        dockerCertPath={dockerCertPath}
        gitToken={gitToken}
        onTypeChange={handleTypeChange}
        onNameChange={setName}
        onDockerHostChange={setDockerHost}
        onDockerTlsVerifyChange={setDockerTlsVerify}
        onDockerCertPathChange={setDockerCertPath}
        onGitTokenChange={setGitToken}
      />
      <div className="flex items-center justify-end gap-2">
        <Button variant="outline" onClick={() => router.push("/settings/executors")}>
          {t("common:cancel")}
        </Button>
        <Button onClick={handleCreate} disabled={isCreating}>
          {isCreating ? t("executors:creating") : t("executors:createExecutor")}
        </Button>
      </div>
    </div>
  );
}
