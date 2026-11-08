package hardening

import "net"

type Hardening struct {
	Config     Config
	allowedIPs []*net.IPNet
}

// Enabled reports whether the hardening report endpoint should be registered.
func (hardening *Hardening) Enabled() bool {
	return hardening != nil && hardening.Config.Enable
}

// AllowsPeer reports whether the connection peer is allowed to access the report.
func (hardening *Hardening) AllowsPeer(peerIP net.IP) bool {
	if !hardening.Enabled() || peerIP == nil || len(hardening.allowedIPs) == 0 {
		return false
	}

	for _, allowedIP := range hardening.allowedIPs {
		if allowedIP.Contains(peerIP) {
			return true
		}
	}

	return false
}
