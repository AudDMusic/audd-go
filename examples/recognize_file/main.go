// Recognize a song from a local file. Pass the file path as an argument:
//
//	go run examples/recognize_file/main.go path/to/song.mp3
package main

import (
	"fmt"
	"log"
	"os"

	audd "github.com/AudDMusic/audd-go"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatalf("usage: %s <file-path>", os.Args[0])
	}
	client := audd.NewClient("test")
	defer func() { _ = client.Close() }()

	result, err := client.Recognize(os.Args[1], &audd.RecognizeOptions{
		ReturnMetadata: "apple_music,spotify",
	})
	if err != nil {
		log.Fatal(err)
	}
	if result == nil {
		fmt.Println("no match")
		return
	}
	fmt.Printf("%s — %s\n", result.Artist, result.Title)
}
