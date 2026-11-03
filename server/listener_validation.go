package server

import (
	"crypto/tls"
	"errors"
	"fmt"
	"runtime"

	"github.com/gofiber/fiber/v3"
)

func validateListenerConfig(config ListenerConfig) error {
	validators := []func(ListenerConfig) error{
		validateListenerNetwork,
		validateListenerTLS,
		validateListenerPrefork,
		validateListenerDurations,
	}
	for _, validate := range validators {
		if err := validate(config); err != nil {
			return err
		}
	}

	return nil
}

func validateListenerNetwork(config ListenerConfig) error {
	switch config.Network {
	case fiber.NetworkTCP, fiber.NetworkTCP4, fiber.NetworkTCP6:
		if config.UnixSocketPath != "" {
			return errors.New("server listener: UnixSocketPath is only valid when Network is unix")
		}
	case fiber.NetworkUnix:
		if config.UnixSocketPath == "" {
			return errors.New("server listener: UnixSocketPath is required when Network is unix")
		}
	default:
		return fmt.Errorf("server listener: unsupported network %q; use tcp, tcp4, tcp6, or unix", config.Network)
	}

	return nil
}

func validateListenerTLS(config ListenerConfig) error {
	if (config.TLSCertFile == "") != (config.TLSCertKeyFile == "") {
		return errors.New("server listener: TLSCertFile and TLSCertKeyFile must both be provided")
	}

	if config.TLSCertClientFile != "" && config.TLSCertFile == "" {
		return errors.New("server listener: TLSCertClientFile requires TLSCertFile and TLSCertKeyFile")
	}

	if config.TLSMinVersion != tls.VersionTLS12 && config.TLSMinVersion != tls.VersionTLS13 {
		return fmt.Errorf("server listener: unsupported TLS minimum version %d; use TLS 1.2 or TLS 1.3", config.TLSMinVersion)
	}

	return nil
}

func validateListenerPrefork(config ListenerConfig) error {
	if config.EnablePrefork && config.Network != fiber.NetworkTCP4 && config.Network != fiber.NetworkTCP6 {
		return errors.New("server listener: EnablePrefork requires Network tcp4 or tcp6")
	}

	if config.EnablePrefork && runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		return fmt.Errorf("server listener: GOREST-managed prefork is not supported on %s", runtime.GOOS)
	}

	return nil
}

func validateListenerDurations(config ListenerConfig) error {
	if config.ShutdownTimeout < 0 {
		return errors.New("server listener: ShutdownTimeout cannot be negative")
	}

	if config.PreforkRecoverInterval < 0 {
		return errors.New("server listener: PreforkRecoverInterval cannot be negative")
	}

	if config.PreforkShutdownGracePeriod < 0 {
		return errors.New("server listener: PreforkShutdownGracePeriod cannot be negative")
	}

	if config.PreforkRecoverThreshold < 0 {
		return errors.New("server listener: PreforkRecoverThreshold cannot be negative")
	}

	return nil
}
