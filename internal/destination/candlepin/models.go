package candlepin

type HostGuestMappingRequest struct {
	Hypervisors []Hypervisor `json:"hypervisors"`
}

type HypervisorID struct {
	HypervisorID string `json:"hypervisorId"`
}

type Hypervisor struct {
	HypervisorID HypervisorID `json:"hypervisorId"`
	Name         string       `json:"name,omitempty"`
	Guests       []Guest      `json:"guestIds"`
}

type Guest struct {
	GuestID    string                 `json:"guestId"`
	State      int                    `json:"state"`
	Attributes map[string]interface{} `json:"attributes"`
}

// GuestState represents libvirt domain state
type GuestState int

const (
	StateUnknown     GuestState = 0
	StateRunning     GuestState = 1
	StateBlocked     GuestState = 2
	StatePaused      GuestState = 3
	StateShutdown    GuestState = 4
	StateShutoff     GuestState = 5
	StateCrashed     GuestState = 6
	StatePMSuspended GuestState = 7
)

// IsActive returns true if guest should be marked as active in Candlepin
func (s GuestState) IsActive() bool {
	return s == StateRunning || s == StatePaused
}

type JobResponse struct {
	ID    string `json:"id"`
	State string `json:"state"`
}
