// Side-by-side: v0 deprecated flat API vs v1 namespaced API.
//
//	go run examples/migration_v0_to_v1/main.go
package main

import (
	"fmt"
	"log"

	audd "github.com/AudDMusic/audd-go"
)

func main() {
	client := audd.NewClient("test")
	defer func() { _ = client.Close() }()

	// ---- v0 (deprecated, still works) ----------------------------------
	songV0, err := client.RecognizeByUrl("https://audd.tech/example.mp3", "apple_music", nil)
	if err != nil {
		log.Fatal(err)
	}
	if songV0 != nil {
		fmt.Printf("v0: %s — %s\n", songV0.Artist, songV0.Title)
	}

	// ---- v1 (preferred) ------------------------------------------------
	resultV1, err := client.Recognize("https://audd.tech/example.mp3", &audd.RecognizeOptions{
		Return: []string{"apple_music"},
	})
	if err != nil {
		log.Fatal(err)
	}
	if resultV1 != nil {
		fmt.Printf("v1: %s — %s\n", resultV1.Artist, resultV1.Title)
	}

	// Streams: v0 → v1 ---------------------------------------------------
	// Old:    client.AddStream("twitch:foo", 7, "before", nil)
	// New:    client.Streams().Add(audd.AddStreamRequest{URL: "twitch:foo", RadioID: 7, Callbacks: "before"})

	// FindLyrics: v0 → v1 ------------------------------------------------
	// Old:    client.FindLyrics("hello", nil)
	// New:    client.Advanced().FindLyrics("hello")
}
