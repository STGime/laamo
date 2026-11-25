// Command vapid prints a new VAPID key pair for notifications (Web Push),
// as lines for .env and deploy/<env>.env:
//
//	go run ./cmd/vapid
//
// Make one pair and use it everywhere that shares a database. A new pair
// cuts off every browser that subscribed with the old one.
package main

import (
	"fmt"
	"os"

	webpush "github.com/SherClockHolmes/webpush-go"
)

func main() {
	private, public, err := webpush.GenerateVAPIDKeys()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("VAPID_PUBLIC_KEY=%s\nVAPID_PRIVATE_KEY=%s\n", public, private)
}
