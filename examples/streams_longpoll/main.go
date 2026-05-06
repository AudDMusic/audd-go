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

	for ev := range consumer.IterateContext(ctx, nil) {
		if ev.Err != nil {
			log.Fatal(ev.Err)
		}
		fmt.Printf("event: %v\n", ev.Body)
	}
}
