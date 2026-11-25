# stream_to_csv

Subscribes to a radio stream, longpolls its recognitions, and appends each
match as a row in a CSV file.

```sh
export AUDD_API_TOKEN=your-token
go run . "https://stream.example/live.m3u8"
go run . "https://stream.example/live.m3u8" --output recordings.csv --radio-id 99999
```

Columns: `received_at, radio_id, timestamp, score, artist, title, album, song_link`.
Header is written only when the output file is new.

How it works:

- Calls `Streams().GetCallbackUrl()`. AudD's longpoll won't deliver events
  unless a callback URL is set on the account, even though the events arrive
  via longpoll rather than that URL. If the server returns error #19 ("no
  callback URL configured"), this example sets the placeholder
  `https://audd.tech/empty/` so longpoll starts working. We track this so
  teardown knows whether to mention restoring it.
- Calls `Streams().Add({URL, RadioID})` to subscribe.
- Computes the longpoll category locally via `DeriveLongpollCategory(radioID)`
  and opens the iterator with `Streams().LongpollContext(ctx, category, nil)`.
  Longpoll uses the context form because we need cancellation: when SIGINT or
  SIGTERM fires, the context ends the poll and its channels close.
- Each typed match from `poll.Matches` becomes CSV rows (one per candidate
  song); notifications from `poll.Notifications` (stream stopped, can't
  connect, etc.) go to stderr. The loop exits when `poll.Errors` closes,
  logging the error first if the poll stopped on one.
- The CSV writer flushes after every row, so a `kill -9` still leaves a
  valid file.
- On exit: `Streams().Delete(radioID)`. If we set the placeholder callback
  URL, we leave it in place — the AudD API has no "unset" verb, and
  flipping it back to nothing isn't an option.

The module has its own `go.mod` so the SDK module doesn't take on any deps
beyond `audd-go` itself.
