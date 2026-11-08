package hardening

type Config struct {
	// Enable the runtime hardening report endpoint. Defaults to false.
	Enable bool `json:"enable"`

	// HardeningAllowedIPs lists peer IP addresses or CIDR ranges allowed to
	// access GET /gorest/hardening. An empty list denies access. The connection
	// peer is checked directly; forwarded headers are not used.
	//nolint:tagliatelle // Keep snake_case JSON config keys consistent.
	HardeningAllowedIPs []string `json:"hardening_allowed_ips"`
}

var ConfigDefault = Config{
	Enable:              false,
	HardeningAllowedIPs: nil,
}

func configDefault(config ...Config) Config {
	if len(config) < 1 {
		cfg := ConfigDefault
		cfg.HardeningAllowedIPs = append([]string(nil), ConfigDefault.HardeningAllowedIPs...)
		return cfg
	}

	cfg := config[0]
	cfg.HardeningAllowedIPs = append([]string(nil), cfg.HardeningAllowedIPs...)
	return cfg
}
