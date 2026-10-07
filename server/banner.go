package server

import (
	"fmt"
	"reflect"
	"runtime"
	"strings"
	"time"

	"github.com/elsyahtech/gorest/log"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/mem"
)

//nolint:revive
func printStartupBanner(
	server *Server,
	config *Config,
	logger *log.Log,
	appName,
	version,
	timezone,
	environment string,
	modules []string,
	routes ...any,
) error {
	address := fmt.Sprintf(
		"%s:%d",
		config.Host,
		config.Port,
	)

	memoryUsage := getMemoryUsage()
	processorUsage := getProcessorUsage()

	for _, route := range routes {
		rute, ok := route.(func(any))
		if !ok {
			continue
		}

		rute(server)
	}

	registeredPaths := getRegisteredEndpoints(server)

	quotedPaths := make([]string, 0, len(registeredPaths))

	for _, path := range registeredPaths {
		quotedPaths = append(quotedPaths, fmt.Sprintf("'%s'", path))
	}

	joinedPaths := strings.Join(quotedPaths, ", ")
	message := fmt.Sprintf("Endpoints running [%s]", joinedPaths)

	logger.Log(nil, true).Info(message)

	logger.Log(nil, true).Info("server started successfully")

	logger.Log(nil, true).Info("gorest is running")

	fmt.Println()
	fmt.Println("╔═════════════════════════════════════════════════════════════════════════════╗")
	fmt.Println("║  GOREST                                                                     ║")
	fmt.Println("║  v1.0.0                                                                     ║")
	fmt.Println("╠═════════════════════════════════════════════════════════════════════════════╣")
	fmt.Printf("║  App Name ...................... %-28s               ║\n", strings.ToUpper(appName))
	fmt.Printf("║  App Version ................... %-28s               ║\n", version)
	fmt.Printf("║  App Timezone .................. %-28s               ║\n", timezone)
	fmt.Printf("║  App Mode Running .............. %-28s               ║\n", environment)
	fmt.Printf("║  Host Running .................. %-28s               ║\n", address)
	fmt.Printf("║  Memory Usage .................. %-28s               ║\n", memoryUsage)
	fmt.Printf("║  Processor Usage ............... %-28s               ║\n", processorUsage)
	fmt.Printf("║  Modules Running ............... %-28d               ║\n", len(modules))

	for _, moduleRegister := range modules {
		fmt.Printf("║   - %-58s              ║\n", moduleRegister)
	}

	fmt.Printf("║  Endpoints Running ............. %-28d               ║\n", len(registeredPaths))

	for _, path := range registeredPaths {
		fmt.Printf("║   - %-58s              ║\n", path)
	}

	fmt.Println("╚═════════════════════════════════════════════════════════════════════════════╝")
	fmt.Println()

	return nil
}

func getRegisteredEndpoints(server *Server) []string {
	serverRoutes := server.SRV.GetRoutes()
	registeredPaths := make([]string, 0, len(serverRoutes))
	seen := make(map[string]bool)

	for _, route := range serverRoutes {
		if route.Path == "" || route.Method == "" {
			continue
		}

		if route.Path == "/" && route.Method != "GET" {
			continue
		}

		switch route.Method {
		case "GET", "POST", "PUT", "DELETE", "PATCH":
			key := route.Method + " " + route.Path

			if !seen[key] {
				seen[key] = true

				registeredPaths = append(registeredPaths, route.Method+" "+route.Path)
			}
		default:
		}
	}

	return registeredPaths
}

//nolint:unused
func routeName(register any) string {
	funFor := runtime.FuncForPC(reflect.ValueOf(register).Pointer())

	if funFor == nil {
		return "unknown"
	}

	return funFor.Name()
}

func getMemoryUsage() string {
	virtualMem, err := mem.VirtualMemory()

	var totalRAM string

	if err != nil {
		totalRAM = "Unknown RAM"
	} else {
		totalRAM = fmt.Sprintf("(%.0f GB)", float64(virtualMem.Total)/1024/1024/1024)
	}

	var memstat runtime.MemStats

	runtime.ReadMemStats(&memstat)

	goHeapAlloc := fmt.Sprintf("%.2f MB Usage", float64(memstat.Alloc)/1024/1024)

	return fmt.Sprintf("%s, %s", goHeapAlloc, totalRAM)
}

func getProcessorUsage() string {
	percentages, err := cpu.Percent(100*time.Millisecond, false)
	usage := 0.0

	if err == nil && len(percentages) > 0 {
		usage = percentages[0]
	}

	return fmt.Sprintf("%.1f%% Usage, (%d Cores)", usage, runtime.NumCPU())
}
