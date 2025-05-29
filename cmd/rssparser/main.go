package main

import (
	"agregator/rss/internal/pkg/app"
	"log/slog"
)

func main() {
	logger := slog.Default()
	app.New(logger).Run()
}
