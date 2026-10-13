# TASK-009 Performance Report

## Summary

TASK-009's comparison budget, visible-paste path, line-height cache, and
benchmark fixtures were measured on the documented Linux reference profile. All
measured target gates passed: the 1 MiB paste benchmark stayed below 250 ms,
adversarial comparisons either completed or returned the coarse fallback before
2 s, and the 10 MiB input fixture stayed well below the 512 MiB RSS ceiling.

The numbers below are engineering measurements from five benchmark repetitions
(`-count=5`, `-benchtime=1x`), not statistical claims about every terminal or
machine. The implementation report deliberately separates the paste-to-visible
measurement from comparison latency.

## Reference environment

- Container image: Ubuntu 24.04.

- Architecture: Linux/amd64.

- Go: `go1.26.8 linux/amd64`.

- CPU: Docker `--cpus=4`, `GOMAXPROCS=4`; the host reports an AMD Ryzen 7 5700U
  with Radeon Graphics and 16 logical CPUs. This is a four-CPU cgroup quota on
  that host, not a separate four-vCPU reference VM.

- Memory: Docker `--memory=8g`.

- Host kernel and memory are those of the local NixOS Docker host; Docker
  enforces the stated container CPU and memory limits.

- Terminal: paste-to-visible uses Micro's `tcell` simulation screen at 100 by 30
  cells. It measures paste handling plus a draw, without a physical terminal
  emulator or terminal transport latency. Comparison and viewport benchmarks are
  non-interactive.

- Benchmark commands:

  ```sh
  go test -run '^$' -bench='^(BenchmarkAlign|BenchmarkExpand|BenchmarkPaste|BenchmarkCached)' -benchtime=1x -benchmem -count=5 -v ./internal/compare ./internal/viewport ./internal/prototype
  go test -run '^$' -bench='^BenchmarkRSSMemoryGate$' -benchtime=1x -benchmem -count=5 -v ./internal/compare
  ```

The repository's `just bench` recipe runs these commands and writes output to
`/tmp/mdiff-bench.txt` by default. Set `MDIFF_BENCH_OUT` to choose another
external report path.

## Results

Times are medians of five runs, with the observed range shown. Allocation
columns are per operation. RSS is maximum observed process RSS from the five RSS
fixture runs, measured with the platform-specific helper; the Linux report uses
`getrusage(RUSAGE_SELF).ru_maxrss`.

| Workload                                | Exact input                                     |                Median time (range) |           Allocated per operation | Outcome                               |
| --------------------------------------- | ----------------------------------------------- | ---------------------------------: | --------------------------------: | ------------------------------------- |
| Near-equal 10k lines                    | 10,000 / 10,000 lines; 280,008 bytes            |             3.92 ms (3.75–5.71 ms) |               2.63 MB; 150 allocs | Exact diff                            |
| Near-equal 100k lines, one changed line | 100,000 / 100,000 lines; 2,800,008 bytes        |          75.76 ms (72.13–78.11 ms) |          27.80 MB; 629–630 allocs | Exact diff                            |
| Large replacement                       | 50,000 / 50,000 lines; 1,300,000 bytes          |            1.559 s (1.555–1.564 s) |          24.53 MB; 613–614 allocs | Coarse fallback in every run          |
| Repeated-line insertion                 | 100,000 / 100,001 lines; 1,400,008 bytes        |          22.90 ms (22.31–23.81 ms) |            10.26 MB; 61–62 allocs | Exact aligned insertion               |
| Unique reverse order                    | 50,000 / 50,000 lines; 2,700,000 bytes          |            1.537 s (1.530–1.542 s) |              17.78 MB; 353 allocs | Coarse fallback in every run          |
| Long-line middle change                 | 1 / 1 lines; 2,097,154 bytes total              |          31.84 ms (28.61–44.62 ms) |            11.57 MB; 31–32 allocs | Exact character diff                  |
| Unequal wrap-width viewport expansion   | 50,000 / 50,000 lines; 10,485,760 bytes         |       155.98 ms (136.39–203.33 ms) |   125.73 MB; about 302,732 allocs | Completed                             |
| 1 MiB paste to visible draw             | 1,048,576 ASCII bytes into an empty pane        |       213.23 ms (210.42–217.11 ms) | 9.82–9.87 MB; about 20,150 allocs | Under 250 ms target                   |
| Cached 10k line heights                 | 10,000 lines; 189,999 bytes                     |             1.88 ms (1.85–2.01 ms) |     0 B in 4/5 runs; 112 B in one | Warm-cache pass                       |
| RSS gate, near-equal fixture            | 50,000 / 50,000 lines; exactly 10,485,760 bytes | 107.48 ms median (99.28–119.66 ms) |    55.31 MB; about 302,690 allocs | Peak RSS 53,301,248 bytes (50.83 MiB) |

The two adversarial fallback results are measured by `AlignWithBudget`, which
uses the production 1.5 s comparison budget and returns a usable coarse changed
region. Observed wall-clock durations include fixture comparison and fallback
construction overhead and remained below the 2 s worker-start target. The
coordinator installs the result only when its buffer versions still match; its
queue retains at most one active and one latest pending snapshot pair.

After the TASK-009 review fixes, the full `just bench` suite was rerun locally
with `GOMAXPROCS=4`. On the same Ryzen 7 5700U host, the 1 MiB paste-to-visible
median was 213.82 ms (210.81–221.92 ms); the large-replacement and reverse-order
fallbacks remained below 1.562 s; and the RSS fixture's maximum observed RSS was
52,940,800 bytes (50.49 MiB). This supplemental run includes the buffer-cache
reset and status-line divider changes, but it is not the constrained Ubuntu
container run reported in the table above.

The RSS fixture is `rss-10mib-100k-balanced-near-equal-v1`, generated by the
versioned deterministic fixture in `internal/perfworkload`. Each side has 50,000
LF-terminated lines and 5 MiB including newlines. Lines 0–42,879 have 104
payload bytes; lines 42,880–49,999 have 103 payload bytes. Payloads start with
`L%05d:` and use the specified repeating lowercase fill. The right pane changes
one payload byte on each line divisible by 1,000: 50 one-byte changed lines, no
line insertions or deletions. Peak RSS was 50.83 MiB, about one tenth of the 512
MiB limit.

The adversarial generators are pinned in code and log fixture and generator
versions with their exact line and byte counts:

- `repeated-insertion-100k-v1`: 100,000 `repeat` lines against a 50,000-line
  prefix, one `changed` line, and a 50,000-line suffix.
- `unique-reverse-50k-seed-20261011-v1`: 50,000 fixed-width records using the
  next `Uint64` values from Go `math/rand.NewSource(20261011)`, compared with
  the records in reverse order.
- `single-1mib-line-middle-change-v1`: one 1 MiB ASCII line on each side with a
  single middle-byte change.

## Optimization evidence

The first 1 MiB paste profile during this task measured about 500 ms and
1,070,000 allocations in the same simulated-screen harness. CPU allocation
profiling showed `BufWindow.getStartInfo` calling syntax style lookup for each
skipped rune while calculating the cursor's visual position. The renderer also
expanded every wrapped row of a long line despite drawing only visible rows. The
changes now look up syntax style for the visible rune, cache Micro visual cursor
locations, and render/cache only requested visible wrapped rows. Together with
bulk line-array insertion and avoiding redundant syntax-rule reloads after each
edit, the final paste median is 213 ms and about 20,150 allocations. This is a
before/after measurement from the working implementation, not a separately
archived clean-revision baseline.

The retained per-line height cache avoids recomputing unchanged line heights
after an edit. It uses stable line IDs and per-line content versions, while
width, tab size, and wrap settings remain part of the cache key. Resize or
relevant configuration changes invalidate the geometry inputs. The measured warm
10k-line lookup takes about 1.88 ms with effectively no per-operation
allocation.

No incremental diff tree or bespoke aligner was added. The established `go-diff`
implementation remains the exact comparator; a shared finite budget with an
explicit coarse changed-region fallback handles adversarial cases.

## Caveats and reproduction

The benchmark workload is deterministic, but wall-clock results vary with CPU
frequency, kernel scheduling, Docker implementation, and host contention. The
CPU quota and memory limit match the reference profile; the host CPU is a mobile
Ryzen system, so the measurements should not be generalized to every four-vCPU
VM. The paste result measures application event handling and screen drawing in a
simulation, not actual terminal input delivery or terminal emulator refresh.

Reproduce from the repository root with Go 1.26.8 and Docker available:

```sh
GOMAXPROCS=4 MDIFF_BENCH_OUT=/tmp/mdiff-bench.txt just bench
```

For the constrained reference run, execute the benchmark commands above in
Ubuntu 24.04 with Docker limits `--cpus=4 --memory=8g`, Go 1.26.8, and
`GOMAXPROCS=4`. Keep generated benchmark output outside the checkout.

The measured container was created with the following setup. It installs the Go
patch release inside Ubuntu and writes the log to a mounted temporary directory;
normal Go module cache/network access is required on a fresh host. The recorded
pass additionally mounted the host's populated Go module cache read-only and set
`GOPROXY=off`; the snippet omits that machine-specific mount so another host can
fetch modules normally.

```sh
docker run --rm --cpus=4 --memory=8g \
  -v "$PWD:/work" -v /tmp/mdiff-task009-docker-cache:/cache \
  -v /tmp/mdiff-task009-docker-out:/out -w /work ubuntu:24.04 \
  bash -lc 'set -euo pipefail
    apt-get update
    apt-get install -y ca-certificates curl tar
    curl -fsSL https://go.dev/dl/go1.26.8.linux-amd64.tar.gz -o /tmp/go.tar.gz
    tar -C /usr/local -xzf /tmp/go.tar.gz
    export PATH=/usr/local/go/bin:/usr/bin:/bin
    export GOMODCACHE=/go/pkg/mod GOCACHE=/cache GOMAXPROCS=4
    mkdir -p /out
    go version
    go test -run "^$" -bench="^(BenchmarkAlign|BenchmarkExpand|BenchmarkPaste|BenchmarkCached)" -benchtime=1x -benchmem -count=5 -v ./internal/compare ./internal/viewport ./internal/prototype | tee /out/bench.txt
    go test -run "^$" -bench="^BenchmarkRSSMemoryGate$" -benchtime=1x -benchmem -count=5 -v ./internal/compare | tee -a /out/bench.txt'
```
