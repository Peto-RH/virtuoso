package provider

import (
	"testing"

	"github.com/Peto-RH/virtuoso/internal/report"
	"libvirt.org/go/libvirt"
)

func TestGuestStateFromLibvirt(t *testing.T) {
	tests := []struct {
		name  string
		state libvirt.DomainState
		want  report.GuestState
	}{
		{name: "no state", state: libvirt.DOMAIN_NOSTATE, want: report.GuestStateUnknown},
		{name: "running", state: libvirt.DOMAIN_RUNNING, want: report.GuestStateRunning},
		{name: "blocked", state: libvirt.DOMAIN_BLOCKED, want: report.GuestStateBlocked},
		{name: "paused", state: libvirt.DOMAIN_PAUSED, want: report.GuestStatePaused},
		{name: "shutdown", state: libvirt.DOMAIN_SHUTDOWN, want: report.GuestStateShutdown},
		{name: "shutoff", state: libvirt.DOMAIN_SHUTOFF, want: report.GuestStateShutoff},
		{name: "crashed", state: libvirt.DOMAIN_CRASHED, want: report.GuestStateCrashed},
		{name: "power management suspended", state: libvirt.DOMAIN_PMSUSPENDED, want: report.GuestStatePMSuspended},
		{name: "unsupported", state: libvirt.DomainState(99), want: report.GuestStateUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := guestStateFromLibvirt(tt.state); got != tt.want {
				t.Errorf("guestStateFromLibvirt(%d) = %d, want %d", tt.state, got, tt.want)
			}
		})
	}
}
