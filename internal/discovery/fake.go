package discovery

import (
	"context"
	"time"
)

// Fake is a deterministic Discoverer for tests and dry-run scenarios.
type Fake struct {
	Devices []Device
	Err     error
}

func (f Fake) Browse(_ context.Context, _ time.Duration) ([]Device, error) {
	if f.Err != nil {
		return nil, f.Err
	}
	out := make([]Device, len(f.Devices))
	copy(out, f.Devices)
	return out, nil
}
