// Subscribe to a radio stream, longpoll its recognitions, and append each
// match to a CSV file. On SIGINT/SIGTERM, deletes the stream slot and exits.
//
//	export AUDD_API_TOKEN=aud_xxx
//	go run . "https://stream.example/live.m3u8"
//	go run . "https://stream.example/live.m3u8" --output recordings.csv --radio-id 99999
//
// AudD requires a callback URL to be set on the account before longpoll will
// deliver events. If the account has none, this example sets it to
// https://audd.tech/empty/ on startup. The AudD API has no "unset" verb, so
// the placeholder is left in place on exit (and only logged).
package main

import (
	"context"
	"encoding/csv"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	audd "github.com/AudDMusic/audd-go"
)

const placeholderCallbackURL = "https://audd.tech/empty/"

var csvHeader = []string{"received_at", "radio_id", "timestamp", "score", "artist", "title", "album", "song_link"}

func main() {
	output := flag.String("output", "recordings.csv", "CSV file to append matches to.")
	radioID := flag.Int("radio-id", 99999, "radio_id slot to use for this subscription.")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [--output recordings.csv] [--radio-id N] <stream-url>\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(2)
	}
	streamURL := flag.Arg(0)

	client := audd.NewClient(os.Getenv("AUDD_API_TOKEN"))
	defer func() { _ = client.Close() }()

	// 1. Make sure a callback URL is set. Longpoll won't deliver events
	//    without one. Track whether we set the placeholder so teardown can
	//    log that on exit.
	priorCallback, weSetCallback, err := ensureCallbackURL(client)
	if err != nil {
		log.Fatalf("ensure callback url: %v", err)
	}

	// 2. Subscribe to the stream.
	if err := client.Streams().Add(audd.AddStreamRequest{
		URL: streamURL, RadioID: *radioID,
	}); err != nil {
		log.Fatalf("add stream %d: %v", *radioID, err)
	}
	log.Printf("subscribed radio_id=%d to %s", *radioID, streamURL)

	// 3. Open the CSV. Append mode; write header only if we're creating a
	//    new file. Flushed after every row so a kill -9 still leaves a
	//    valid file.
	csvFile, csvWriter, err := openCSV(*output)
	if err != nil {
		log.Fatalf("open csv %q: %v", *output, err)
	}
	defer func() { _ = csvFile.Close() }()

	// 4. Wire SIGINT / SIGTERM to a context that stops the longpoll loop.
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// 5. Open the longpoll subscription. Three typed streams: matches,
	//    notifications, errors.
	category := client.Streams().DeriveLongpollCategory(*radioID)
	poll, err := client.Streams().LongpollContext(ctx, category, nil)
	if err != nil {
		_ = teardown(client, *radioID, priorCallback, weSetCallback)
		log.Fatalf("longpoll: %v", err)
	}
	defer func() { _ = poll.Close() }()
	log.Printf("longpolling category=%s — Ctrl-C to stop", category)

	// 6. Demux events. Matches go to CSV, notifications to stderr, errors are terminal.
loop:
	for {
		select {
		case m, ok := <-poll.Matches:
			if !ok {
				break loop
			}
			writeMatch(csvWriter, &m)
		case n, ok := <-poll.Notifications:
			if !ok {
				break loop
			}
			log.Printf("notification radio_id=%d code=%d %q",
				n.RadioID, n.NotificationCode, n.NotificationMessage)
		case err, ok := <-poll.Errors:
			if !ok {
				break loop
			}
			log.Printf("longpoll error: %v", err)
			break loop
		case <-ctx.Done():
			break loop
		}
	}

	if err := teardown(client, *radioID, priorCallback, weSetCallback); err != nil {
		log.Printf("teardown: %v", err)
	}
}

// ensureCallbackURL reads the current callback URL. If the server returns
// error #19 (no callback URL configured), this sets the placeholder. Returns
// the prior value (if any) and a flag indicating whether we set the
// placeholder ourselves (so teardown can mention it on exit).
func ensureCallbackURL(client *audd.Client) (prior string, weSet bool, err error) {
	prior, gErr := client.Streams().GetCallbackUrl()
	if gErr == nil {
		return prior, false, nil
	}
	var apiErr *audd.AuddAPIError
	if errors.As(gErr, &apiErr) && apiErr.ErrorCode == 19 {
		log.Printf("no callback URL configured; setting placeholder %s", placeholderCallbackURL)
		if sErr := client.Streams().SetCallbackUrl(placeholderCallbackURL, nil); sErr != nil {
			return "", false, fmt.Errorf("set placeholder callback: %w", sErr)
		}
		return "", true, nil
	}
	return "", false, gErr
}

// teardown removes the stream slot. If we set the placeholder callback URL
// ourselves, we don't restore (there's nothing to restore TO — the prior
// state was "no URL configured", and the API has no "unset" verb).
func teardown(client *audd.Client, radioID int, prior string, weSet bool) error {
	log.Printf("removing radio_id=%d", radioID)
	if err := client.Streams().Delete(radioID); err != nil {
		return fmt.Errorf("delete stream: %w", err)
	}
	if weSet {
		// We set the placeholder; the prior state was "unset". The AudD API
		// has no "unset callback URL" verb, so we leave the placeholder in
		// place and just note it.
		log.Printf("note: callback URL left at placeholder %s (no API to unset)", placeholderCallbackURL)
		return nil
	}
	if prior != "" {
		// Defensive: prior was set, leave it as-is (we never changed it).
		log.Printf("callback URL unchanged: %s", prior)
	}
	return nil
}

// openCSV opens output in append mode, writing a header if the file is new.
// Writer is flushed by the caller after each append.
func openCSV(path string) (*os.File, *csv.Writer, error) {
	existing, statErr := os.Stat(path)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, nil, err
	}
	w := csv.NewWriter(f)
	isNew := os.IsNotExist(statErr) || (statErr == nil && existing.Size() == 0)
	if isNew {
		if err := w.Write(csvHeader); err != nil {
			_ = f.Close()
			return nil, nil, err
		}
		w.Flush()
		if err := w.Error(); err != nil {
			_ = f.Close()
			return nil, nil, err
		}
	}
	return f, w, nil
}

// writeMatches appends one CSV row per recognition entry. Most callbacks have
// exactly one entry, but the schema is an array, so we honor it.
// writeMatch appends a CSV row for the top song of a match, plus one row per
// alternative candidate (rare — only when the fingerprint resolves to multiple
// near-identical catalog records, which may have a different artist or title).
func writeMatch(w *csv.Writer, m *audd.StreamCallbackMatch) {
	writeSong(w, m, &m.Song)
	for i := range m.Alternatives {
		writeSong(w, m, &m.Alternatives[i])
	}
}

func writeSong(w *csv.Writer, m *audd.StreamCallbackMatch, s *audd.StreamCallbackSong) {
	now := time.Now().UTC().Format(time.RFC3339)
	row := []string{
		now,
		strconv.FormatInt(m.RadioID, 10),
		m.Timestamp,
		strconv.Itoa(s.Score),
		s.Artist,
		s.Title,
		s.Album,
		s.SongLink,
	}
	if err := w.Write(row); err != nil {
		log.Printf("csv write: %v", err)
		return
	}
	w.Flush()
	if err := w.Error(); err != nil {
		log.Printf("csv flush: %v", err)
		return
	}
	log.Printf("logged %s — %s (radio_id=%d)", s.Artist, s.Title, m.RadioID)
}
