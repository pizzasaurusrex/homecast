package bridge

import (
	"encoding/xml"
	"strings"
	"testing"

	"github.com/pizzasaurusrex/homecast/internal/config"
)

func TestGenerateAirConnectXMLEmpty(t *testing.T) {
	out, err := GenerateAirConnectXML(config.Default())
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if !strings.HasPrefix(s, xml.Header) {
		t.Error("output missing XML header")
	}
	if !strings.Contains(s, "<aircast>") {
		t.Errorf("expected <aircast> root, got: %s", s)
	}
	if strings.Contains(s, "<device>") {
		t.Errorf("expected no <device> elements for empty config, got: %s", s)
	}
}

func TestGenerateAirConnectXMLEnabledFlag(t *testing.T) {
	cfg := config.Default()
	cfg.Devices = []config.Device{
		{ID: "kitchen-id", Name: "Kitchen Home", Enabled: true},
		{ID: "office-id", Name: "Office Nest", Enabled: false},
	}
	out, err := GenerateAirConnectXML(cfg)
	if err != nil {
		t.Fatal(err)
	}

	var parsed AirConnectConfig
	if err := xml.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("round-trip unmarshal: %v", err)
	}
	if len(parsed.Devices) != 2 {
		t.Fatalf("expected 2 devices, got %d", len(parsed.Devices))
	}
	if parsed.Devices[0].UDN != "kitchen-id" || parsed.Devices[0].Enabled != "1" {
		t.Errorf("device[0] wrong: %+v", parsed.Devices[0])
	}
	if parsed.Devices[1].UDN != "office-id" || parsed.Devices[1].Enabled != "0" {
		t.Errorf("device[1] wrong: %+v", parsed.Devices[1])
	}
}

func TestGenerateAirConnectXMLPreservesOrder(t *testing.T) {
	cfg := config.Default()
	cfg.Devices = []config.Device{
		{ID: "a", Name: "A", Enabled: true},
		{ID: "b", Name: "B", Enabled: true},
		{ID: "c", Name: "C", Enabled: true},
	}
	out, err := GenerateAirConnectXML(cfg)
	if err != nil {
		t.Fatal(err)
	}
	idxA := strings.Index(string(out), "<udn>a</udn>")
	idxB := strings.Index(string(out), "<udn>b</udn>")
	idxC := strings.Index(string(out), "<udn>c</udn>")
	if idxA >= idxB || idxB >= idxC {
		t.Errorf("order not preserved: a=%d b=%d c=%d", idxA, idxB, idxC)
	}
}
