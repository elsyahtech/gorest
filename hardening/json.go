package hardening

import "encoding/json"

// RenderJSON encodes a report as indented JSON.
func RenderJSON(report Report) ([]byte, error) {
	return json.MarshalIndent(report, "", "  ")
}
