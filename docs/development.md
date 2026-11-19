# Development

## Build and run locally

Build with Go, a C toolchain with cgo enabled, `pkg-config`, and libvirt
development headers. The Makefile also uses `rpmspec` to read the package
version. Check the required Go version in [go.mod](../go.mod).

```sh
make build
./bin/virtuoso --help
./bin/virtuoso --config config/virtuoso.toml --log-level debug run --print
```

The sample configuration connects to local libvirt at `qemu:///system`. To
try collection without a running libvirt daemon, copy the sample and set
`source.uri` to `test:///default`, then pass that copy to `--config`.
Use `status` with a configured source and destination to check both connections.

For VMware, use [govmomi’s `vcsim`](https://github.com/vmware/govmomi/blob/v0.52.0/vcsim/README.md)
to run a mock vCenter locally with simulated hosts and VMs.

## Testing

From the repository root, run `make test` for the Go package tests and
`make component-test` for the balck-box component tests. Run `make lint` after
you make any code changes. See [Testing](testing.md) for the scope of each suite.

For a focused test run, enable verbose output and select a test by name. The
component tests need the built binary's absolute path.

```sh
go test -v ./internal/config -run TestLoadConfig
make build
VIRTUOSO_COMPONENT_BINARY="$PWD/bin/virtuoso" \
    go test -v -count=1 -tags=component ./tests/component -run VMware
```

## Build an RPM

For an RPM build, `make rpm` archives committed `HEAD` and writes the build
output under `rpmbuild/`. Use the local binary to test working-tree application
changes. RPM builds also need `rpmbuild` and the build dependencies in
[virtuoso.spec](../virtuoso.spec).

## Debug the systemd service

Use a drop-in to enable debug logging with the packaged service's user, groups,
and credential settings. Run `sudo systemctl edit virtuoso.service` and enter
the following. The empty `ExecStart=` clears the packaged command before setting
the replacement.

```ini
[Service]
ExecStart=
ExecStart=/usr/bin/virtuoso --log-level debug run
```

This works for both libvirt and VMware. To preview the JSON report and skip
submission to Candlepin, append `--print` to the replacement `ExecStart` command.
The packaged service writes the JSON to the journal.

Start the service and inspect its journal output.

```sh
run_started_at=$(date '+%Y-%m-%d %H:%M:%S')
sudo systemctl start virtuoso.service
sudo journalctl -u virtuoso.service --since "$run_started_at" --no-pager
```

The override also applies to runs started by `virtuoso.timer`. Remove it and
run `sudo systemctl daemon-reload` when you are done.
