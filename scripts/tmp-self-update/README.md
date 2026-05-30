# Temporary self-update manual test scripts

Temporary helpers for testing service-gated UI self-update from a source checkout.
Remove this directory after the manual test pass.

## Quick path

Fake helper path:

```
scripts/tmp-self-update/build-local.sh
scripts/tmp-self-update/setup-fake-service.sh
scripts/tmp-self-update/run-app.sh
```

In another shell:

```
scripts/tmp-self-update/seed-update.sh
```

Open:

```
http://localhost:38429/settings/system/updates
```

Expected result: the Updates card shows `Apply update`. Clicking it should queue
a fake `self-update` job because `KANDEV_E2E_MOCK=true` is set by the generated
test environment.

## Real npm upgrade path

This path installs a real user service and writes an isolated npm prefix under
`$TEST_HOME/npm-global`. It should not overwrite a Homebrew `kandev` binary, but
still use a disposable machine or VM because it owns the Kandev user service
while the test is running.

It builds this branch as the previous published npm version, installs that
temporary package into the isolated prefix, installs a user service from it, and
seeds the update cache to the current npm latest. Clicking `Apply update` should
run:

```
npm install -g --prefix "$TEST_HOME/npm-global" kandev@latest
kandev service install --home-dir "$TEST_HOME"
systemctl --user restart kandev
```

On macOS the final restart command is
`launchctl kickstart -k gui/<uid>/com.kdlbs.kandev`.

Run:

```
KANDEV_REAL_NPM_CONFIRM=1 scripts/tmp-self-update/real-npm-setup.sh
scripts/tmp-self-update/real-npm-seed-latest.sh
```

The setup step fails early if the installed service metadata is not detected as
`kind=npm`, because the UI updater is intentionally hidden for unsupported
install kinds.

Then open:

```
http://localhost:38429/settings/system/updates
```

Click `Apply update`, wait for the service to restart, then check:

```
scripts/tmp-self-update/real-npm-status.sh
```

Expected result: `/api/v1/system/info` reports the current npm latest version,
and `npm list -g --prefix "$TEST_HOME/npm-global" kandev --depth=0` reports the
same version.

Clean up:

```
scripts/tmp-self-update/real-npm-teardown.sh
```

## Knobs

All scripts use:

```
TEST_HOME="${TEST_HOME:-$HOME/.kandev-selfupdate-test}"
```

The real npm scripts default to:

```
TEST_HOME="${TEST_HOME:-$HOME/.kandev-selfupdate-real-npm}"
KANDEV_TEST_NPM_PREFIX="${KANDEV_TEST_NPM_PREFIX:-$TEST_HOME/npm-global}"
```

Override it by exporting `TEST_HOME` before running any script.

Other useful overrides:

- `KANDEV_TEST_CURRENT_VERSION=0.53.0`
- `KANDEV_TEST_TARGET_VERSION=0.54.0`
- `KANDEV_TEST_TARGET_TAG=v99.0.0`
- `KANDEV_TEST_NPM_PREFIX="$TEST_HOME/npm-global"`
- `KANDEV_TEST_PORT=38429`
- `KANDEV_TEST_WEB_PORT=37429`
- `KANDEV_TEST_MANAGER=systemd` or `launchd`
- `KANDEV_TEST_KIND=npm`, `npx`, or `homebrew`

Clean up:

```
scripts/tmp-self-update/teardown.sh
```
