package filter

import (
	"errors"
	"fmt"
	"net"
	"strings"
)

func validateIPRestrictions(allowedEntries, blockedEntries []string) error {
	if _, err := parseIPRanges(allowedEntries); err != nil {
		return fmt.Errorf("allowed IPs: %w", err)
	}
	if _, err := parseIPRanges(blockedEntries); err != nil {
		return fmt.Errorf("blocked IPs: %w", err)
	}

	return nil
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

func ipMatchesAny(ip net.IP, ranges []*net.IPNet) bool {
	if ip == nil {
		return false
	}

	for _, ipRange := range ranges {
		if ipRange.Contains(ip) {
			return true
		}
	}

	return false
}

func ipAllowed(peerIP net.IP, allowed, blocked []*net.IPNet) bool {
	if ipMatchesAny(peerIP, blocked) {
		return false
	}

	return len(allowed) == 0 || ipMatchesAny(peerIP, allowed)
}
