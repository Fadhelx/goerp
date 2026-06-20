package cli

import (
	"fmt"
	"io"
	"net/http"
	"sort"

	"gorp/internal/config"
	"gorp/internal/logging"
	"gorp/internal/runtime"
)

const version = "0.0.0"

func Run(args []string, stdout, stderr io.Writer) int {
	logging.New("error", stderr)

	if len(args) == 0 {
		printHelp(stdout)
		return 0
	}

	switch args[0] {
	case "help", "-h", "--help":
		printHelp(stdout)
		return 0
	case "version", "--version":
		fmt.Fprintf(stdout, "gorp %s\n", version)
		return 0
	case "env":
		cfg := config.Load()
		fmt.Fprintf(stdout, "env=%s\nhttp_addr=%s\nodoo_source_root=%s\n", cfg.Environment, cfg.HTTPAddr, cfg.OdooSourceRoot)
		return 0
	case "modules":
		app, err := runtime.BootstrapOI("")
		if err != nil {
			fmt.Fprintf(stderr, "bootstrap failed: %v\n", err)
			return 1
		}
		names := make([]string, 0, len(app.Modules))
		for name := range app.Modules {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			fmt.Fprintln(stdout, name)
		}
		return 0
	case "serve":
		cfg := config.Load()
		app, err := runtime.BootstrapOI("")
		if err != nil {
			fmt.Fprintf(stderr, "bootstrap failed: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "listening on %s\n", cfg.HTTPAddr)
		if err := http.ListenAndServe(cfg.HTTPAddr, app.Server().Handler()); err != nil {
			fmt.Fprintf(stderr, "server failed: %v\n", err)
			return 1
		}
		return 0
	default:
		fmt.Fprintf(stderr, "unknown command: %s\n", args[0])
		printHelp(stderr)
		return 2
	}
}

func printHelp(w io.Writer) {
	fmt.Fprintln(w, "gorpd commands:")
	fmt.Fprintln(w, "  help      Show help")
	fmt.Fprintln(w, "  version   Show version")
	fmt.Fprintln(w, "  env       Print sanitized environment configuration")
	fmt.Fprintln(w, "  modules   List installed runtime modules")
	fmt.Fprintln(w, "  serve     Start OI-bootstrapped HTTP server")
}
