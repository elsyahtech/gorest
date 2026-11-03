package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	stdlog "log"
	"net"
	"os"
	"strings"
	"syscall"
	"time"
)

func announcePreforkWorker() error {
	address := os.Getenv(preforkControlAddrEnv)

	token := os.Getenv(preforkControlTokenEnv)
	if address == "" || token == "" {
		return errors.New("prefork worker registration settings are missing")
	}

	connection, err := (&net.Dialer{Timeout: 5 * time.Second}).DialContext(context.Background(), "tcp4", address)
	if err != nil {
		return fmt.Errorf("connect to prefork supervisor: %w", err)
	}

	if _, err := fmt.Fprintf(connection, "%d %s\n", os.Getpid(), token); err != nil {
		closePreforkResource(connection)
		return fmt.Errorf("register with prefork supervisor: %w", err)
	}

	go monitorPreforkSupervisor(connection)

	return nil
}

func monitorPreforkSupervisor(connection net.Conn) {
	if _, err := io.Copy(io.Discard, connection); err != nil {
		stdlog.Printf("gorest prefork worker: monitor supervisor connection: %v", err)
	}

	closePreforkResource(connection)

	process, err := os.FindProcess(os.Getpid())
	if err != nil {
		return
	}

	if err := process.Signal(syscall.SIGTERM); err != nil && !errors.Is(err, os.ErrProcessDone) {
		stdlog.Printf("gorest prefork worker could not stop after supervisor disconnect: %v", err)
	}
}

func closePreforkResource(resource io.Closer) {
	if err := resource.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
		stdlog.Printf("gorest prefork: close resource: %v", err)
	}
}

func setEnvironment(environment []string, values map[string]string) []string {
	updated := make([]string, 0, len(environment)+len(values))
	for _, entry := range environment {
		separator := strings.IndexByte(entry, '=')
		if separator < 0 {
			updated = append(updated, entry)
			continue
		}

		if _, replace := values[entry[:separator]]; !replace {
			updated = append(updated, entry)
		}
	}

	for key, value := range values {
		updated = append(updated, key+"="+value)
	}

	return updated
}
