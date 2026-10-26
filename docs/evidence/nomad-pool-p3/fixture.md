# Two-node pool fixture (P3)

Two Nomad clients in node pool `kandev` against the same throwaway server: the
existing `ssh` client and one new `local` client on the same machine as Kandev.
Not a multi-machine fleet; the boundary is recorded in each run's evidence.

## Local client

Pick a fixture directory `$FX` (outside the Kandev home the runner creates).

```bash
# 1. Host identity token at a fixed fixture path, and the client meta block.
KANDEV_HOME_DIR="$FX/local-identity" scripts/nomad-pool-onboard local "$FX/local-state"
chmod 600 "$FX/local-identity/data/nomad-host-identity"
```

The command prints the `client { meta ... reserved ... }` block. Put it in a
second client config, with its own name, data dir and ports:

```hcl
name      = "kandev-local"
data_dir  = "<FX>/local-client/data"
bind_addr = "127.0.0.1"
ports { http = 4747 }

client {
  enabled         = true
  servers         = ["127.0.0.1:4647"]   # the fixture server's RPC address
  node_pool       = "kandev"
  alloc_dir       = "<FX>/local-client/alloc"
  meta {
    "kandev.attach"               = "local"
    "kandev.state_root"           = "<FX>/local-state"
    "kandev.host_identity_sha256" = "<printed digest>"
  }
  reserved { cpu = 1000  memory = 2048 }
}
# Apple Silicon: Nomad fingerprints ~48 MHz, below the reservation. Pin the total
# (top level of the client config) and keep reserved well under it:
# cpu_total_compute = 4000
plugin "raw_exec" { config { enabled = true } }
```

Start it as a second `nomad agent -client -config=...` and wait until
`nomad node status` lists both nodes ready and eligible in pool `kandev`.

## Why the token path matters

The runner starts a fresh Kandev home per run, so a token under `~/.kandev`
is not the one the run's backend hashes. Set `pool.host_identity_token_file` in
the runner config to the token above; the runner copies it into the run home's
`data/nomad-host-identity` (mode 600), so the digest in the node's metadata
matches. The local `kandev.state_root` must lie outside the run home.

## Runner config

Add to the existing `pool` block: `"host_identity_token_file": "<FX>/local-identity/data/nomad-host-identity"`.
The fixture repository must be a local path (`repository.path`) so the local
node can add a git worktree of it.

## Scenarios beyond placement

`pool-queue`, `pool-partition` and `pool-refusals` replace the manual procedures; each is one
invocation of `scripts/nomad-native-e2e run --require-real --create-fixture --scenario <name>`.

- `pool-queue` sizes the profile memory per run so each node holds one environment. No extra setup.
- `pool-partition` needs an `ssh_client` block (`pid`, `config`) in the runner config naming the
  ssh node's Nomad client agent process and its config file. The runner verifies the pid's
  command line (a `nomad agent` process started with that config path) before every signal,
  freezes it with SIGSTOP, waits for the ssh node to leave `ready`, and always sends SIGCONT
  afterwards, including on failure. The ssh node's raw_exec allocations run under a separate
  executor process and keep running while the agent is frozen. A relay in front of the server
  RPC cannot isolate one client on a one-server fixture, so the relay stays only for the
  single-node WO06 disconnect scenario. The run sends one inert prompt to the local
  environment while the ssh agent is frozen.
- `pool-refusals` overrides the ssh node's `kandev.ssh_host_key_sha256` through Nomad dynamic node
  metadata (Nomad 1.5 or later; the runner's token needs `node:write`), and swaps the run-home
  host identity token (read on every launch, so no backend restart). It needs one eligible local
  and one eligible ssh node, and `pool.host_identity_token_file`. Metadata, per-node eligibility
  and the token are restored in teardown and on failure.
