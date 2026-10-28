package provider

import (
	"context"
	"crypto/x509"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Peto-RH/virtuoso/internal/config"
	"github.com/Peto-RH/virtuoso/internal/report"
	"github.com/google/uuid"
	"github.com/vmware/govmomi/property"
	"github.com/vmware/govmomi/session"
	"github.com/vmware/govmomi/view"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/soap"
	"github.com/vmware/govmomi/vim25/types"
)

const (
	vmwarePasswordCredentialName = "virtuoso.vmware.password"
	vmwareCleanupTimeout         = 5 * time.Second
)

type VMwareProvider struct {
	vimClient      *vim25.Client
	sessionManager *session.Manager
}

func NewVMwareProvider(ctx context.Context, sourceConfig *config.SourceConfig) (*VMwareProvider, error) {
	endpoint, err := url.Parse(sourceConfig.URI)
	if err != nil {
		return nil, fmt.Errorf("VMware source.uri must be a valid absolute HTTPS URI")
	}
	if !endpoint.IsAbs() || endpoint.Scheme != "https" || endpoint.Hostname() == "" {
		return nil, fmt.Errorf("VMware source.uri must be an absolute HTTPS URI with a hostname")
	}
	if endpoint.User != nil {
		return nil, fmt.Errorf("VMware source.uri must not contain credentials; use source.username and the runtime credential")
	}
	if sourceConfig.Username == "" {
		return nil, fmt.Errorf("VMware source.username is required")
	}

	password, err := readVMwarePassword()
	if err != nil {
		return nil, err
	}

	soapClient, err := newVMwareSOAPClient(endpoint, sourceConfig.CAFile)
	if err != nil {
		return nil, err
	}
	loginSucceeded := false
	defer func() {
		if !loginSucceeded {
			soapClient.CloseIdleConnections()
		}
	}()
	slog.Debug("connecting to VMware", "host", endpoint.Hostname())
	vimClient, err := vim25.NewClient(ctx, soapClient)
	if err != nil {
		return nil, fmt.Errorf("cannot connect to VMware: %w", err)
	}
	if vimClient.ServiceContent.SessionManager == nil {
		return nil, fmt.Errorf("VMware service has no session manager")
	}
	provider := &VMwareProvider{
		vimClient:      vimClient,
		sessionManager: session.NewManager(vimClient),
	}
	if err := provider.sessionManager.Login(ctx, url.UserPassword(sourceConfig.Username, password)); err != nil {
		logVMwareSessionFailure("login", err)
		if soapClient.SessionCookie() != nil {
			if cleanupErr := provider.Close(); cleanupErr != nil {
				slog.Warn("VMware session cleanup failed", "err", cleanupErr)
			}
		}
		// A login fault can echo the password. Keep only safe context errors.
		if ctx.Err() != nil {
			return nil, fmt.Errorf("VMware login failed: %w", ctx.Err())
		}
		return nil, fmt.Errorf("VMware login failed")
	}
	loginSucceeded = true
	return provider, nil
}

func newVMwareSOAPClient(endpoint *url.URL, caFile string) (*soap.Client, error) {
	var caPool *x509.CertPool
	if caFile != "" {
		caPEM, err := os.ReadFile(caFile)
		if err != nil {
			return nil, fmt.Errorf("cannot read VMware source.ca_file: %w", err)
		}
		caPool = x509.NewCertPool()
		if !caPool.AppendCertsFromPEM(caPEM) {
			return nil, fmt.Errorf("VMware source.ca_file contains no usable PEM certificates")
		}
	}

	soapClient := soap.NewClient(endpoint, false)
	transport := soapClient.DefaultTransport()
	transport.TLSClientConfig.RootCAs = caPool
	// Govmomi v0.52.0 uses tls.Dial without its context; restore the standard
	// transport's cancellation and TLS handshake timeout handling.
	transport.DialTLSContext = nil
	soapClient.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return soapClient, nil
}

func readVMwarePassword() (string, error) {
	credentialsDir := os.Getenv("CREDENTIALS_DIRECTORY")
	if credentialsDir == "" {
		return "", fmt.Errorf("CREDENTIALS_DIRECTORY is required for VMware credential %s", vmwarePasswordCredentialName)
	}
	passwordBytes, err := os.ReadFile(filepath.Join(credentialsDir, vmwarePasswordCredentialName))
	if err != nil {
		return "", fmt.Errorf("cannot read VMware credential %s: %w", vmwarePasswordCredentialName, err)
	}
	password, hadCRLF := strings.CutSuffix(string(passwordBytes), "\r\n")
	if !hadCRLF {
		password = strings.TrimSuffix(password, "\n")
	}
	if password == "" {
		return "", fmt.Errorf("VMware credential %s is empty", vmwarePasswordCredentialName)
	}
	return password, nil
}

func (p *VMwareProvider) Collect(ctx context.Context) ([]report.Hypervisor, error) {
	hosts, err := p.retrieveHosts(ctx)
	if err != nil {
		return nil, err
	}

	eligibleHosts, vmRefs := selectVMwareHostsAndVMRefs(hosts)
	vmsByRef, err := p.retrieveVMsByRef(ctx, vmRefs)
	if err != nil {
		return nil, err
	}

	return hypervisorsFromVMwareHosts(eligibleHosts, vmsByRef), nil
}

func (p *VMwareProvider) retrieveHosts(ctx context.Context) ([]mo.HostSystem, error) {
	if p.vimClient.ServiceContent.ViewManager == nil {
		return nil, fmt.Errorf("VMware service has no view manager")
	}
	hostView, err := view.NewManager(p.vimClient).CreateContainerView(ctx, p.vimClient.ServiceContent.RootFolder, []string{"HostSystem"}, true)
	if err != nil {
		return nil, fmt.Errorf("cannot create VMware host view: %w", err)
	}
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), vmwareCleanupTimeout)
		defer cancel()
		if err := hostView.Destroy(cleanupCtx); err != nil {
			slog.Warn("cannot destroy VMware host view", "view", hostView.Reference().Value)
		}
	}()

	var hosts []mo.HostSystem
	if err := hostView.Retrieve(ctx, []string{"HostSystem"}, []string{"hardware.systemInfo.uuid", "config.network.dnsConfig", "vm"}, &hosts); err != nil {
		return nil, fmt.Errorf("cannot retrieve VMware hosts: %w", err)
	}
	return hosts, nil
}

func (p *VMwareProvider) retrieveVMsByRef(ctx context.Context, vmRefs []types.ManagedObjectReference) (map[types.ManagedObjectReference]mo.VirtualMachine, error) {
	var vms []mo.VirtualMachine
	// Govmomi rejects empty reference lists, even though hosts without VMs are valid.
	if len(vmRefs) != 0 {
		if err := property.DefaultCollector(p.vimClient).Retrieve(ctx, vmRefs, []string{"config.uuid", "runtime.powerState"}, &vms); err != nil {
			return nil, fmt.Errorf("cannot retrieve VMware guests: %w", err)
		}
	}
	vmsByRef := make(map[types.ManagedObjectReference]mo.VirtualMachine, len(vms))
	for _, vm := range vms {
		vmsByRef[vm.Reference()] = vm
	}
	return vmsByRef, nil
}

func selectVMwareHostsAndVMRefs(hosts []mo.HostSystem) ([]mo.HostSystem, []types.ManagedObjectReference) {
	eligibleHosts := make([]mo.HostSystem, 0, len(hosts))
	var vmRefs []types.ManagedObjectReference
	for _, host := range hosts {
		if host.Hardware == nil || strings.TrimSpace(host.Hardware.SystemInfo.Uuid) == "" {
			slog.Warn("skipping VMware host without hardware UUID", "host", host.Reference().Value)
			continue
		}
		if host.Config == nil || host.Config.Network == nil || host.Config.Network.DnsConfig == nil {
			slog.Warn("skipping VMware host without DNS configuration", "host", host.Reference().Value)
			continue
		}
		eligibleHosts = append(eligibleHosts, host)
		vmRefs = append(vmRefs, host.Vm...)
	}
	return eligibleHosts, vmRefs
}

func hypervisorsFromVMwareHosts(eligibleHosts []mo.HostSystem, vmsByRef map[types.ManagedObjectReference]mo.VirtualMachine) []report.Hypervisor {
	hypervisors := make([]report.Hypervisor, 0, len(eligibleHosts))
	for _, host := range eligibleHosts {
		dnsConfig := host.Config.Network.DnsConfig.GetHostDnsConfig()
		hostname := dnsConfig.HostName
		if dnsConfig.DomainName != "" {
			hostname += "." + dnsConfig.DomainName
		}
		hypervisors = append(hypervisors, report.Hypervisor{
			Source: "esx",
			ID:     host.Hardware.SystemInfo.Uuid,
			Name:   hostname,
			Guests: guestsForVMwareHost(host, vmsByRef),
		})
	}
	return hypervisors
}

func guestsForVMwareHost(host mo.HostSystem, vmsByRef map[types.ManagedObjectReference]mo.VirtualMachine) []report.Guest {
	guests := make([]report.Guest, 0, len(host.Vm))
	// HostSystem.vm is the sole source of host/guest associations.
	for _, vmRef := range host.Vm {
		vm := vmsByRef[vmRef]
		if vm.Config == nil {
			slog.Warn("skipping unavailable VMware guest configuration", "host", host.Reference().Value, "guest", vmRef.Value)
			continue
		}
		guestID, err := uuid.Parse(strings.TrimSpace(vm.Config.Uuid))
		if err != nil {
			slog.Warn("skipping VMware guest with missing or invalid BIOS UUID", "host", host.Reference().Value, "guest", vmRef.Value)
			continue
		}
		guests = append(guests, report.Guest{
			ID:    guestID.String(),
			State: guestStateFromVMware(vm.Runtime.PowerState),
		})
	}
	return guests
}

func guestStateFromVMware(state types.VirtualMachinePowerState) report.GuestState {
	switch state {
	case types.VirtualMachinePowerStatePoweredOn:
		return report.GuestStateRunning
	case types.VirtualMachinePowerStateSuspended:
		return report.GuestStatePaused
	case types.VirtualMachinePowerStatePoweredOff:
		return report.GuestStateShutoff
	default:
		return report.GuestStateUnknown
	}
}

func (p *VMwareProvider) Ping(ctx context.Context) error {
	userSession, err := p.sessionManager.UserSession(ctx)
	if err != nil {
		return fmt.Errorf("VMware session check failed: %w", err)
	}
	if userSession == nil {
		return fmt.Errorf("VMware session is not authenticated")
	}
	return nil
}

func (p *VMwareProvider) Close() error {
	defer p.vimClient.CloseIdleConnections()
	ctx, cancel := context.WithTimeout(context.Background(), vmwareCleanupTimeout)
	defer cancel()
	if err := p.sessionManager.Logout(ctx); err != nil {
		logVMwareSessionFailure("logout", err)
		if ctx.Err() != nil {
			return fmt.Errorf("VMware logout failed: %w", ctx.Err())
		}
		return fmt.Errorf("VMware logout failed")
	}
	return nil
}

func logVMwareSessionFailure(operation string, err error) {
	// Server fault text can echo credentials, including during failed-login cleanup.
	errorType := fmt.Sprintf("%T", err)
	if soap.IsSoapFault(err) {
		if fault := soap.ToSoapFault(err).VimFault(); fault != nil {
			errorType = fmt.Sprintf("%T", fault)
		}
	}
	slog.Debug("VMware session operation failed", "operation", operation, "error_type", errorType)
}
