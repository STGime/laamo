// Command worker runs the job queue: run engine, outbound messages and digests.
package main

import (
	"log/slog"
	"os"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	log.Info("worker: not implemented yet", "issue", "#9, #18")
}
