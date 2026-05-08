// Configure streams: set the callback URL, add a stream, list streams.
//
//	go run examples/streams_setup/main.go
package main

import (
	"fmt"
	"log"

	audd "github.com/AudDMusic/audd-go"
)

func main() {
	client := audd.NewClient("test")
	defer func() { _ = client.Close() }()

	if err := client.Streams().SetCallbackUrl("https://audd.tech/empty/", nil); err != nil {
		log.Fatal(err)
	}
	if err := client.Streams().Add(audd.AddStreamRequest{
		URL: "https://npr-ice.streamguys1.com/live.mp3", RadioID: 1,
	}); err != nil {
		log.Fatal(err)
	}
	streams, err := client.Streams().List()
	if err != nil {
		log.Fatal(err)
	}
	for _, s := range streams {
		fmt.Printf("radio %d: %s (running=%v)\n", s.RadioID, s.URL, s.StreamRunning)
	}
}
