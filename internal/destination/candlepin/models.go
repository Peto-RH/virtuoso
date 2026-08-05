package candlepin

import "github.com/Peto-RH/virtuoso/internal/report"

type hostGuestMappingRequest struct {
	Hypervisors []hypervisor `json:"hypervisors"`
}

type hypervisorID struct {
	HypervisorID string `json:"hypervisorId"`
}

type hypervisor struct {
	HypervisorID hypervisorID `json:"hypervisorId"`
	Name         string       `json:"name,omitempty"`
	Guests       []guest      `json:"guestIds"`
}

type guest struct {
	GuestID    string          `json:"guestId"`
	State      guestState      `json:"state"`
	Attributes guestAttributes `json:"attributes"`
}

type guestAttributes struct {
	VirtWhoType string `json:"virtWhoType"`
	Active      int    `json:"active"`
}

type guestState int

const (
	guestStateUnknown     guestState = 0
	guestStateRunning     guestState = 1
	guestStateBlocked     guestState = 2
	guestStatePaused      guestState = 3
	guestStateShutdown    guestState = 4
	guestStateShutoff     guestState = 5
	guestStateCrashed     guestState = 6
	guestStatePMSuspended guestState = 7
)

type jobResponse struct {
	ID    string `json:"id"`
	State string `json:"state"`
}

func newHostGuestMappingRequest(collectedHypervisor *report.Hypervisor) hostGuestMappingRequest {
	guests := make([]guest, 0, len(collectedHypervisor.Guests))
	for _, collectedGuest := range collectedHypervisor.Guests {
		guests = append(guests, guest{
			GuestID: collectedGuest.ID,
			State:   guestStateFromReport(collectedGuest.State),
			Attributes: guestAttributes{
				VirtWhoType: collectedHypervisor.Source,
				Active:      activeValue(collectedGuest.State),
			},
		})
	}

	return hostGuestMappingRequest{
		Hypervisors: []hypervisor{
			{
				HypervisorID: hypervisorID{HypervisorID: collectedHypervisor.ID},
				Name:         collectedHypervisor.Name,
				Guests:       guests,
			},
		},
	}
}

func guestStateFromReport(state report.GuestState) guestState {
	switch state {
	case report.GuestStateRunning:
		return guestStateRunning
	case report.GuestStateBlocked:
		return guestStateBlocked
	case report.GuestStatePaused:
		return guestStatePaused
	case report.GuestStateShutdown:
		return guestStateShutdown
	case report.GuestStateShutoff:
		return guestStateShutoff
	case report.GuestStateCrashed:
		return guestStateCrashed
	case report.GuestStatePMSuspended:
		return guestStatePMSuspended
	default:
		return guestStateUnknown
	}
}

func activeValue(state report.GuestState) int {
	if state == report.GuestStateRunning || state == report.GuestStatePaused {
		return 1
	}
	return 0
}
