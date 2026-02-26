// Package main is the entry point for the claudewatch CLI.
package main

import "github.com/blackwell-systems/claudewatch/internal/app"

// version is set at build time via ldflags:
//
//	go build -ldflags "-X main.version=1.0.0"
var version = "dev"

func main() {
	app.SetVersion(version)
	app.Execute()
}
