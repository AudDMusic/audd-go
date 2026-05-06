// Receive AudD stream callbacks via a net/http handler.
//
//	go run examples/streams_callback_handler/main.go
package main

import (
	"fmt"
	"io"
	"log"
	"net/http"

	audd "github.com/AudDMusic/audd-go"
)

func main() {
	http.HandleFunc("/audd-callback", func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		payload, err := audd.ParseCallback(body)
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		switch {
		case payload.IsResult():
			for _, m := range payload.Result.Results {
				fmt.Printf("recognized: %s — %s\n", m.Artist, m.Title)
			}
		case payload.IsNotification():
			fmt.Printf("notification %d: %s\n",
				payload.Notification.NotificationCode, payload.Notification.NotificationMessage)
		}
		w.WriteHeader(http.StatusOK)
	})
	log.Println("listening on :8080")
	if err := http.ListenAndServe(":8080", nil); err != nil { // nolint:gosec
		log.Fatal(err)
	}
}
