package discovery

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/grandcat/zeroconf"
)

func TestFakeReturnsCopy(t *testing.T) {
	f := Fake{Devices: []Device{{ID: "a", Name: "A"}}}
	got, err := f.Browse(context.Background(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	got[0].Name = "mutated"
	if f.Devices[0].Name == "mutated" {
		t.Error("Fake.Browse should return a copy, but caller mutated source")
	}
}

func TestFakePropagatesError(t *testing.T) {
	want := errors.New("boom")
	f := Fake{Err: want}
	if _, err := f.Browse(context.Background(), 0); !errors.Is(err, want) {
		t.Fatalf("got %v, want %v", err, want)
	}
}

func TestMapEntryUsesFriendlyNameFromTXT(t *testing.T) {
	e := &zeroconf.ServiceEntry{
		ServiceRecord: zeroconf.ServiceRecord{Instance: "instance-id"},
		HostName:      "host.local.",
		Port:          8009,
		AddrIPv4:      []net.IP{net.ParseIP("192.168.1.10")},
		Text:          []string{"id=x", "fn=Kitchen Home", "md=Google Home"},
	}
	d := mapEntry(e)
	if d.ID != "instance-id" {
		t.Errorf("ID = %q, want %q", d.ID, "instance-id")
	}
	if d.Name != "Kitchen Home" {
		t.Errorf("Name = %q, want %q", d.Name, "Kitchen Home")
	}
	if d.Port != 8009 {
		t.Errorf("Port = %d, want 8009", d.Port)
	}
	if len(d.Addrs) != 1 || d.Addrs[0] != "192.168.1.10" {
		t.Errorf("Addrs = %v, want [192.168.1.10]", d.Addrs)
	}
}

func TestMapEntryFallsBackToInstanceWhenNoTXT(t *testing.T) {
	e := &zeroconf.ServiceEntry{
		ServiceRecord: zeroconf.ServiceRecord{Instance: "instance-id"},
		HostName:      "host.local.",
		Port:          8009,
	}
	d := mapEntry(e)
	if d.Name != "instance-id" {
		t.Errorf("Name = %q, want fallback to instance", d.Name)
	}
}

func TestZeroconfBrowseShortTimeoutReturnsQuickly(t *testing.T) {
	// Exercises the real zeroconf code path without asserting on results,
	// which vary by host network. Success criteria: the call returns within
	// a bounded time, regardless of whether mDNS is available.
	d := New()
	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, _ = d.Browse(ctx, 100*time.Millisecond)
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("Browse took too long: %v", elapsed)
	}
}

func TestMapEntryIncludesIPv6(t *testing.T) {
	e := &zeroconf.ServiceEntry{
		ServiceRecord: zeroconf.ServiceRecord{Instance: "x"},
		AddrIPv4:      []net.IP{net.ParseIP("10.0.0.1")},
		AddrIPv6:      []net.IP{net.ParseIP("fe80::1")},
	}
	d := mapEntry(e)
	if len(d.Addrs) != 2 {
		t.Fatalf("expected 2 addrs, got %v", d.Addrs)
	}
}
