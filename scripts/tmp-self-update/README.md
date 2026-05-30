# Temporary self-update manual test scripts

Temporary helpers for testing service-gated UI self-update from a source checkout.
Remove this directory after the manual test pass.

## Quick path

```
scripts/tmp-self-update/build-local.sh
scripts/tmp-self-update/setup-fake-service.sh
scripts/tmp-self-update/run-backend.sh
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

## Knobs

All scripts use:

```
TEST_HOME="${TEST_HOME:-$HOME/.kandev-selfupdate-test}"
```

Override it by exporting `TEST_HOME` before running any script.

Other useful overrides:

- `KANDEV_TEST_CURRENT_VERSION=v0.53.0`
- `KANDEV_TEST_TARGET_TAG=v99.0.0`
- `KANDEV_TEST_PORT=38429`
- `KANDEV_TEST_MANAGER=systemd` or `launchd`
- `KANDEV_TEST_KIND=npm`, `npx`, or `homebrew`

Clean up:

```
scripts/tmp-self-update/teardown.sh
```
