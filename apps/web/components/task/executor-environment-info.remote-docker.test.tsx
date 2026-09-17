import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";

import type {
  ContainerLiveStatus,
  SSHLiveStatus,
  TaskEnvironment,
} from "@/lib/api/domains/task-environment-api";
import { EnvironmentInfo } from "./executor-environment-info";

const CONTAINER_ID = "326694a5666a0a9b896749ef19b85396";
const SHORT_ID = CONTAINER_ID.slice(0, 12);

function environment(executorType: string): TaskEnvironment {
  return {
    id: "environment-1",
    task_id: "task-1",
    repository_id: "repository-1",
    executor_type: executorType,
    executor_id: "executor-1",
    executor_profile_id: "profile-1",
    agent_execution_id: "execution-1",
    control_port: 8765,
    status: "ready",
    container_id: CONTAINER_ID,
    created_at: "2026-09-17T09:30:00Z",
  };
}

const RUNNING: ContainerLiveStatus = {
  container_id: CONTAINER_ID,
  state: "running",
  status: "Up 21 minutes",
};

const REMOTE_SSH: SSHLiveStatus = { host: "10.8.4.206", user: "root", port: 22 };

type InfoProps = {
  env: TaskEnvironment;
  container: ContainerLiveStatus | null;
  ssh: SSHLiveStatus | null;
  kubernetes: null;
};
const Info = EnvironmentInfo as unknown as React.ComponentType<InfoProps>;

function shellValue(): string {
  // The shell command is rendered as a copyable field value; find it by its
  // docker-exec prefix rather than by position, which shifts with the rows
  // above it.
  const match = screen
    .getAllByText(/docker exec/)
    .map((node) => node.textContent ?? "")
    .find((text) => text.includes(SHORT_ID));
  return match ?? "";
}

afterEach(cleanup);

describe("EnvironmentInfo shell hint", () => {
  // A remote Docker container is not on the machine the user is sitting at,
  // so a bare `docker exec` is a command that cannot work. It has to be run
  // through the same SSH connection the executor uses.
  it("routes the command through SSH for a remote Docker environment", () => {
    render(
      <Info
        env={environment("remote_docker")}
        container={RUNNING}
        ssh={REMOTE_SSH}
        kubernetes={null}
      />,
    );

    expect(shellValue()).toBe(`ssh root@10.8.4.206 docker exec -it ${SHORT_ID} sh`);
  });

  it("omits the user when the connection did not report one", () => {
    render(
      <Info
        env={environment("remote_docker")}
        container={RUNNING}
        ssh={{ host: "10.8.4.206" }}
        kubernetes={null}
      />,
    );

    expect(shellValue()).toBe(`ssh 10.8.4.206 docker exec -it ${SHORT_ID} sh`);
  });

  it("includes a non-default port so the command reaches the right host", () => {
    render(
      <Info
        env={environment("remote_docker")}
        container={RUNNING}
        ssh={{ host: "10.8.4.206", user: "root", port: 2222 }}
        kubernetes={null}
      />,
    );

    expect(shellValue()).toBe(`ssh -p 2222 root@10.8.4.206 docker exec -it ${SHORT_ID} sh`);
  });

  // Without a host there is nothing truthful to prefix, and guessing would be
  // worse than the plain form.
  it("falls back to the plain command when no host is known", () => {
    render(
      <Info env={environment("remote_docker")} container={RUNNING} ssh={null} kubernetes={null} />,
    );

    expect(shellValue()).toBe(`docker exec -it ${SHORT_ID} sh`);
  });

  it("leaves a local Docker environment unchanged", () => {
    render(
      <Info env={environment("local_docker")} container={RUNNING} ssh={null} kubernetes={null} />,
    );

    expect(shellValue()).toBe(`docker exec -it ${SHORT_ID} sh`);
  });
});
