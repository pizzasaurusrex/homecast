package bridge

import (
	"encoding/xml"
	"fmt"

	"github.com/pizzasaurusrex/homecast/internal/config"
)

// AirConnectConfig mirrors the subset of aircast's XML config we generate.
// The schema here is a best-effort match; in M3 we verify against a config
// produced by `aircast -i` and adjust if needed.
type AirConnectConfig struct {
	XMLName xml.Name    `xml:"aircast"`
	Common  AirCommon   `xml:"common"`
	Devices []AirDevice `xml:"device"`
}

type AirCommon struct {
	Enabled string `xml:"enabled"`
	Codec   string `xml:"codec,omitempty"`
}

type AirDevice struct {
	UDN     string `xml:"udn"`
	Name    string `xml:"name"`
	Enabled string `xml:"enabled"`
}

func GenerateAirConnectXML(cfg *config.Config) ([]byte, error) {
	ac := AirConnectConfig{
		Common: AirCommon{Enabled: "1", Codec: "flac"},
	}
	for _, d := range cfg.Devices {
		enabled := "0"
		if d.Enabled {
			enabled = "1"
		}
		ac.Devices = append(ac.Devices, AirDevice{
			UDN:     d.ID,
			Name:    d.Name,
			Enabled: enabled,
		})
	}
	body, err := xml.MarshalIndent(ac, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal aircast config: %w", err)
	}
	out := make([]byte, 0, len(xml.Header)+len(body)+1)
	out = append(out, xml.Header...)
	out = append(out, body...)
	out = append(out, '\n')
	return out, nil
}
