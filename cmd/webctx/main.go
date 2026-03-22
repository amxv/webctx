package main

import (
	"os"

	"github.com/amxv/webctx/internal/app"
)

func main() {
	os.Exit(app.Run(os.Args[1:], os.Stdout, os.Stderr))
}
