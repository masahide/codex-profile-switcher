package main

import (
	"context"
	"os"

	"github.com/masahide/codex-profile-switcher/internal/app"
)

const version = "dev"

func main() {
	code := app.New(version).Run(context.Background(), os.Args[1:])
	if code != 0 {
		os.Exit(code)
	}
}
