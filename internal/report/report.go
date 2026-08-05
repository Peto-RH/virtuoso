package report

type Hypervisor struct {
	Source string  `json:"source"`
	ID     string  `json:"id"`
	Name   string  `json:"name,omitempty"`
	Guests []Guest `json:"guests"`
}

type Guest struct {
	ID    string     `json:"id"`
	State GuestState `json:"state"`
}

type GuestState int

const (
	GuestStateUnknown     GuestState = 0
	GuestStateRunning     GuestState = 1
	GuestStateBlocked     GuestState = 2
	GuestStatePaused      GuestState = 3
	GuestStateShutdown    GuestState = 4
	GuestStateShutoff     GuestState = 5
	GuestStateCrashed     GuestState = 6
	GuestStatePMSuspended GuestState = 7
)
