# Turn changed-files performance results

## Environment

Benchmarks ran on Debian 13 with Linux 7.0.14, ext4, an AMD Ryzen 5 7640HS, and 11 reported CPUs.
The versions were Go 1.26.0, Git 2.47.3, and Docker 26.1.5.
The Git benchmarks used a local `GitOperator` on the host. They did not use a remote executor or a database.

Command rerun on 2026-10-08:

```bash
go test -trimpath ./internal/agentctl/server/process ./internal/task/changes -run '^$' -bench 'TurnCheckpoint|TurnChangeExport' -benchtime=10x -benchmem -v
```

Each benchmark created its fixture before timing. The reported median, p95, and maximum use the ten final per-operation observations. The first calibration observation is excluded. The p95 uses nearest-rank selection, so it equals the maximum for ten observations. All measured operations completed without error.

## Results

| Case | Fixture and operation | Median | p95 | Maximum | Other measured data |
| --- | --- | ---: | ---: | ---: | --- |
| 100 tracked files | One 24-byte tracked-file change, capture, compare, export | 168.750 ms | 182.548 ms | 182.548 ms | 490 exported bytes on eight of ten observations; 3.3 new Git objects and 13.2 KiB loose objects per operation |
| 20,000 tracked files | One 24-byte tracked-file change, capture, compare, export | 1,009.116 ms | 1,033.038 ms | 1,033.038 ms | 490 exported bytes on eight of ten observations; 4.0 new Git objects and 64.0 KiB loose objects per operation |
| Large text and binary | One changed 1 MiB text file and one changed 1 MiB binary file | 330.483 ms | 385.571 ms | 385.571 ms | 2,098,328 exported bytes on eight of ten observations; 4.4 new Git objects and 1,046 KiB loose objects per operation |
| Committed change | Capture a 100-file repository, modify and commit one tracked file, capture and export | 190.455 ms | 197.166 ms | 197.166 ms | 396 exported bytes on eight of ten observations |
| Two checkouts | Capture, compare, and export one change in each of two 100-file repositories, sequentially | 412.601 ms | 526.406 ms | 526.406 ms | 798 exported bytes across both checkouts on eight of ten observations |
| Shared checkout contention | Two concurrent captures through separate operators against one 100-file repository | 116.442 ms | 137.676 ms | 137.676 ms | Two captures per operation; no compare or export |

Git object growth comes from `git count-objects -v` before and after the timed loop. This value measures loose Git objects. It is not the size of compressed database retention.

## Limits

These results measure local Git capture, comparison, and export. They exclude lifecycle RPC, executor transport, SQL writes, content compression, and database retention. The two-checkout case runs its checkouts in sequence.

This run did not measure cold-cache behavior, slow storage, size-limit failures, retention growth across database-backed turns, or Stop and Cancel deadlines. SSH, Kubernetes, Sprites, and plugin executors need separate qualification. No remote-executor performance claim follows from these local measurements.
