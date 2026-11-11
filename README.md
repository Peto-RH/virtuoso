# Virtuoso

Virtuoso is a Go reimplementation of virt-who. It collects associations between
hypervisors and guests from libvirt or VMware and reports them to Candlepin
(RHSM or Satellite).

**Work in progress:** this is not the final product state. Configuration,
behavior, and packaging may change.

## Quick start

The intended deployment is an RPM on RHEL 10 or later, running through systemd.
After installing the RPM, edit `/etc/virtuoso/virtuoso.toml`; see the
[sample configuration](config/virtuoso.toml). Choose a source below and set
`destination.org_id`, `server`, `port`, and `prefix` for your RHSM or Satellite
deployment.

Reporting requires the system's consumer certificate and key at
`/etc/pki/consumer/{cert,key}.pem` and CA certificates in `/etc/rhsm/ca/*.pem`.
The `virtuoso` service account must be able to read them.

```sh
sudo systemctl start virtuoso.service
sudo journalctl -u virtuoso.service
sudo systemctl enable --now virtuoso.timer
```

Each service activation collects and reports once, then exits. The timer runs
hourly with a randomized delay of up to one hour.

## Libvirt

```toml
[source]
type = "libvirt"
uri = "qemu:///system"
```

The packaged service runs as `virtuoso` with the `rhsm` supplementary group.
The adapter opens a read-only libvirt connection, which needs no group
membership under libvirt's default authorization. Ensure libvirt is available
at the configured URI.
When running the binary directly for debugging, it uses your current user,
which must also have permission to access libvirt.
No VMware password is needed.

## VMware

Replace the libvirt source with:

```toml
[source]
type = "vmware"
uri = "https://vcenter.example.com/sdk"
username = "virtuoso-reporter"
# ca_file = "/etc/virtuoso/vcenter-ca.pem"
```

Use the final HTTPS SOAP endpoint. Server certificate verification is required;
`ca_file` optionally supplies a PEM CA bundle instead of system trust.
If needed, copy your CA certificate and enable `ca_file` above:

```sh
sudo install -o root -g virtuoso -m 0640 \
    /path/to/ca.crt /etc/virtuoso/vcenter-ca.pem
```

Provision the password on the machine running Virtuoso:

```sh
sudo install -d -o root -g root -m 0700 /etc/credstore.encrypted
systemd-ask-password -n "VMware password:" |
    sudo systemd-creds encrypt - /etc/credstore.encrypted/virtuoso.vmware.password
```

The packaged service loads and decrypts this credential on each activation.
Keep the password out of TOML and the URI. Running the binary directly does not
decrypt it; use the VMware debugging command below.

## Binary debugging

You can also run the binary manually. Building requires Go 1.23 or later, a C
toolchain, `pkg-config`, libvirt development headers, and `rpmspec`.

```sh
make build
./bin/virtuoso --config config/virtuoso.toml --log-level debug run --print
./bin/virtuoso --config config/virtuoso.toml status
```

`run --print` collects and prints JSON without contacting the destination.
`status` checks source and destination access. The caller needs access to the
configured source and, for `status` or reporting, the consumer credentials.

For a VMware print run with the installed binary, let systemd deliver the password:

```sh
sudo systemd-run --wait --pipe --collect \
    --property=User=virtuoso --property=Group=virtuoso \
    --property=LoadCredentialEncrypted=virtuoso.vmware.password \
    /usr/bin/virtuoso --log-level debug run --print
```

To build the RPM, use `make rpm` with the dependencies in
[virtuoso.spec](virtuoso.spec). It packages committed `HEAD` only.
