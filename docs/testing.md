# Testing

Choose the test layer by the boundary you need to verify. For test commands
and debugging, see the [development guide](development.md#testing).

## Go package tests

Untagged `*_test.go` files live alongside the production code in the same Go
package.  Package tests can range from small unit tests to broader tests of
interactions within that package.

Keep package tests deterministic and runnable with ordinary user permissions
and local test dependencies.

## Black-box Go component tests

Tests in `tests/component` use the `component` build tag and `TestComponent` prefix
names. The component test target supplies a compiled CLI through
`VIRTUOSO_COMPONENT_BINARY`. Most scenarios write a temporary TOML configuration file
and check the command’s exit status and output.

- Libvirt's test driver reads an XML fixture to exercise guest collection and
  report output without a running libvirt service.
- The local govmomi HTTPS simulator exercises VMware authentication,
  certificate trust, and host-to-guest reporting across simulated inventory.
  A temporary `CREDENTIALS_DIRECTORY` supplies the runtime password file that
  systemd normally delivers.

## Integration tests (end to end) - not implemented yet

The Python end-to-end layer targets an installed RPM in a production-like
environment. Its scope includes systemd activation, service-account and SELinux
access, encrypted credential delivery and cleanup, and reporting against real
hypervisors and a real destination. The RPM CI workflow also runs packaging
smoke checks for configuration permissions and CLI startup.
