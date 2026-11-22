// fakesystem serves the fake openccu-lite the page's development and the Playwright tests run
// against (scripts/dev-serve.sh).
package main

import (
	"flag"
	"log"
	"net/http"

	"github.com/openccu-lite/lite-addon-logic/internal/fakesys"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:18081", "the address to listen on")
	wake := flag.Int("wake-port", 0, "a loopback UDP port that takes Wake-on-LAN packets (0: none)")
	flag.Parse()
	s := fakesys.New()
	defer s.Close()
	if *wake != 0 {
		if err := s.ListenWake(*wake); err != nil {
			log.Fatalf("fakesystem: %v", err)
		}
	}
	s.SeedDevices()
	log.Printf("fakesystem: on %s", *addr)
	log.Fatal(http.ListenAndServe(*addr, s.Handler()))
}
