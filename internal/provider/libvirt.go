package provider

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/Peto-RH/virtuoso/internal/destination/candlepin"
	"libvirt.org/go/libvirt"
)

type LibvirtProvider struct {
	conn *libvirt.Connect
	uri  string
}

func NewLibvirtProvider(ctx context.Context, uri string) (*LibvirtProvider, error) {
	slog.Debug("connecting to libvirt", "uri", uri)

	conn, err := libvirt.NewConnect(uri)
	if err != nil {
		return nil, fmt.Errorf("cannot connect to libvirt: %w", err)
	}

	slog.Debug("connection established")
	return &LibvirtProvider{
		conn: conn,
		uri:  uri,
	}, nil
}

func (p *LibvirtProvider) Collect(ctx context.Context) (*candlepin.Hypervisor, error) {
	slog.Debug("collecting virtual machine data")
	slog.Debug("listing domains")

	domains, err := p.conn.ListAllDomains(libvirt.CONNECT_LIST_DOMAINS_ACTIVE | libvirt.CONNECT_LIST_DOMAINS_INACTIVE)
	if err != nil {
		return nil, fmt.Errorf("cannot list domains: %w", err)
	}

	defer func() {
		for _, domain := range domains {
			domain.Free()
		}
	}()

	guests := make([]candlepin.Guest, 0, len(domains))
	for _, domain := range domains {
		uuid, err := domain.GetUUIDString()
		if err != nil {
			slog.Warn("skipping domain, cannot get UUID", "err", err)
			continue
		}

		state, _, err := domain.GetState()
		if err != nil {
			slog.Warn("cannot get domain state, assuming unknown", "uuid", uuid, "err", err)
			state = libvirt.DOMAIN_NOSTATE
		}

		guestState := candlepin.GuestState(state)
		active := 0
		if guestState.IsActive() {
			active = 1
		}

		guests = append(guests, candlepin.Guest{
			GuestID: uuid,
			State:   int(guestState),
			Attributes: map[string]interface{}{
				"virtWhoType": "libvirt",
				"active":      active,
			},
		})
	}

	hypervisorID, err := p.conn.GetHostname()
	if err != nil {
		slog.Warn("cannot get hypervisor hostname, using URI", "err", err)
		hypervisorID = p.uri
	}

	return &candlepin.Hypervisor{
		HypervisorID: candlepin.HypervisorID{
			HypervisorID: hypervisorID,
		},
		Name:   hypervisorID,
		Guests: guests,
	}, nil
}

func (p *LibvirtProvider) Ping(ctx context.Context) error {
	slog.Debug("testing libvirt connection")

	// Simple version check to verify connection works
	_, err := p.conn.GetVersion()
	if err != nil {
		return fmt.Errorf("libvirt connection test failed: %w", err)
	}

	slog.Debug("connection test successful")
	return nil
}

func (p *LibvirtProvider) Close() error {
	slog.Debug("closing libvirt connection")
	if p.conn != nil {
		_, err := p.conn.Close()
		return err
	}
	return nil
}
