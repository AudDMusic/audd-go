// Receive AudD stream callbacks via a net/http handler.
//
//	go run examples/streams_callback_handler/main.go
package main

import (
	"fmt"
	"log"
	"net/http"

	audd "github.com/AudDMusic/audd-go"
)

func main() {
	http.HandleFunc("/audd-callback", func(w http.ResponseWriter, r *http.Request) {
		match, notif, err := audd.HandleCallback(r)
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		switch {
		case match != nil:
			fmt.Printf("recognized: %s — %s (radio %d, score %d)\n",
				match.Song.Artist, match.Song.Title, match.RadioID, match.Song.Score)
			for _, alt := range match.Alternatives {
				fmt.Printf("  alt: %s — %s\n", alt.Artist, alt.Title)
			}
		case notif != nil:
			fmt.Printf("notification %d: %s\n", notif.NotificationCode, notif.NotificationMessage)
		}
		w.WriteHeader(http.StatusOK)
	})
	log.Println("listening on :8080")
	if err := http.ListenAndServe(":8080", nil); err != nil { // nolint:gosec
		log.Fatal(err)
	}
}
