package server

import (
	"log"
	"strings"

	"github.com/gofiber/fiber/v3"
)

func warnOnUnboundedTimeouts(core *fiber.Config) {
	if core == nil {
		return
	}

	var unbounded []string
	if core.ReadTimeout == 0 {
		unbounded = append(unbounded, "ReadTimeout")
	}

	if core.WriteTimeout == 0 {
		unbounded = append(unbounded, "WriteTimeout")
	}

	if len(unbounded) == 0 {
		return
	}

	log.Printf(
		"WARNING: server Core sets %s to zero, which Fiber treats as unlimited; "+
			"slow request bodies or response writes can hold connections for an unbounded time. "+
			"Configure finite positive timeouts unless this is intentional. "+
			"Fiber uses ReadTimeout when IdleTimeout is zero.",
		strings.Join(unbounded, " and "),
	)
}
