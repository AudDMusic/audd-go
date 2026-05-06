// Add a song to your private fingerprint catalog.
//
// **This is NOT how you submit audio for music recognition.** For
// recognition, use client.Recognize (or client.RecognizeEnterprise for files
// longer than 25 seconds). This example demonstrates the custom-catalog
// upload — for adding YOUR OWN tracks to YOUR private fingerprint database.
// Requires special access; contact api@audd.io.
//
//	go run examples/custom_catalog_add/main.go <audio_id> <file>
package main

import (
	"fmt"
	"log"
	"os"
	"strconv"

	audd "github.com/AudDMusic/audd-go"
)

func main() {
	if len(os.Args) < 3 {
		log.Fatalf("usage: %s <audio_id> <file>", os.Args[0])
	}
	id, err := strconv.Atoi(os.Args[1])
	if err != nil {
		log.Fatal(err)
	}
	client := audd.NewClient("test")
	defer func() { _ = client.Close() }()

	if err := client.CustomCatalog().Add(id, os.Args[2]); err != nil {
		log.Fatal(err)
	}
	fmt.Println("added")
}
