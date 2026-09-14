# Kubernetes worker images

These recipes prepare the main container for a Kandev Kubernetes task session.
They do not start a session by themselves. Kandev adds the command, agentctl
helper, credentials, runtime files, and workspace mounts during launch.

## Targets and pins

Build the targets from the repository root with the immutable base and pnpm
inputs in pins.env.

| Target | Added or verified tools | Runtime paths |
|---|---|---|
| minimal | Node 24, npm, Python 3, venv, Git, and the POSIX tools from the published Kandev base | npm prefix /workspace/.npm-global, pnpm home /workspace/.pnpm, Python cache /workspace/.cache/pip |
| node-pnpm | pnpm 9.15.9 installed under /usr/local | npm globals use /workspace/.npm-global; pnpm uses /workspace/.pnpm and /workspace/.pnpm-store |
| python | Python 3 and venv from the published Kandev base | Python virtual environments and pip cache use /workspace |

The current pin is the multi-platform index for
ghcr.io/kdlbs/kandev:0.93.0:
sha256:1a3c98e62bb3ab20141695b6a6b6fdcd058480dd2ef42a89277724b95afc6884.
The image build and smoke command below is explicitly linux/amd64. A build
for another architecture does not establish Kubernetes lifecycle support for it.

~~~bash
bash scripts/test-kubernetes-worker-images.sh --check
bash scripts/test-kubernetes-worker-images.sh --smoke --platform linux/amd64
~~~

The smoke command builds each target with a unique local tag, runs as UID 1000,
uses a disposable Docker volume, performs a local Git commit, and checks the
target-specific toolchain. It removes only those tags and volumes when it exits.
It does not publish images.

## Use a target in a profile

Copy one of the strict PodTemplate examples from k8s/presets into the existing
Kubernetes executor profile. Replace its image marker with the immutable digest
of the image you built and pushed to your registry. Keep main_container
kandev-agent and set the profile platform to linux/amd64 for the evidence
covered here.

Each example starts with these profile fields:

~~~text
platform: linux/amd64
main_container: kandev-agent
workspace.mode: managed_pvc
workspace.size: 10Gi
workspace.access_modes: ["ReadWriteOnce"]
~~~

Use workspace.mode: empty_dir for disposable work, or
workspace.mode: existing_claim with an administrator-provisioned claim. The
managed PVC preserves the workspace across ordinary Stop and Resume. Archive or
Delete can remove a managed workspace through the existing task lifecycle.

The templates request 250m CPU and 512Mi memory, and limit memory to 2Gi.
These are starting values, not usage measurements. They run the main container
as UID 1000 with a non-root Pod security context, disable service-account token
automount, drop all Linux capabilities, and disable privilege escalation.

The templates deliberately omit Kandev-owned fields. Do not add commands,
arguments, working directories, reserved mounts, ports, restart policy, HOME,
or KANDEV_* environment keys. The existing profile validator rejects those
conflicts before a cluster write.

## Registry substitution

Build and push each target with your registry's immutable digest, then replace
only the image value in the selected PodTemplate. For a local Kind test, the
test fixture may use imagePullPolicy: Never and a locally loaded tag. That
fixture-only tag is not a production pin.

The recipes contain no credentials, repository content, host paths, or Docker
socket mounts. Registry access, Pod admission, storage policy, and outbound
connectivity remain administrator responsibilities.
