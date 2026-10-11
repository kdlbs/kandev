import { test, expect, fullWorkerPrepare } from "../../fixtures/kubernetes-docker-test-base";
import { kubernetesProfileConfig } from "../../fixtures/kubernetes-test-base";
import {
  execInKubernetesPod,
  waitForKubernetesPod,
  waitForKubernetesPVC,
  waitForKubernetesRestart,
  waitForKubernetesResourceAbsent,
} from "../../helpers/kubernetes";
import {
  waitForSessionDone,
  waitForAgentMessage,
  waitForSessionState,
} from "../../helpers/session";
import { SessionPage } from "../../pages/session-page";
import { watchWs } from "../../helpers/causal-waits";
import {
  isolatedWorkerTemplate,
  loadValidationImage,
  podCommand,
  validationEvidence,
  liveValidationCgroup,
} from "../../helpers/kubernetes-validation";

test.describe.configure({ retries: 0 });

// @covers AC-EXECUTORS-K8S-FAILURE-RECOVERY-001.9 through .13
// Precise cleanup-before-poll and credential persistence failures also have lifecycle unit gates.
test("main restart preserves native conversation and one continuation", async ({
  apiClient,
  seedData,
  cluster,
  backend,
  testPage,
}, testInfo) => {
  test.setTimeout(360_000);
  const releaseEnv = await backend.useEnv({
    KANDEV_FEATURES_PROVIDER_INTERRUPTION_CONTINUATION: "false",
  });
  const baselineSettings = (await apiClient.getUserSettings()).settings;
  const { agents } = await apiClient.listAgents();
  const mock = agents.find((agent) => agent.name === "mock-agent")!;
  const agent = await apiClient.createAgentProfile(mock.id, "Restart trace", {
    env_vars: [{ key: "E2E_MOCK_AGENT_ACP_TRACE_FILE", value: "/workspace/restart-trace.jsonl" }],
  });
  const profile = await apiClient.createExecutorProfile(seedData.executorId, {
    name: "Restart retained workspace",
    config: kubernetesProfileConfig(cluster, {
      "workspace.mode": "managed_pvc",
      "workspace.size": "1Gi",
      "workspace.access_modes": '["ReadWriteOnce"]',
    }),
    prepare_script: "",
    cleanup_script: "",
    env_vars: [],
  });
  let paused = false;
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "Native restart continuity",
    agent.id,
    {
      description: 'e2e:message("original-once")',
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      executor_id: seedData.executorId,
      executor_profile_id: profile.id,
    },
  );
  try {
    await waitForSessionDone(apiClient, task.id, task.session_id!, "Initial native conversation");
    const before = await waitForKubernetesPod(cluster, task.id, task.session_id!);
    const claim = await waitForKubernetesPVC(cluster, task.id, task.session_id!);
    execInKubernetesPod(cluster, before.metadata.name, [
      "sh",
      "-ceu",
      "printf retained > /workspace/restart-sentinel",
    ]);
    const trace = () =>
      execInKubernetesPod(cluster, before.metadata.name, ["cat", "/workspace/restart-trace.jsonl"])
        .trim()
        .split("\n")
        .map(
          (line) =>
            JSON.parse(line) as {
              event: string;
              session_id: string;
              connection_id: string;
              prompt?: string;
            },
        );
    const original = trace().filter((row) => row.event === "session_new");
    expect(original).toHaveLength(1);
    const restarts = before.status!.containerStatuses!.find(
      (row) => row.name === "kandev-agent",
    )!.restartCount;
    await apiClient.addUserMessage(
      task.id,
      task.session_id!,
      'e2e:message("interrupted-once")\ne2e:delay(120000)',
    );
    await expect
      .poll(
        () =>
          trace().filter(
            (row) => row.event === "prompt" && row.prompt?.includes("interrupted-once"),
          ).length,
      )
      .toBe(1);
    await waitForSessionState(apiClient, {
      taskId: task.id,
      sessionId: task.session_id!,
      expectedState: "RUNNING",
      message: "Native prompt accepted while the interrupted turn is active",
    });
    // Pause only the fixture backend: no background poll can repair credentials during restart.
    process.kill(backend.pid()!, "SIGSTOP");
    paused = true;
    try {
      execInKubernetesPod(cluster, before.metadata.name, ["sh", "-c", "kill -KILL 1"]);
    } catch {
      /* PID 1 closes its exec stream. */
    }
    const restarted = await waitForKubernetesRestart(cluster, before.metadata.name, restarts);
    const termination = restarted.status?.containerStatuses?.find(
      (row) => row.name === "kandev-agent",
    )?.lastState?.terminated;
    expect(termination?.exitCode, "Active main process must be killed abnormally").toBe(137);
    await waitForKubernetesPod(cluster, task.id, task.session_id!);
    process.kill(backend.pid()!, "SIGCONT");
    paused = false;
    await apiClient.saveUserSettings({ prevent_auto_start_agent_on_open: true });
    const ws = watchWs(testPage);
    const page = new SessionPage(testPage);
    await testPage.goto(`/t/${task.id}`);
    await page.waitForLoad();
    await expect(page.recoveryResumeButton()).toBeVisible({ timeout: 60_000 });
    const [response] = await Promise.all([
      ws.waitForResponse("session.recover", { timeout: 90_000 }),
      page.recoveryResumeButton().click(),
    ]);
    expect(response.payload.success).toBe(true);
    await apiClient.addUserMessage(task.id, task.session_id!, 'e2e:message("continued-once")');
    await waitForAgentMessage(apiClient, task.session_id!, "continued-once");
    await waitForSessionDone(apiClient, task.id, task.session_id!, "Native continuation");
    await expect(page.activeChat()).toContainText("continued-once");
    await testPage.reload();
    await page.waitForLoad();
    await expect(page.activeChat()).toContainText("continued-once");
    const rows = trace();
    expect(rows.filter((row) => row.event === "session_new")).toHaveLength(1);
    expect(rows.filter((row) => row.event === "prompt")).toHaveLength(3);
    const loads = rows.filter((row) => row.event === "session_load");
    expect(loads).toHaveLength(1);
    expect(loads[0].session_id).toBe(original[0].session_id);
    expect(loads[0].connection_id).not.toBe(original[0].connection_id);
    expect(
      rows.filter((row) => row.event === "prompt" && row.prompt?.includes("interrupted-once")),
    ).toHaveLength(1);
    expect(
      rows.filter((row) => row.event === "prompt" && row.prompt?.includes("original-once")),
    ).toHaveLength(1);
    expect(
      rows.filter((row) => row.event === "prompt" && row.prompt?.includes("continued-once")),
    ).toHaveLength(1);
    expect((await waitForKubernetesPod(cluster, task.id, task.session_id!)).metadata.uid).toBe(
      before.metadata.uid,
    );
    expect((await waitForKubernetesPVC(cluster, task.id, task.session_id!)).metadata.uid).toBe(
      claim.metadata.uid,
    );
    expect(
      execInKubernetesPod(cluster, before.metadata.name, ["cat", "/workspace/restart-sentinel"]),
    ).toBe("retained");
    await testInfo.attach("native-restart-receipt", {
      body: JSON.stringify({
        pod: before.metadata,
        claim: claim.metadata,
        termination,
        original,
        loads,
        prompts: rows.filter((row) => row.event === "prompt"),
      }),
      contentType: "application/json",
    });
    await apiClient.archiveTask(task.id);
    await waitForKubernetesResourceAbsent(cluster, "pod", before.metadata.name);
    await waitForKubernetesResourceAbsent(cluster, "persistentvolumeclaim", claim.metadata.name);
  } finally {
    if (paused) process.kill(backend.pid()!, "SIGCONT");
    await apiClient.saveUserSettings({
      prevent_auto_start_agent_on_open: baselineSettings.prevent_auto_start_agent_on_open === true,
    });
    await apiClient.deleteExecutorProfile(profile.id);
    await apiClient.deleteAgentProfile(agent.id, true);
    await releaseEnv();
  }
});

// @covers AC-EXECUTORS-K8S-VALIDATION-001.1 through .6
// No opt-in skip: a requested acceptance run without the image must fail.
test("shared bounded validation survives child OOM and rejects unverifiable admission", async ({
  apiClient,
  seedData,
  cluster,
  fullWorkerImage,
  testPage,
}, testInfo) => {
  test.setTimeout(1_200_000);
  const image = process.env.KANDEV_E2E_FULL_WORKER_IMAGE!;
  const { agents } = await apiClient.listAgents();
  const mock = agents.find((agent) => agent.name === "mock-agent")!;
  const agent = await apiClient.createAgentProfile(mock.id, "Validation native trace", {
    env_vars: [
      { key: "E2E_MOCK_AGENT_ACP_TRACE_FILE", value: "/workspace/validation-native-trace.jsonl" },
    ],
  });
  const profile = await apiClient.createExecutorProfile(seedData.executorId, {
    name: "Isolated validation acceptance",
    config: kubernetesProfileConfig(cluster, {
      pod_template_yaml: isolatedWorkerTemplate(fullWorkerImage, image),
      "workspace.mode": "managed_pvc",
      "workspace.size": "2Gi",
      "workspace.access_modes": '["ReadWriteOnce"]',
    }),
    prepare_script: fullWorkerPrepare().replace(
      "proof_deadline=$(( $(date +%s) + 240 ))",
      "proof_deadline=$(( $(date +%s) + 660 ))",
    ),
    cleanup_script: "",
    env_vars: [],
  });
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "Validation remains alive",
    agent.id,
    {
      description: 'e2e:message("agent-ready")',
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      executor_id: seedData.executorId,
      executor_profile_id: profile.id,
    },
  );
  try {
    const pod = await waitForKubernetesPod(cluster, task.id, task.session_id!);
    const claim = await waitForKubernetesPVC(cluster, task.id, task.session_id!);
    await loadValidationImage(cluster, pod.metadata.name, image);
    await waitForSessionDone(
      apiClient,
      task.id,
      task.session_id!,
      "Accounting-gated preparation",
      660_000,
    );
    const sibling = await apiClient.launchSession(
      {
        task_id: task.id,
        agent_profile_id: agent.id,
        executor_id: seedData.executorId,
        executor_profile_id: profile.id,
        prompt: 'e2e:message("sibling-ready")',
      },
      120_000,
    );
    await waitForSessionDone(apiClient, task.id, sibling.session_id, "Sibling validation session");
    const evidence = await validationEvidence(cluster, pod.metadata.name);
    await testInfo.attach("companion-accounting", {
      body: JSON.stringify(evidence),
      contentType: "application/json",
    });
    const run = (script: string) => podCommand(cluster, pod.metadata.name, script);
    const nativeBefore = JSON.parse(
      (
        await run(
          "python3 -c 'import json; print(json.dumps([json.loads(line) for line in open(\"/workspace/validation-native-trace.jsonl\")]))'",
        )
      ).stdout,
    ) as Array<{ event: string; session_id: string; connection_id: string }>;
    const conversations = nativeBefore.filter((row) => row.event === "session_new");
    expect(conversations).toHaveLength(2);
    const setup = await run(`mkdir -p /workspace/check-source; cd /workspace/check-source
printf 'module checksource\\n\\ngo 1.26.0\\n' > go.mod
printf 'package main\\nfunc main() {}\\n' > main.go
printf 'version: "2"\\n' > .golangci.yml
cat > browser.cjs <<'JS'
const { chromium } = require('/opt/full-worker/playwright-core');
(async () => { const browser = await chromium.launch({headless:true,args:['--disable-dev-shm-usage']}); try {const page=await browser.newPage();await page.setContent('<button onclick="this.textContent=42">run</button>');await page.getByRole('button').click();if(await page.getByRole('button').textContent()!=='42')throw Error('interaction');await page.screenshot({path:'/workspace/validation-browser.png'});} finally {await browser.close();} })().catch(e=>{console.error(e);process.exitCode=1;});
JS`);
    expect(setup.code).toBe(0);
    const check = "cd /workspace/check-source; python3 /opt/full-worker/check.py";
    for (const command of [
      "--kind lint -- golangci-lint run --concurrency=2 ./...",
      "--kind build -- go build -o /workspace/check-binary .",
      "--kind browser -- node browser.cjs",
    ]) {
      const result = await run(`${check} ${command}`);
      expect(result.code, result.stderr).toBe(0);
    }
    const first = run(
      `${check} --kind test -- bash -ceu 'printf first-start >> /workspace/order; touch /workspace/first-started; while [ ! -e /workspace/release-first ]; do sleep .1; done; printf first-end >> /workspace/order'`,
    );
    await expect.poll(async () => (await run("test -f /workspace/first-started")).code).toBe(0);
    const second = run(
      `${check} --kind test -- bash -ceu 'printf second-start >> /workspace/order'`,
    );
    expect(
      (await run("docker ps -q --filter name=^/kandev-validation$")).stdout.trim().split("\n"),
    ).toHaveLength(1);
    await testInfo.attach("independent-validator-cgroup", {
      body: await liveValidationCgroup(cluster, pod.metadata.name),
      contentType: "text/plain",
    });
    await run("touch /workspace/release-first");
    expect((await first).code).toBe(0);
    expect((await second).code).toBe(0);
    expect((await run("cat /workspace/order")).stdout).toBe("first-startfirst-endsecond-start");
    const oom = await run(
      `${check.replace("python3", "FULL_WORKER_CHECK_MEMORY_BYTES=67108864 FULL_WORKER_CHECK_JOB_SECONDS=20 python3")} --kind test -- python3 -c 'data=bytearray(128*1024*1024)'`,
    );
    expect(oom.code, oom.stderr).toBe(137);
    expect(oom.stderr).toContain("exceeded its memory limit");
    expect(
      (
        await run(
          `${check.replace("python3", "FULL_WORKER_CHECK_JOB_SECONDS=1 python3")} --kind test -- sleep 30`,
        )
      ).code,
    ).toBe(124);
    const cancel = await run(
      `${check} --kind test -- sleep 30 & job=$!; while ! docker inspect --format '{{.State.Running}}' kandev-validation 2>/dev/null | grep -q true; do kill -0 "$job"; sleep .1; done; kill -TERM "$job"; set +e; wait "$job"; exit "$?"`,
    );
    expect(cancel.code).toBe(130);
    expect((await run("docker ps -aq --filter name=^/kandev-validation$")).stdout).toBe("");
    const rejection = `${check} --kind test -- touch /workspace/forbidden-start`;
    const malformed = await run(
      `FULL_WORKER_CHECK_IMAGE=mutable:latest ${rejection.replace("cd /workspace/check-source; ", "")}`,
    );
    expect(malformed.code).toBe(2);
    const missingImage = await run(
      `${rejection.replace("python3", `FULL_WORKER_CHECK_IMAGE=sha256:${"b".repeat(64)} python3`)}`,
    );
    expect(missingImage.code).toBe(2);
    const missingReceipt = await run(
      `mv /run/docker/validation-accounting.json /run/docker/receipt.saved; trap 'mv /run/docker/receipt.saved /run/docker/validation-accounting.json' EXIT; set +e; ${rejection}`,
    );
    expect(missingReceipt.code).toBe(2);
    const foreign = await run(
      `foreign=$(docker create --name kandev-validation --network=none --memory=67108864 --entrypoint=sh ${image} -c true); trap 'docker rm "$foreign" >/dev/null' EXIT; set +e; ${rejection}; status=$?; test "$(docker inspect --format '{{.Id}}' kandev-validation)" = "$foreign" || exit 99; exit "$status"`,
    );
    expect(foreign.code).toBe(2);
    const overBudget = await run(
      `other=$(docker run -d --network=none --memory=2147483648 --memory-swap=2147483648 --entrypoint=sh ${image} -c 'sleep 30'); trap 'docker rm -f "$other" >/dev/null' EXIT; set +e; ${rejection.replace("python3", "FULL_WORKER_CHECK_QUEUE_SECONDS=1 python3")}`,
    );
    expect(overBudget.code).toBe(2);
    const unbounded = await run(
      `other=$(docker run -d --network=none --entrypoint=sh ${image} -c 'sleep 30'); trap 'docker rm -f "$other" >/dev/null' EXIT; set +e; ${rejection}`,
    );
    expect(unbounded.code).toBe(2);
    expect((await run("test ! -e /workspace/forbidden-start")).code).toBe(0);
    expect((await run("docker ps -aq --filter name=^/kandev-validation$")).stdout).toBe("");
    const after = await waitForKubernetesPod(cluster, task.id, task.session_id!);
    expect(after.metadata.uid).toBe(pod.metadata.uid);
    expect(after.status!.containerStatuses!.map((row) => [row.name, row.restartCount])).toEqual(
      pod.status!.containerStatuses!.map((row) => [row.name, row.restartCount]),
    );
    await apiClient.addUserMessage(
      task.id,
      sibling.session_id,
      'e2e:message("survived-check-oom")',
    );
    await waitForAgentMessage(apiClient, sibling.session_id, "survived-check-oom");
    await waitForSessionDone(apiClient, task.id, sibling.session_id, "Sibling survives check OOM");
    await testPage.goto(`/t/${task.id}`);
    const page = new SessionPage(testPage);
    await page.waitForLoad();
    await expect(page.activeChat()).toContainText("agent-ready");
    await testPage.reload();
    await page.waitForLoad();
    await expect(page.activeChat()).toContainText("agent-ready");
    await validationEvidence(cluster, pod.metadata.name);
    const nativeAfter = JSON.parse(
      (
        await run(
          "python3 -c 'import json; print(json.dumps([json.loads(line) for line in open(\"/workspace/validation-native-trace.jsonl\")]))'",
        )
      ).stdout,
    ) as Array<{ event: string; session_id: string; connection_id: string }>;
    expect(nativeAfter.filter((row) => row.event === "session_new")).toEqual(conversations);
    expect(nativeAfter.filter((row) => row.event === "session_load")).toHaveLength(0);
    await testInfo.attach("validation-survival", {
      body: JSON.stringify({
        pod: pod.metadata,
        before: pod.status,
        after: after.status,
        conversations,
        oom,
        timeoutCode: 124,
        cancellationCode: 130,
      }),
      contentType: "application/json",
    });
    await apiClient.archiveTask(task.id);
    await waitForKubernetesResourceAbsent(cluster, "pod", pod.metadata.name);
    await waitForKubernetesResourceAbsent(cluster, "persistentvolumeclaim", claim.metadata.name);
  } finally {
    await apiClient.deleteExecutorProfile(profile.id);
    await apiClient.deleteAgentProfile(agent.id, true);
  }
});
