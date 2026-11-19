# Configuration

Virtuoso reads TOML from `/etc/virtuoso/virtuoso.toml` by default. Use
`--config FILE` to select another file. Required values must be set explicitly.
`run --print` uses only `[source]`, while `run` and `status` require both
sections. See the [sample configuration](../config/virtuoso.toml).

## Source

Set one `[source]` section. Both `type` and `uri` are required.

```toml
[source]
type = "libvirt"
uri = "qemu:///system"
```

Virtuoso opens a read-only libvirt connection. The account running Virtuoso
needs access to the configured libvirt endpoint.

For VMware, use an absolute HTTPS URI for the final SOAP endpoint. The
`username` is required. Keep credentials out of the URI and the password out
of TOML. Virtuoso verifies the server certificate using system trust by default
and leaves redirects disabled. Set `ca_file` to a readable PEM CA bundle to
replace the system trust roots for VMware.

```toml
[source]
type = "vmware"
uri = "https://vcenter.example.com/sdk"
username = "virtuoso-reporter"
# ca_file = "/etc/virtuoso/vcenter-ca.pem"
```

For a private CA, place the PEM bundle at the configured path and give the
service account read access. For example, copy it to `/etc/virtuoso/vcenter-ca.pem`
as `root:virtuoso` with mode `0640` and enable `ca_file` above.

### VMware password

On the machine running Virtuoso, prompt for the password and encrypt it as a
systemd credential.

```sh
sudo install -d -o root -g root -m 0700 /etc/credstore.encrypted
systemd-ask-password -n "VMware password:" |
    sudo systemd-creds encrypt --name=virtuoso.vmware.password \
        - /etc/credstore.encrypted/virtuoso.vmware.password
```

The packaged service loads and decrypts the credential on each activation.
Virtuoso reads `$CREDENTIALS_DIRECTORY/virtuoso.vmware.password` and removes one
final LF or CRLF, preserving other whitespace. To preview a report with the
installed service, see [Debug the systemd service](development.md#debug-the-systemd-service).

## Destination

Set all four fields for your RHSM or Satellite deployment. The values below
show a typical RHSM endpoint. Replace `org_id` with the organization ID from
`subscription-manager identity`.

```toml
[destination]
org_id = "123456"
server = "subscription.rhsm.redhat.com"
port = 443
prefix = "/subscription"
```

| Option | Value |
| --- | --- |
| `org_id` | Organization ID as a string. |
| `server` | Hostname or IP address only. Use brackets around IPv6 addresses. |
| `port` | HTTPS port as an integer. |
| `prefix` | Candlepin base path starting with `/`. |

Candlepin uses mutual TLS with `/etc/pki/consumer/cert.pem` and
`/etc/pki/consumer/key.pem`. Virtuoso builds its Candlepin trust pool from
`/etc/rhsm/ca/*.pem` and requires at least one usable CA certificate. Ensure the
running account can read the required files. The packaged service runs as `virtuoso`
with the supplementary group `rhsm`.
