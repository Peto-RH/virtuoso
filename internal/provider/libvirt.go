package provider

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/pschrimp/virtuoso/internal/report"
	"libvirt.org/go/libvirt"
)

// LibvirtProvider collects virtual machine data from libvirt
type LibvirtProvider struct {
	conn *libvirt.Connect
	uri  string
}

// NewLibvirtProvider creates a new libvirt provider and establishes connection
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

// Collect retrieves virtual machine information from libvirt
func (p *LibvirtProvider) Collect(ctx context.Context) (*report.Report, error) {
	slog.Debug("collecting virtual machine data")
	slog.Debug("listing domains")

	domains, err := p.conn.ListAllDomains(libvirt.CONNECT_LIST_DOMAINS_ACTIVE | libvirt.CONNECT_LIST_DOMAINS_INACTIVE)
	if err != nil {
		return nil, fmt.Errorf("cannot list domains: %w", err)
	}

	// Free all domain objects
	defer func() {
		for _, domain := range domains {
			domain.Free()
		}
	}()

	guests := make([]report.Guest, 0, len(domains))
	for _, domain := range domains {
		uuid, err := domain.GetUUIDString()
		if err != nil {
			slog.Debug("skipping domain, cannot get UUID", "err", err)
			continue
		}

		name, err := domain.GetName()
		if err != nil {
			slog.Debug("skipping domain, cannot get name", "uuid", uuid, "err", err)
			continue
		}

		guests = append(guests, report.Guest{
			UUID: uuid,
			Name: name,
		})
	}

	slog.Debug("data collection finished", "count", len(guests))

	return &report.Report{
		Source: "libvirt",
		URI:    p.uri,
		Guests: guests,
	}, nil
}

// Ping tests the libvirt connection without collecting data
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

// Close closes the libvirt connection
func (p *LibvirtProvider) Close() error {
	slog.Debug("closing libvirt connection")
	if p.conn != nil {
		_, err := p.conn.Close()
		return err
	}
	return nil
}
