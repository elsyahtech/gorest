package hardening

import (
	"errors"
	"fmt"
	"net"
	"strings"
)

func Run(config ...Config) Config {
	return configDefault(config...)
}

func New(config Config) (*Hardening, error) {
	config = Run(config)
	allowedIPs, err := parseIPRanges(config.HardeningAllowedIPs)
	if err != nil {
		return nil, fmt.Errorf("invalid hardening IP allowlist: %w", err)
	}

	return &Hardening{
		Config:     config,
		allowedIPs: allowedIPs,
	}, nil
}

func parseIPRanges(entries []string) ([]*net.IPNet, error) {
	ranges := make([]*net.IPNet, 0, len(entries))
	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			return nil, errors.New("IP address or CIDR entry must not be empty")
		}

		if ip := net.ParseIP(entry); ip != nil {
			if ipv4 := ip.To4(); ipv4 != nil {
				ranges = append(ranges, &net.IPNet{IP: ipv4, Mask: net.CIDRMask(32, 32)})
			} else {
				ranges = append(ranges, &net.IPNet{IP: ip.To16(), Mask: net.CIDRMask(128, 128)})
			}
			continue
		}

		_, ipRange, err := net.ParseCIDR(entry)
		if err != nil {
			return nil, fmt.Errorf("invalid IP address or CIDR %q: %w", entry, err)
		}
		ranges = append(ranges, ipRange)
	}

	return ranges, nil
}
