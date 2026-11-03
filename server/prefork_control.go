package server

import (
	"bufio"
	"crypto/subtle"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"
)

func (supervisor *preforkSupervisor) acceptWorkers() {
	for {
		connection, err := supervisor.control.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return
			}

			select {
			case supervisor.failures <- fmt.Errorf("accept prefork worker registration: %w", err):
			case <-supervisor.done:
			}

			return
		}

		go supervisor.readWorkerReady(connection)
	}
}

func (supervisor *preforkSupervisor) readWorkerReady(connection net.Conn) {
	if err := connection.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		closePreforkResource(connection)
		return
	}

	line, err := bufio.NewReader(io.LimitReader(connection, preforkRegistrationLimit)).ReadString('\n')
	if err != nil {
		closePreforkResource(connection)
		return
	}

	fields := strings.Fields(line)
	if len(fields) != 2 {
		closePreforkResource(connection)
		return
	}

	pid, err := strconv.Atoi(fields[0])
	if err != nil {
		closePreforkResource(connection)
		return
	}

	if err := connection.SetReadDeadline(time.Time{}); err != nil {
		closePreforkResource(connection)
		return
	}

	supervisor.sendReady(preforkWorkerReady{
		pid:     pid,
		token:   fields[1],
		control: connection,
	})
}

func (supervisor *preforkSupervisor) sendReady(ready preforkWorkerReady) {
	select {
	case supervisor.ready <- ready:
	case <-supervisor.done:
		if ready.control != nil {
			closePreforkResource(ready.control)
		}
	}
}

func (supervisor *preforkSupervisor) registerReadyWorker(ready preforkWorkerReady) error {
	if subtle.ConstantTimeCompare([]byte(ready.token), []byte(supervisor.token)) != 1 {
		closePreforkResource(ready.control)
		return nil
	}

	worker, exists := supervisor.workers[ready.pid]
	if !exists || worker.ready {
		closePreforkResource(ready.control)
		return nil
	}

	worker.ready = true
	worker.control = ready.control
	supervisor.readyCount++

	return nil
}
