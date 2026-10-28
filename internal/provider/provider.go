package provider

import (
	"context"
	"fmt"

	"github.com/Peto-RH/virtuoso/internal/config"
	"github.com/Peto-RH/virtuoso/internal/report"
)

type Provider interface {
	Collect(context.Context) ([]report.Hypervisor, error)
	Ping(context.Context) error
	Close() error
}

func NewProvider(ctx context.Context, cfg *config.SourceConfig) (Provider, error) {
	switch cfg.Type {
	case "libvirt":
		return NewLibvirtProvider(ctx, cfg.URI)
	case "vmware":
		return NewVMwareProvider(ctx, cfg)
	default:
		return nil, fmt.Errorf("unsupported source.type %q: use libvirt or vmware", cfg.Type)
	}
}
