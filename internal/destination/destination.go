package destination

import (
	"context"

	"github.com/Peto-RH/virtuoso/internal/destination/candlepin"
)

type Destination interface {
	Send(ctx context.Context, hyp *candlepin.Hypervisor) error
	Ping(ctx context.Context) error
	Close() error
}
