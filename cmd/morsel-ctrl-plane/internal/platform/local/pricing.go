// LocalPlatform has no billing — Prices returns zero values immediately.
package local

import (
	"context"
	"time"

	"github.com/ifeanyiecheruo/morsel/internal/platform"
)

type localPricingProvider struct{}

func (lpp *localPricingProvider) Prices(_ context.Context) (platform.Prices, error) {
	return platform.Prices{FetchedAt: time.Now()}, nil
}
