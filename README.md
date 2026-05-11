# audd-go

[![CI](https://github.com/AudDMusic/audd-go/actions/workflows/ci.yml/badge.svg)](https://github.com/AudDMusic/audd-go/actions/workflows/ci.yml)
[![Contract](https://github.com/AudDMusic/audd-go/actions/workflows/contract.yml/badge.svg)](https://github.com/AudDMusic/audd-go/actions/workflows/contract.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/AudDMusic/audd-go.svg)](https://pkg.go.dev/github.com/AudDMusic/audd-go)
[![Go Report Card](https://goreportcard.com/badge/github.com/AudDMusic/audd-go)](https://goreportcard.com/report/github.com/AudDMusic/audd-go)

Official Go SDK for [music recognition API](https://audd.io): identify music from a short audio clip, a long audio file, or a live stream.

The API itself is so simple that it can easily be used even without an SDK: [docs.audd.io](https://docs.audd.io).

## Hello, AudD

```sh
go get github.com/AudDMusic/audd-go
```

Get your API token at [dashboard.audd.io](https://dashboard.audd.io).

Identify a song hosted at a URL:

```go
package main

import (
    "fmt"
    "log"

    audd "github.com/AudDMusic/audd-go"
)

func main() {
    client := audd.NewClient("your-api-token")
    defer client.Close()

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
```

Identify a song from a local file path:

```go
result, err := client.Recognize("/path/to/clip.mp3", nil)
```

`Recognize` accepts a URL string, a file-path string, `[]byte`, or any
`io.Reader`. For files longer than ~25 seconds, use
`client.RecognizeEnterprise(source, &audd.EnterpriseOptions{Limit: ptr(1)})`,
which returns `[]EnterpriseMatch` across the file's chunks. Each match
carries the same core tags plus `Score`, `StartOffset`, `EndOffset`,
`ISRC`, `UPC`. Access to `ISRC`, `UPC`, and `Score` requires a Startup
plan or higher — [contact us](mailto:api@audd.io) for enterprise features.

Every method has an explicit-context twin: `RecognizeContext(ctx, source, opts)`,
`Streams().AddContext(ctx, req)`, `Advanced().FindLyricsContext(ctx, query)`,
and so on. The plain form is the default for short scripts; the
`*Context` form is what you reach for in servers and pipelines that need
cancellation, deadlines, or a context-propagated tracing span.

Requires Go 1.21+.

## Authentication

Pass your token to `NewClient`:

```go
client := audd.NewClient("your-api-token")
```

Or leave it empty and the SDK reads the `AUDD_API_TOKEN` environment variable:

```go
// AUDD_API_TOKEN=your-token ...
client := audd.NewClient("")
```

Get a real token at [dashboard.audd.io](https://dashboard.audd.io). The
public `"test"` token works for the snippets above but is capped at 10
requests.

For long-running services that pull tokens from a secret manager and need to
swap them without restarting:

```go
if err := client.SetAPIToken(newToken); err != nil {
    log.Fatal(err)
}
```

In-flight requests continue with the previous token; subsequent ones use the
new value.

If you'd rather fail fast at construction time when no token is configured,
use `NewClientStrict`, which returns `audd.ErrMissingAPIToken`.

## What you get back

By default `Recognize` returns the core tags plus AudD's universal song link —
no metadata-block opt-in needed:

```go
result, err := client.Recognize("https://audd.tech/example.mp3", nil)
if err != nil { log.Fatal(err) }
if result == nil { return } // no match

fmt.Println(result.Artist, "—", result.Title)
fmt.Println("Album:        ", result.Album)
fmt.Println("Released:     ", result.ReleaseDate)
fmt.Println("Label:        ", result.Label)
fmt.Println("AudD song:    ", result.SongLink) // links into every provider

// Helpers, driven off SongLink — work without any Return opt-in:
fmt.Println("Cover art:    ", result.ThumbnailURL())
fmt.Println("On Spotify:   ", result.StreamingURL(audd.ProviderSpotify))
for provider, url := range result.StreamingURLs() {
    fmt.Printf("  %s -> %s\n", provider, url)
}
```

If you need provider-specific metadata blocks, opt in per call. Request only
what you need — each provider you ask for adds latency:

```go
result, _ := client.Recognize("https://audd.tech/example.mp3", &audd.RecognizeOptions{
    Return: "apple_music,spotify",
})
fmt.Println("Apple Music:", result.AppleMusic.URL)
fmt.Println("Spotify URI:", result.Spotify.URI)
fmt.Println("Preview:    ", result.PreviewURL())  // first preview across requested providers, "" if none
```

Valid `Return` values: `apple_music`, `spotify`, `deezer`, `napster`,
`musicbrainz`. The metadata-block fields (`AppleMusic`, `Spotify`, `Deezer`,
`Napster`) are pointers and may be nil; `MusicBrainz` is a slice — guard
accordingly.

### Reading additional metadata

`result.Extras` is a `map[string]json.RawMessage` of every server field outside
the typed surface; `result.RawResponse` is the original JSON object. Use them
to read undocumented metadata or beta fields not yet exposed as typed
properties:

```go
if raw, ok := result.Extras["song_length"]; ok {
    var seconds int
    _ = json.Unmarshal(raw, &seconds)
    fmt.Println("song length:", seconds)
}

// Same channel exists on every metadata block:
if raw, ok := result.AppleMusic.Extras["genreNames"]; ok {
    var genres []string
    _ = json.Unmarshal(raw, &genres)
}
```

## Errors

Match by category with sentinel errors:

```go
import "errors"

result, err := client.Recognize(source, nil)
switch {
case errors.Is(err, audd.ErrAuthentication):
    // 900 / 901 / 903 — token problems
case errors.Is(err, audd.ErrQuota):
    // 902 — quota exceeded
case errors.Is(err, audd.ErrInvalidAudio):
    // 300 / 400 / 500 — audio is the problem
case errors.Is(err, audd.ErrRateLimit):
    // 611 — back off and retry
case errors.Is(err, audd.ErrServer):
    // 5xx, non-JSON gateway responses
case err != nil:
    log.Println("audd:", err)
}
```

For the underlying numeric code, message, and request ID, extract the typed
error:

```go
var apiErr *audd.AudDAPIError
if errors.As(err, &apiErr) {
    log.Printf("[#%d] %s (request_id=%s)", apiErr.ErrorCode, apiErr.Message, apiErr.RequestID)
}
```

Sentinels: `ErrAuthentication`, `ErrQuota`, `ErrSubscription`,
`ErrCustomCatalogAccess`, `ErrInvalidRequest`, `ErrInvalidAudio`,
`ErrRateLimit`, `ErrStreamLimit`, `ErrNotReleased`, `ErrBlocked`,
`ErrNeedsUpdate`, `ErrServer`, `ErrConnection`, `ErrSerialization`.

## Configuration

```go
client := audd.NewClient("your-api-token",
    audd.WithMaxAttempts(5),
    audd.WithBackoffFactor(time.Second),
    audd.WithStandardTimeout(2*time.Minute),
    audd.WithEnterpriseTimeout(2*time.Hour),
    audd.WithHTTPClient(&http.Client{ /* corporate proxy, mTLS, etc. */ }),
    audd.WithOnEvent(func(e audd.AudDEvent) { /* see Observability */ }),
    audd.WithOnDeprecation(func(msg string) {
        // Server-side soft-deprecation warnings (code 51) land here.
        slog.Warn("audd-deprecation", "msg", msg)
    }),
)
```

Retries are cost-aware:

- Read endpoints retry on 408/429/5xx and any net error.
- Recognition endpoints retry only on 5xx and pre-upload connection failures
  (DNS, TCP dial). Post-upload errors are not retried — the server may have
  already done the metered work.
- Mutating endpoints retry only on pre-upload connection failures.

## Observability

`WithOnEvent` receives a structured `AudDEvent` for every `request` /
`response` / `exception` on the wire — method/URL/status/elapsed/request_id,
no api_token, no body bytes. It pairs cleanly with the standard library's
[`log/slog`](https://pkg.go.dev/log/slog), so every API call shows up as a
structured JSON log record. See
[`examples/observability_slog`](./examples/observability_slog) for a ~50-line
drop-in.

## Streams

Stream recognition turns AudD into a continuous monitor for an audio stream
(internet radio, Twitch, YouTube live, raw HLS/Icecast) and notifies you for
every recognized song. Set up streams once, then either receive matches via a
callback URL or poll for them.

```go
streams := client.Streams()

// 1. Tell AudD where to POST recognition results for your account.
streams.SetCallbackUrl("https://your.app/audd/callback", &audd.SetCallbackUrlOptions{
    ReturnMetadata: "apple_music,spotify",
})

// 2. Add streams to monitor.
streams.Add(audd.AddStreamRequest{
    URL: "https://example.com/radio.m3u8", RadioID: 1,
})
streams.Add(audd.AddStreamRequest{URL: "twitch:somechannel", RadioID: 2})

// 3. Inspect what you have configured.
list, _ := streams.List()
for _, s := range list {
    fmt.Println(s.RadioID, s.URL, "running:", s.StreamRunning)
}
```

Inside your callback receiver, parse the POST body into a typed payload:

```go
http.HandleFunc("/audd/callback", func(w http.ResponseWriter, r *http.Request) {
    match, notif, err := audd.HandleCallback(r)
    if err != nil {
        http.Error(w, err.Error(), http.StatusBadRequest)
        return
    }
    if match != nil {
        fmt.Println("matched:", match.Song.Artist, "—", match.Song.Title)
        for _, alt := range match.Alternatives {
            fmt.Println("  alt:", alt.Artist, "—", alt.Title)
        }
    }
    if notif != nil {
        fmt.Println("notification:", notif.NotificationMessage)
    }
})
```

`HandleCallback(r)` reads the body and parses it. Use `audd.ParseCallback(body)` instead if you already have the bytes (queue consumer, replay tool).

See [`examples/streams_callback_handler`](./examples/streams_callback_handler)
and [`examples/streams_setup`](./examples/streams_setup) for runnable code.

### Receiving events without a callback URL (longpoll)

If hosting a callback receiver isn't an option, longpoll for events from the
client side. Three typed channels — Matches, Notifications, Errors — drive a
`select` loop:

```go
ctx, cancel := context.WithCancel(context.Background())
defer cancel()

radioID := 1 // any integer you choose — your handle for this stream
poll, err := streams.LongpollByRadioIDContext(ctx, radioID, nil)
if err != nil { log.Fatal(err) }
defer poll.Close()

for {
    select {
    case m := <-poll.Matches:
        fmt.Println("matched:", m.Song.Artist, "—", m.Song.Title)
    case n := <-poll.Notifications:
        fmt.Println("notification:", n.NotificationMessage)
    case err := <-poll.Errors:
        log.Fatal(err)
    case <-ctx.Done():
        return
    }
}
```

For browser widgets, embedded UIs, or anywhere you need to consume a category
without leaking the API token, use the tokenless variant — same channels,
same select loop:

```go
consumer := audd.NewLongpollConsumer(category)
defer consumer.Close()
poll := consumer.Iterate(nil)
defer poll.Close()
// then the same select loop as above
```

`audd.DeriveLongpollCategory(token, radioID)` is also available as a package
function for computing categories on a server and shipping them to a frontend
that runs `NewLongpollConsumer`.

## Migrating from v0.x

The v0 flat methods (`RecognizeByUrl`, `AddStream`, `FindLyrics`, …) still
work as deprecated wrappers and will be removed in v2.0.0. The current API is
namespaced:

| v0 | v1 |
|---|---|
| `client.RecognizeByUrl(url, "apple_music", nil)` | `client.Recognize(url, &audd.RecognizeOptions{Return: "apple_music"})` |
| `client.RecognizeByFile(reader, "", nil)` | `client.Recognize(reader, nil)` |
| `client.AddStream(url, 7, "before", nil)` | `client.Streams().Add(audd.AddStreamRequest{URL: url, RadioID: 7, Callbacks: "before"})` |
| `client.FindLyrics(query, nil)` | `client.Advanced().FindLyrics(query)` |
| `client.SetCallbackUrl(url)` | `client.Streams().SetCallbackUrl(url, nil)` |
| `client.GetStreams()` | `client.Streams().List()` |
| `client.AddSongToCustomDB(id, src)` | `client.CustomCatalog().Add(id, src)` |

See [`examples/migration_v0_to_v1`](./examples/migration_v0_to_v1) for a
side-by-side runnable example.

## Custom catalog (advanced)

> [!WARNING]
> The custom-catalog endpoint is **not** music recognition. It adds songs to
> your account's **private fingerprint database**, so AudD's recognition can
> later identify *your own* tracks for *your account only*. If you intended
> to identify music, use `client.Recognize` (or `client.RecognizeEnterprise`
> for files longer than 25 seconds) instead.

```go
err := client.CustomCatalog().Add(audioID, "/path/to/track.mp3")
```

Custom-catalog access requires a separate subscription. Contact api@audd.io
to get it enabled.

## Advanced

`client.Advanced()` exposes the typed `FindLyrics(query)` method plus a
generic raw-method escape hatch for AudD endpoints not yet wrapped on this
SDK:

```go
body, err := client.Advanced().RawRequest("newMethodName", map[string]string{
    "param": "value",
})
```

## License & support

MIT — see [LICENSE](./LICENSE). Security policy: [SECURITY.md](./SECURITY.md).

Bug reports and PRs welcome. For account / API questions, email
api@audd.io.
