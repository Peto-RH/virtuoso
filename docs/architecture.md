# Architecture

Virtuoso is one Go binary. Each `run` invocation collects and reports once,
then exits. The systemd timer provides periodic execution.

## Components

| Location | Responsibility |
| --- | --- |
| `cmd/virtuoso` | Parse CLI options and coordinate `run` and `status`. |
| `internal/config` | Load TOML and validate the source and, when required, the destination. |
| `internal/provider` | Connect to libvirt or VMware, check source access, and collect host and guest data. |
| `internal/report` | Represent hosts-guests report in a shared internal model. |
| `internal/destination/candlepin` | Format mappings, authenticate to Candlepin, submit requests, and poll jobs. |
| `data/systemd` | Define the oneshot service and the timer that starts it. |

## Reporting flow

The `[source]` configuration tells Virtuoso which backend to connect to and where
to find it.

```text
Read inventory from libvirt or VMware
        |
        v
Build shared report (hosts and guests)
        |
        +-- run --print --> Format Candlepin mapping --> JSON to stdout
        |
        `-- run --> Candlepin client
                         |
                         +-- Hosts found --> Format Candlepin mapping
                         |                           |
                         |                           v
                         |                    HTTPS submission
                         |
                         `-- Zero hosts ----> HTTPS heartbeat
```

Regular runs submit all collected hosts in one request, including hosts with
zero guests. A heartbeat refreshes check-in timestamps for previously reported
hosts. Both requests use a reporter ID derived from the reporting machine's
hostname and machine ID, with a hostname fallback, and the client waits for
the resulting asynchronous job.

`status` follows a separate path. It checks the source connection and then
uses a heartbeat request with the `status_test` reporter ID to check
destination access.

## Source mapping

- **Libvirt** opens a read-only connection. It reports the connection's
  hostname as the hypervisor ID and name, and includes active and inactive
  domains by UUID.
- **VMware** uses govmomi over HTTPS and discovers eligible hosts across the
  accessible inventory. It identifies each host by its hardware UUID and
  derives its name from DNS configuration. Hosts with either property missing
  are skipped. Each host's VM references determine membership, including
  powered-off guests. Guests use their BIOS UUIDs.

The shared report model keeps provider API objects and Candlepin request types
at their respective boundaries. This lets both sources use the same formatter
and submission flow.
