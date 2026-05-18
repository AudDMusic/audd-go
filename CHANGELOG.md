# Changelog

All notable changes to this project are documented here.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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

## [Unreleased]

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

## [1.0.0] — 2026-05-04

The official refreshed Go SDK. Adds namespaced sub-clients
(`client.Streams()`, `client.CustomCatalog()`, `client.Advanced()`),
tokenless `LongpollConsumer`, cost-aware retry, per-attempt source
re-opener, code-51 deprecation pass-through, HTTP-vs-JSON error
distinction, and hardened `LongpollConsumer` failure modes.

The existing flat API (`client.RecognizeByUrl`, `client.AddStream`,
`client.FindLyrics`) remains as deprecated wrappers that delegate to the
new namespaced shape. They will be removed at v2.0.0.
