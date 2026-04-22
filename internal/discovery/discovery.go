// Package discovery browses the local network for Google Cast devices via mDNS.
package discovery

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/grandcat/zeroconf"
)

const castService = "_googlecast._tcp"

type Device struct {
	ID    string
	Name  string
	Host  string
	Port  int
	Addrs []string
}

type Discoverer interface {
	Browse(ctx context.Context, timeout time.Duration) ([]Device, error)
}

type zeroconfDiscoverer struct{}

func New() Discoverer { return zeroconfDiscoverer{} }

func (zeroconfDiscoverer) Browse(ctx context.Context, timeout time.Duration) ([]Device, error) {
	resolver, err := zeroconf.NewResolver(nil)
	if err != nil {
		return nil, err
	}

	entries := make(chan *zeroconf.ServiceEntry)
	var (
		mu      sync.Mutex
		results []Device
	)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for e := range entries {
			d := mapEntry(e)
			mu.Lock()
			results = append(results, d)
			mu.Unlock()
		}
	}()

	browseCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if err := resolver.Browse(browseCtx, castService, "local.", entries); err != nil {
		return nil, err
	}
	<-browseCtx.Done()
	<-done

	mu.Lock()
	defer mu.Unlock()
	out := make([]Device, len(results))
	copy(out, results)
	return out, nil
}

func mapEntry(e *zeroconf.ServiceEntry) Device {
	d := Device{
		ID:   e.Instance,
		Name: friendlyName(e),
		Host: e.HostName,
		Port: e.Port,
	}
	for _, ip := range e.AddrIPv4 {
		d.Addrs = append(d.Addrs, ip.String())
	}
	for _, ip := range e.AddrIPv6 {
		d.Addrs = append(d.Addrs, ip.String())
	}
	return d
}

func friendlyName(e *zeroconf.ServiceEntry) string {
	for _, txt := range e.Text {
		if strings.HasPrefix(txt, "fn=") {
			return strings.TrimPrefix(txt, "fn=")
		}
	}
	return e.Instance
}

// Fake is a deterministic Discoverer for tests.
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
