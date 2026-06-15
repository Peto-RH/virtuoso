package report

// Report contains virtual machine association data collected from a hypervisor
type Report struct {
	Source string  `json:"source"` // Source type: "libvirt"
	URI    string  `json:"uri"`    // Source identifier: "qemu:///system" or hostname
	Guests []Guest `json:"guests"` // List of virtual machines
}

// Guest represents a virtual machine instance
type Guest struct {
	UUID string `json:"uuid"` // Domain UUID
	Name string `json:"name"` // Domain name
}
