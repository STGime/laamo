// Command mailin receives email for helper addresses on in.laamo.app and queues runs.
package main

import (
	"log/slog"
	"os"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	log.Info("mailin: not implemented yet", "issue", "#43")
}
