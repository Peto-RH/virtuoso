# Virtuoso

Virtuoso is a Go reimplementation of virt-who. It collects associations between
hypervisors and guests from libvirt or VMware and reports them to Candlepin
(RHSM or Satellite).

**Work in progress:** this is not the final product state. Configuration,
behavior, and packaging may change.

## Quick start

The intended deployment is an RPM on RHEL 10 or later, running through systemd.
After installing the RPM, follow the [configuration guide](docs/configuration.md)
to select source and destination in `/etc/virtuoso/virtuoso.toml`.

```sh
sudo systemctl start virtuoso.service
sudo journalctl -u virtuoso.service
sudo systemctl enable --now virtuoso.timer
```

Each service activation collects and reports once, then exits. The timer runs
hourly with a randomized but fixed delay.

## Documentation

- [Configuration](docs/configuration.md) for options, examples, and credentials.
- [Architecture](docs/architecture.md) for components and reporting flow.
- [Development](docs/development.md) for building, running, and debugging.
- [Testing](docs/testing.md) for test scope and fixtures.
