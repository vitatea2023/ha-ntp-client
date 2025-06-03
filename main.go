package main

import (
	"log"
	"os"
)

func main() {
	if err := Execute(); err != nil {
		log.SetOutput(os.Stderr)
		log.Printf("❌ Error: %v", err)
		os.Exit(1)
	}
}