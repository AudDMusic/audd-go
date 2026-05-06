// Recognize a long file via the enterprise endpoint. Returns all matches
// (multiple chunks may match different songs).
//
//	go run examples/recognize_enterprise/main.go https://example.com/long.mp3
package main

import (
	"fmt"
	"log"
	"os"

	audd "github.com/AudDMusic/audd-go"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatalf("usage: %s <url-or-path>", os.Args[0])
	}
	client := audd.NewClient("test")
	defer func() { _ = client.Close() }()

	limit := 1 // hard rule: examples pass limit=1 during dev
	matches, err := client.RecognizeEnterprise(os.Args[1], &audd.EnterpriseOptions{
		Limit: &limit,
	})
	if err != nil {
		log.Fatal(err)
	}
	for i, m := range matches {
		fmt.Printf("%d. %s — %s (timecode %s)\n", i+1, m.Artist, m.Title, m.Timecode)
	}
}
