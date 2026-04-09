//go:build headless

package main

import (
	"log"
)

func main() {
	log.Println("FoxTrack Bridge (headless dev build) starting...")
	StartServer()
}
