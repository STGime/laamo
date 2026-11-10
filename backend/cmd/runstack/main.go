// Command runstack runs a stack JSON locally against fixtures or live APIs (dev and evals).
package main

import (
	"log/slog"
	"os"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	log.Info("runstack: not implemented yet", "issue", "#28")
}
