# Changelog

All notable changes to this project are documented here.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [1.5.19] - 2026-07-12

### Changed

- Lenient parsing now coerces convertible wrong-typed scalars instead of
  always zeroing them: a numeric string parses into a number field
  (`{"score":"85"}` → 85, floats truncate), numbers and bools render into
  string fields, numbers map into bool fields (`!= 0`), and recognized
  boolean strings ("true"/"1"/"yes"/"on", "false"/"0"/"no"/"off"/"",
  case-insensitive) map into bool fields. Unconvertible values (garbage
  strings, wrong-shaped objects/arrays) still degrade to the zero value —
  never an error, never a misleading zero from a partial parse. Numeric
  strings parse strictly: the whole trimmed string must be a finite number.

## [1.5.18] - 2026-07-12

### Fixed

- Response parsing is now lenient about wrong-typed fields: a field whose
  wire value has an unexpected type (e.g. a string `score`, a numeric
  `timecode`) degrades to its zero value instead of failing the whole call.
  This applies across `Recognition`, `EnterpriseMatch`, `Stream`,
  `LyricsResult`, the provider metadata blocks, and stream
  callbacks/longpoll payloads.
- Fixed data races: the `Streams()` / `CustomCatalog()` / `Advanced()`
  sub-clients are initialized when the `Client` is constructed (concurrent
  first accesses were racy), and `StreamsClient.DeriveLongpollCategory` now
  reads the API token through the guarded accessor, so it is safe alongside
  `SetAPIToken`.
- `RecognizeOptions.Timeout` and `EnterpriseOptions.Timeout` are now applied:
  each bounds its call with a per-call deadline. Previously the fields were
  accepted but ignored.
- File-path sources no longer leak a file handle per request attempt: the
  transport closes the file it opened once the attempt finishes.
  Caller-supplied `io.Reader` sources are never closed by the SDK.
- The v0 compatibility shims no longer drop options: `RecognizeByUrl` /
  `RecognizeByFile` forward all legacy option keys (not just `market`), and
  the `FindLyrics` / `AddStream` shims forward their options maps as extra
  form fields.
- Longpoll requests are sized to the poll timeout plus a margin, so
  `LongpollOptions.Timeout` values above 60 seconds work instead of being
  cut short by the standard HTTP timeout. Applies to the authenticated
  poller and the tokenless `LongpollConsumer`.
- The longpoll preflight only reports "no callback URL is configured" when
  the server's error #19 actually indicates that condition; other #19
  responses (e.g. maintenance) pass through unchanged. The
  `GetCallbackUrl` doc comment now describes the real error mapping.
- `Advanced().FindLyrics` errors now carry the real HTTP status and request
  ID, and code-51 deprecation warnings pass through with the result, same
  as every other endpoint.
- `Recognition.StreamingURL`, `StreamingURLs`, and `PreviewURL` now resolve
  direct provider URLs and previews from the decoded metadata blocks
  (`apple_music`, `spotify`, `deezer`, `napster`).

### Changed

- The `OnEvent` inspection hook now fires for every API call — enterprise
  recognition, stream management, custom catalog uploads, and
  `Advanced()` requests — not only `Recognize`.

## [1.5.17] - 2026-06-01

- `EnterpriseMatch` gains `StartSeconds` / `EndSeconds`: file-absolute match
  positions computed from the chunk offset plus the fragment-relative
  `start_offset` / `end_offset`. `nil` when the chunk carried no usable offset.
- Enterprise requests send `accurate_offsets=true` by default; set
  `EnterpriseOptions.AccurateOffsets` to a non-nil `false` to opt out.

## [1.5.16] - 2026-05-29

- Stream callbacks and longpoll matches whose `results` array is absent or
  empty parse without error: `Song` stays its zero value and `Alternatives`
  is empty.

## [1.5.15] - 2026-05-18

- Documentation: option docs updated to the `ReturnMetadata` name. No API
  changes.

## [1.5.14] - 2026-05-18

- The `Return` option on `RecognizeOptions` / `EnterpriseOptions` is renamed
  `ReturnMetadata`.

## [1.5.13] - 2026-05-11

- New `ExtraParameters map[string]string` on `RecognizeOptions`,
  `EnterpriseOptions`, `AddStreamRequest`, and `SetCallbackUrlOptions`:
  pass any form field the typed options don't cover. Typed fields win on
  collision.

## [1.5.12] - 2026-05-11

- The `Return` option is now a comma-separated string (e.g.
  `"apple_music,spotify"`) instead of a `[]string`. New `JoinProviders`
  helper builds the string from a list.
- `SetCallbackUrlOptions.ReturnMetadata` appends `?return=<csv>` to the
  callback URL; a URL that already carries a `return` parameter combined
  with a non-empty `ReturnMetadata` returns an error instead of silently
  overwriting.

## [1.5.11] - 2026-05-09

- Adds the `go.mod` retract directive for v1.5.10. Public API matches
  v1.5.9.

## [1.5.10] - 2026-05-09

- Retracted — use v1.5.11. Public API matches v1.5.9.

## [1.5.9] - 2026-05-08

- `CustomCatalog.Add` no longer retries by default.

## [1.5.8] - 2026-05-08

- New `LongpollByRadioID` / `LongpollByRadioIDContext`: one-step longpoll
  subscription from a radio ID — the SDK derives the category from the
  client's token locally.
- The typed error and event symbols are renamed `Audd*` → `AudD*`
  (`AudDAPIError`, `AudDConnectionError`, `AudDSerializationError`,
  `AudDCustomCatalogAccessError`, `AudDEvent`). The old names remain as
  deprecated aliases until v2.0.0.

## [1.5.6] - 2026-05-06

- README: the quickstart constructor uses a `your-api-token` placeholder
  with a pointer to dashboard.audd.io.

## [1.5.5] - 2026-05-06

- README: CI and contract badges.

## [1.5.4] - 2026-05-06

- LICENSE copyright line updated to "AudD, LLC (https://audd.io)".

## [1.5.2], [1.5.3] - 2026-05-06

- README and example polish. No API changes.

## [1.5.1] - 2026-05-06

- Internal cleanup of `LongpollPoll`. No API changes.

## [1.5.0] - 2026-05-06

- Longpoll rework: `Streams().Longpoll(...)` and `LongpollConsumer.Iterate(...)`
  return a `*LongpollPoll` with typed `Matches` / `Notifications` / `Errors`
  channels (previously a single event-channel).
- Callback types renamed: `StreamCallbackMatch` (with `Song` +
  `Alternatives`) and `StreamCallbackSong` replace the previous
  result/payload shapes. `ParseCallback` / `HandleCallback` are
  package-level functions.

## [1.4.6] - 2026-05-06

### Changed

- Public sub-client methods now follow the same Style B pattern as
  `Recognize`/`RecognizeContext`: every method has a no-context convenience
  form (`Foo(args)`) plus an explicit-context form (`FooContext(ctx, args)`).
  Callers passing `context.Background()` to the previous ctx-taking
  signatures should drop the argument; callers needing cancellation should
  switch to the new `*Context` variant.
  - `StreamsClient.SetCallbackUrl` / `SetCallbackUrlContext`
  - `StreamsClient.GetCallbackUrl` / `GetCallbackUrlContext`
  - `StreamsClient.Add` / `AddContext`
  - `StreamsClient.SetURL` / `SetURLContext`
  - `StreamsClient.Delete` / `DeleteContext`
  - `StreamsClient.List` / `ListContext`
  - `StreamsClient.Longpoll` / `LongpollContext`
  - `AdvancedClient.FindLyrics` / `FindLyricsContext`
  - `AdvancedClient.RawRequest` / `RawRequestContext`
  - `CustomCatalogClient.Add` / `AddContext`
  - `LongpollConsumer.Iterate` / `IterateContext`

### Fixed

- Contract tests now skip cleanly via `t.Skip()` when
  `AUDD_OPENAPI_FIXTURES` is not set, so `go test ./...` in CI no longer
  fails on missing sibling-repo fixtures. The dedicated `contract.yml` job
  still runs them by setting the env var.
- Integration tests (build tag `integration`) now compile against the
  current `RecognizeContext` signature.

## [1.4.0] - 2026-05-05

README polish: tightened the Capabilities table (longpoll variants
collapsed into a single Streams row + a dedicated Streams subsection),
dropped the implementation-detail thread-safety paragraph from the intro,
removed internal design-spec breadcrumbs from user-visible doc comments,
and updated the install snippet to `@v1.4.0`. No API changes.

## [1.3.0] - 2026-05-05

Coordinated v1.3.0 stable release across the audd-sdks family. No breaking
changes; the version bump signals API stability across all nine SDKs.

The full v0.3.0 polish — env-var auto-pickup, streaming/preview helpers
with metadata fallback, `onEvent` inspection hook, thread-safe token
rotation, plus per-language work (JPMS module-info, Kotlin `Flow<LongpollEvent>`,
Swift `Sendable` + DocC, .NET AOT/source-gen + `IServiceCollection`, Rust
TLS feature flags + `Serialize`, PHP PSR-3 logger, Python `__repr__` +
`pretty_print()`, Go `slog` example) is now the v1.3.0 baseline.

## [1.0.0] — 2026-05-04

The official refreshed Go SDK. Adds namespaced sub-clients
(`client.Streams()`, `client.CustomCatalog()`, `client.Advanced()`),
tokenless `LongpollConsumer`, cost-aware retry, per-attempt source
re-opener, code-51 deprecation pass-through, HTTP-vs-JSON error
distinction, and hardened `LongpollConsumer` failure modes.

The existing flat API (`client.RecognizeByUrl`, `client.AddStream`,
`client.FindLyrics`) remains as deprecated wrappers that delegate to the
new namespaced shape. They will be removed at v2.0.0.
