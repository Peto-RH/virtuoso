//go:build component

package component_test

import (
	"crypto/tls"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vmware/govmomi/simulator"
	"github.com/vmware/govmomi/vim25/types"
)

const (
	simulatorPassword            = "correct-password"
	vmwarePasswordCredentialName = "virtuoso.vmware.password"
	withSimulatorCA              = true
	withoutSimulatorCA           = false
)

func TestComponentVMwareRunPrint(t *testing.T) {
	model := newVMwareComponentModel(t, 1, 2, 3)

	hosts := model.Map().All("HostSystem")
	vms := model.Map().All("VirtualMachine")
	if len(hosts) != 2 || len(vms) != 3 {
		t.Fatalf("simulator has %d hosts and %d VMs, want 2 hosts and 3 VMs", len(hosts), len(vms))
	}

	guestIDs := []string{
		"11111111-1111-1111-1111-111111111111",
		"22222222-2222-2222-2222-222222222222",
	}
	for i := range hosts {
		host := hosts[i].(*simulator.HostSystem)
		vm := vms[i].(*simulator.VirtualMachine)
		hostName := fmt.Sprintf("esxi-%d", i+1)
		host.Name = fmt.Sprintf("inventory-host-%d", i+1)
		host.Summary.Config.Name = host.Name
		host.Config.Network.DnsConfig = &types.HostDnsConfig{
			HostName:   hostName,
			DomainName: "example.test",
		}
		host.Vm = []types.ManagedObjectReference{vm.Reference()}
		vm.Config.Uuid = guestIDs[i]
		// Simulator summaries are independent values; keep both API views consistent.
		vm.Summary.Config.Uuid = vm.Config.Uuid
	}

	// This VM remains in the inventory but is not listed in any host's VM list.
	// Only VMs listed by a host belong in that host's report.
	unlistedVM := vms[2].(*simulator.VirtualMachine)
	unlistedVM.Config.Uuid = "33333333-3333-3333-3333-333333333333"
	unlistedVM.Summary.Config.Uuid = unlistedVM.Config.Uuid

	configPath := setupVMwareComponent(t, model, simulatorPassword+"\n", withSimulatorCA)
	stdout, stderr, err := runComponentBinary(t, configPath, "--log-level", "debug", "run", "--print")
	if err != nil {
		t.Fatalf("virtuoso run --print error = %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}
	output := string(stdout) + string(stderr)
	if strings.Contains(output, simulatorPassword) {
		t.Error("virtuoso printed the VMware password")
	}
	mapping := decodePrintedMapping(t, stdout)
	if len(mapping.Hypervisors) != 2 {
		t.Fatalf("printed hypervisor count = %d, want 2", len(mapping.Hypervisors))
	}

	wantGuests := map[string]string{
		"esxi-1.example.test": guestIDs[0],
		"esxi-2.example.test": guestIDs[1],
	}
	seenHostIDs := make(map[string]bool)
	for _, host := range mapping.Hypervisors {
		if host.HypervisorID.HypervisorID == "" || seenHostIDs[host.HypervisorID.HypervisorID] {
			t.Errorf("printed empty or duplicate hypervisor ID %q", host.HypervisorID.HypervisorID)
		}
		seenHostIDs[host.HypervisorID.HypervisorID] = true
		want, ok := wantGuests[host.Name]
		if !ok {
			t.Errorf("printed unexpected hypervisor name %q", host.Name)
			continue
		}
		if len(host.Guests) != 1 {
			t.Errorf("hypervisor %q has %d guests, want 1", host.Name, len(host.Guests))
			continue
		}
		guest := host.Guests[0]
		if guest.GuestID != want {
			t.Errorf("hypervisor %q guest ID = %q, want %q", host.Name, guest.GuestID, want)
		}
		delete(wantGuests, host.Name)
	}
	for name := range wantGuests {
		t.Errorf("printed mapping does not contain hypervisor %q", name)
	}
}

func TestComponentVMwareIdentifiersAndPowerStates(t *testing.T) {
	model := newVMwareComponentModel(t, 1, 1, 7)
	host := model.Map().Any("HostSystem").(*simulator.HostSystem)
	host.Hardware.SystemInfo.Uuid = "123e4567-e89b-12d3-a456-426614174000"
	host.Summary.Hardware.Uuid = host.Hardware.SystemInfo.Uuid
	vms := model.Map().All("VirtualMachine")
	cases := []struct {
		configUUID   string
		instanceUUID string
		wantGuestID  string
		powerState   types.VirtualMachinePowerState
		state        int
		active       int
	}{
		{" 01234567-89AB-CDEF-1234-567890ABCDEF ", "00000000-0000-0000-0000-000000000001", "01234567-89ab-cdef-1234-567890abcdef", types.VirtualMachinePowerStatePoweredOn, 1, 1},
		{"42231ec1-e482-06b2-a798-a51ff68f0d4b", "00000000-0000-0000-0000-000000000002", "42231ec1-e482-06b2-a798-a51ff68f0d4b", types.VirtualMachinePowerStateSuspended, 3, 1},
		{"78563412-ab90-efcd-1234-567890abcdef", "00000000-0000-0000-0000-000000000003", "78563412-ab90-efcd-1234-567890abcdef", types.VirtualMachinePowerState("mystery"), 0, 0},
		{"", "00000000-0000-0000-0000-000000000004", "", types.VirtualMachinePowerStatePoweredOff, 0, 0},
		{" \t ", "00000000-0000-0000-0000-000000000005", "", types.VirtualMachinePowerStatePoweredOff, 0, 0},
		{"42231ec1-e482-06b2-a798-a51ff68f0d4c", "00000000-0000-0000-0000-000000000006", "42231ec1-e482-06b2-a798-a51ff68f0d4c", types.VirtualMachinePowerStatePoweredOff, 5, 0},
		{"invalid-uuid", "00000000-0000-0000-0000-000000000007", "", types.VirtualMachinePowerStatePoweredOn, 0, 0},
	}
	if len(vms) != len(cases) {
		t.Fatalf("simulator has %d VMs, want %d", len(vms), len(cases))
	}
	host.Vm = nil
	for i, testCase := range cases {
		vm := vms[i].(*simulator.VirtualMachine)
		vm.Config.Uuid = testCase.configUUID
		vm.Config.InstanceUuid = testCase.instanceUUID
		vm.Summary.Config.Uuid = vm.Config.Uuid
		vm.Summary.Config.InstanceUuid = vm.Config.InstanceUuid
		vm.Runtime.PowerState = testCase.powerState
		vm.Summary.Runtime = vm.Runtime
		host.Vm = append(host.Vm, vm.Reference())
	}

	configPath := setupVMwareComponent(t, model, simulatorPassword, withSimulatorCA)
	stdout, stderr, err := runComponentBinary(t, configPath, "run", "--print")
	if err != nil {
		t.Fatalf("virtuoso run --print error = %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}
	mapping := decodePrintedMapping(t, stdout)
	if len(mapping.Hypervisors) != 1 {
		t.Fatalf("printed hypervisor count = %d, want 1", len(mapping.Hypervisors))
	}
	hypervisor := mapping.Hypervisors[0]
	if got := hypervisor.HypervisorID.HypervisorID; got != host.Hardware.SystemInfo.Uuid {
		t.Errorf("hypervisor ID = %q, want hardware UUID %q", got, host.Hardware.SystemInfo.Uuid)
	}
	type guestResult struct{ state, active int }
	wantGuests := make(map[string]guestResult, len(cases))
	for _, testCase := range cases {
		if testCase.wantGuestID != "" {
			wantGuests[testCase.wantGuestID] = guestResult{testCase.state, testCase.active}
		}
	}
	if len(hypervisor.Guests) != len(wantGuests) {
		t.Fatalf("printed guest count = %d, want %d", len(hypervisor.Guests), len(wantGuests))
	}
	for _, guest := range hypervisor.Guests {
		want, ok := wantGuests[guest.GuestID]
		if !ok {
			t.Errorf("unexpected or duplicate guest ID %q", guest.GuestID)
			continue
		}
		if guest.State != want.state {
			t.Errorf("guest %q state = %d, want %d", guest.GuestID, guest.State, want.state)
		}
		if guest.Attributes.Active != want.active {
			t.Errorf("guest %q active = %d, want %d", guest.GuestID, guest.Attributes.Active, want.active)
		}
		if guest.Attributes.VirtWhoType != "esx" {
			t.Errorf("guest %q virtWhoType = %q, want esx", guest.GuestID, guest.Attributes.VirtWhoType)
		}
		delete(wantGuests, guest.GuestID)
	}
	for guestID := range wantGuests {
		t.Errorf("printed mapping omits guest %q", guestID)
	}
}

func TestComponentVMwareHostsAcrossDatacentersWithoutVMs(t *testing.T) {
	model := newVMwareComponentModel(t, 2, 1, 0)
	hosts := model.Map().All("HostSystem")
	if len(hosts) != 2 {
		t.Fatalf("simulator has %d hosts, want 2", len(hosts))
	}
	hostIDs := []string{
		"aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
		"bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb",
	}
	seenHostIDs := make(map[string]bool, len(hostIDs))
	for i, id := range hostIDs {
		host := hosts[i].(*simulator.HostSystem)
		host.Hardware.SystemInfo.Uuid = id
		host.Summary.Hardware.Uuid = id
		seenHostIDs[id] = false
	}

	// Verify that a trailing CRLF is removed before authentication.
	configPath := setupVMwareComponent(t, model, simulatorPassword+"\r\n", withSimulatorCA)
	stdout, stderr, err := runComponentBinary(t, configPath, "run", "--print")
	if err != nil {
		t.Fatalf("virtuoso run --print error = %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}
	mapping := decodePrintedMapping(t, stdout)
	if len(mapping.Hypervisors) != 2 {
		t.Fatalf("printed hypervisor count = %d, want 2", len(mapping.Hypervisors))
	}
	for _, hypervisor := range mapping.Hypervisors {
		id := hypervisor.HypervisorID.HypervisorID
		seen, ok := seenHostIDs[id]
		if !ok || seen {
			t.Errorf("unexpected or duplicate hypervisor ID %q", id)
			continue
		}
		seenHostIDs[id] = true
		if len(hypervisor.Guests) != 0 {
			t.Errorf("hypervisor %q has %d guests, want 0", id, len(hypervisor.Guests))
		}
	}
	for id, seen := range seenHostIDs {
		if !seen {
			t.Errorf("printed mapping omits hypervisor %q", id)
		}
	}
}

func TestComponentVMwareSkipsHostWithoutDNSHostname(t *testing.T) {
	model := newVMwareComponentModel(t, 1, 2, 0)
	hosts := model.Map().All("HostSystem")
	if len(hosts) != 2 {
		t.Fatalf("simulator has %d hosts, want 2", len(hosts))
	}
	validHost := hosts[0].(*simulator.HostSystem)
	validHost.Config.Network.DnsConfig = &types.HostDnsConfig{HostName: "esxi-valid", DomainName: "example.test"}
	missingDNSHost := hosts[1].(*simulator.HostSystem)
	missingDNSHost.Name = "inventory-only-host"
	missingDNSHost.Summary.Config.Name = missingDNSHost.Name
	missingDNSHost.Config.Network.DnsConfig = nil

	configPath := setupVMwareComponent(t, model, simulatorPassword, withSimulatorCA)
	stdout, stderr, err := runComponentBinary(t, configPath, "run", "--print")
	if err != nil {
		t.Fatalf("virtuoso run --print error = %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}
	mapping := decodePrintedMapping(t, stdout)
	if len(mapping.Hypervisors) != 1 {
		t.Fatalf("printed hypervisor count = %d, want 1", len(mapping.Hypervisors))
	}
	host := mapping.Hypervisors[0]
	if host.Name != "esxi-valid.example.test" {
		t.Errorf("printed hypervisor name = %q, want esxi-valid.example.test", host.Name)
	}
}

func TestComponentVMwarePasswordCredentialErrors(t *testing.T) {
	cases := []struct {
		name           string
		password       string
		prepare        func(*testing.T)
		wantErrorParts []string
	}{
		{
			name:     "no credentials directory",
			password: simulatorPassword,
			prepare: func(t *testing.T) {
				t.Setenv("CREDENTIALS_DIRECTORY", "")
			},
			wantErrorParts: []string{"credentials_directory"},
		},
		{
			name:     "missing runtime file",
			password: simulatorPassword,
			prepare: func(t *testing.T) {
				t.Setenv("CREDENTIALS_DIRECTORY", t.TempDir())
			},
			wantErrorParts: []string{"credential", vmwarePasswordCredentialName},
		},
		{
			name:           "empty runtime file",
			wantErrorParts: []string{"credential", "empty"},
		},
		{
			name:           "newline-only runtime file",
			password:       "\n",
			wantErrorParts: []string{"credential", "empty"},
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			model := newVMwareComponentModel(t, 1, 1, 0)
			configPath := setupVMwareComponent(t, model, testCase.password, withSimulatorCA)
			if testCase.prepare != nil {
				testCase.prepare(t)
			}
			output := runFailingVMwareCommand(t, configPath, "run", "--print")
			normalizedOutput := normalizeVMwareOutput(output, configPath)
			assertOutputContainsAll(t, normalizedOutput, testCase.wantErrorParts...)
		})
	}
}

func TestComponentVMwareWrongPasswordWithTrailingSpace(t *testing.T) {
	model := newVMwareComponentModel(t, 1, 1, 0)
	configPath := setupVMwareComponent(t, model, simulatorPassword+" ", withSimulatorCA)
	for _, commandArgs := range [][]string{{"run", "--print"}, {"status"}} {
		t.Run(strings.Join(commandArgs, " "), func(t *testing.T) {
			output := runFailingVMwareCommand(t, configPath, commandArgs...)
			normalizedOutput := normalizeVMwareOutput(output, configPath)
			if !strings.Contains(normalizedOutput, "login") && !strings.Contains(normalizedOutput, "authentication") {
				t.Errorf("virtuoso error does not identify an authentication failure:\n%s", normalizedOutput)
			}
		})
	}
}

func TestComponentVMwareUntrustedCertificate(t *testing.T) {
	model := newVMwareComponentModel(t, 1, 1, 0)
	configPath := setupVMwareComponent(t, model, simulatorPassword, withoutSimulatorCA)
	output := runFailingVMwareCommand(t, configPath, "run", "--print")
	normalizedOutput := normalizeVMwareOutput(output, configPath)
	if !strings.Contains(normalizedOutput, "tls") && !strings.Contains(normalizedOutput, "x509") {
		t.Errorf("virtuoso error does not identify a TLS failure:\n%s", normalizedOutput)
	}
}

func TestComponentVMwareStatusRejectsMissingCredential(t *testing.T) {
	model := newVMwareComponentModel(t, 1, 1, 0)
	configPath := setupVMwareComponent(t, model, simulatorPassword, withSimulatorCA)
	t.Setenv("CREDENTIALS_DIRECTORY", t.TempDir())
	output := runFailingVMwareCommand(t, configPath, "status")
	normalizedOutput := normalizeVMwareOutput(output, configPath)
	assertOutputContainsAll(t, normalizedOutput, "credential", vmwarePasswordCredentialName)
}

func newVMwareComponentModel(t *testing.T, datacenters, hostsPerCluster, machinesPerPool int) *simulator.Model {
	t.Helper()
	model := simulator.VPX()
	model.Datacenter = datacenters
	model.Host = 0
	model.ClusterHost = hostsPerCluster
	model.Machine = machinesPerPool
	model.Autostart = false
	if err := model.Create(); err != nil {
		t.Fatalf("create vCenter simulator: %v", err)
	}
	t.Cleanup(model.Remove)
	return model
}

func runFailingVMwareCommand(t *testing.T, configPath string, commandArgs ...string) string {
	t.Helper()
	stdout, stderr, err := runComponentBinary(t, configPath, append([]string{"--log-level", "debug"}, commandArgs...)...)
	output := string(stdout) + string(stderr)
	if err == nil {
		t.Fatalf("virtuoso succeeded with an unusable VMware source:\n%s", output)
	}
	if strings.Contains(string(stdout), `"hypervisors"`) {
		t.Errorf("virtuoso printed a report after a VMware source failure:\n%s", stdout)
	}
	if strings.Contains(output, simulatorPassword) {
		t.Error("virtuoso printed the VMware password")
	}
	return output
}

func normalizeVMwareOutput(output, configPath string) string {
	// Temporary paths contain scenario names, which must not satisfy error
	// assertions.
	normalizedOutput := strings.ReplaceAll(output, configPath, "<config>")
	if credentialsDir := os.Getenv("CREDENTIALS_DIRECTORY"); credentialsDir != "" {
		normalizedOutput = strings.ReplaceAll(normalizedOutput, credentialsDir, "<runtime>")
	}
	return strings.ToLower(normalizedOutput)
}

func assertOutputContainsAll(t *testing.T, output string, wantParts ...string) {
	t.Helper()
	for _, part := range wantParts {
		if !strings.Contains(output, part) {
			t.Errorf("output does not contain %q:\n%s", part, output)
		}
	}
}

func setupVMwareComponent(t *testing.T, model *simulator.Model, password string, trustSimulatorCertificate bool) string {
	t.Helper()
	model.Service.Listen = &url.URL{Host: "127.0.0.1:0", User: url.UserPassword("reporter", simulatorPassword)}
	model.Service.TLS = &tls.Config{}
	server := model.Service.NewServer()
	t.Cleanup(server.Close)
	sourceURL := *server.URL
	sourceURL.User = nil
	// Systemd decrypts the stored secret before starting Virtuoso. These tests
	// supply only the resulting runtime file and require no system manager.
	credentialsDir := t.TempDir()
	t.Setenv("CREDENTIALS_DIRECTORY", credentialsDir)
	passwordCredentialPath := filepath.Join(credentialsDir, vmwarePasswordCredentialName)
	if err := os.WriteFile(passwordCredentialPath, []byte(password), 0o600); err != nil {
		t.Fatalf("write VMware runtime credential: %v", err)
	}
	configContents := fmt.Sprintf("[source]\ntype = %q\nuri = %q\nusername = %q\n", "vmware", sourceURL.String(), "reporter")
	if trustSimulatorCertificate {
		caFilePath, err := server.CertificateFile()
		if err != nil {
			t.Fatalf("write simulator certificate: %v", err)
		}
		configContents += fmt.Sprintf("ca_file = %q\n", caFilePath)
	}
	configPath := filepath.Join(t.TempDir(), "virtuoso.toml")
	if err := os.WriteFile(configPath, []byte(configContents), 0o600); err != nil {
		t.Fatalf("write VMware component config: %v", err)
	}
	return configPath
}
