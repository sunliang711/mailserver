package main

import (
	"flag"
	"os"

	"github.com/sunliang711/mailserver/internal/app"
)

var version = "dev"

func main() {
	configPath := flag.String("config", "", "path to config.yaml")
	port := flag.String("port", "", "override listen port (e.g. 3000)")
	dataDir := flag.String("data-dir", "", "override data directory (default ./data)")
	masterKey := flag.String("master-key", "", "override 32-byte master key (64 hex chars or base64)")
	basePath := flag.String("base-path", "", "override URL base path prefix (e.g. /api/mail)")
	flag.Parse()

	overrides := map[string]string{}
	if *port != "" {
		overrides["port"] = *port
	}
	if *dataDir != "" {
		overrides["data-dir"] = *dataDir
	}
	if *masterKey != "" {
		overrides["master-key"] = *masterKey
	}
	if *basePath != "" {
		overrides["base-path"] = *basePath
	}

	if *configPath == "" && len(overrides) == 0 && len(os.Args) == 1 {
		app.NewWithBootstrap("", nil, version).Run()
		return
	}
	app.NewWithBootstrap(*configPath, overrides, version).Run()
}
