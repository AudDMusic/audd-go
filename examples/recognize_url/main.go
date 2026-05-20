// Recognize a song from a URL using the public test token.
//
//	go run examples/recognize_url/main.go
package main

import (
	"fmt"
	"log"

	audd "github.com/AudDMusic/audd-go"
)

func main() {
	client := audd.NewClient("test")
	defer func() { _ = client.Close() }()

	result, err := client.Recognize("https://audd.tech/example.mp3", nil)
	if err != nil {
		log.Fatal(err)
	}
	if result == nil {
		fmt.Println("no match")
		return
	}
	fmt.Printf("%s — %s\n", result.Artist, result.Title)
}
