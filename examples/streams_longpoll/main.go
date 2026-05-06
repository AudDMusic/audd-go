// Subscribe to longpoll events for a category. Use the tokenless consumer
// for browser/widget code; use client.Streams().Longpoll(category, opts) when
// you have a token (and want the no-callback-URL preflight).
//
//	go run examples/streams_longpoll/main.go <category>
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	audd "github.com/AudDMusic/audd-go"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatalf("usage: %s <category>", os.Args[0])
	}
	consumer := audd.NewLongpollConsumer(os.Args[1])
	defer func() { _ = consumer.Close() }()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	poll := consumer.IterateContext(ctx, nil)
	defer func() { _ = poll.Close() }()

	for {
		select {
		case m, ok := <-poll.Matches:
			if !ok {
				return
			}
			fmt.Printf("recognized: %s — %s (radio %d, score %d)\n",
				m.Song.Artist, m.Song.Title, m.RadioID, m.Song.Score)
		case n, ok := <-poll.Notifications:
			if !ok {
				return
			}
			fmt.Printf("notification %d: %s\n", n.NotificationCode, n.NotificationMessage)
		case err := <-poll.Errors:
			if err != nil {
				log.Fatal(err)
			}
			return
		case <-ctx.Done():
			return
		}
	}
}
