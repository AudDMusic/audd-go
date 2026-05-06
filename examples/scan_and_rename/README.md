# scan_and_rename

Walks a folder of audio files, recognizes each via the AudD API, writes the
recognition into the file's tags, and renames it to `Artist - Title.ext`.

```sh
export AUDD_API_TOKEN=aud_xxx
go run . /path/to/folder                   # dry-run (default)
go run . /path/to/folder --apply           # actually tag + rename
go run . /path/to/folder --apply --concurrency 8
```

What it does:

- Walks the folder recursively. Picks up `.mp3 .flac .ogg .opus .m4a .mp4 .wav .aac`.
- Calls `client.Recognize(path, nil)` — the no-context default — once per file,
  with `errgroup.SetLimit(N)` (default 4) to bound concurrency.
- On a match, sanitizes `Artist - Title` (replaces `/ \ : * ? " < > |` with
  `_`, trims to 200 chars) and renames the file in place. Skips on collision.
- Writes ID3v2 tags (artist, title, album, year) for `.mp3` files. For other
  formats this example logs `tag-write skipped, mp3 only` and renames anyway —
  ID3v2 has a clean Go writer; the multi-format Vorbis/MP4 tag-write story in
  Go is messier, so we left it out of the example.
- Prints `[3/27] foo.mp3  tagged + renamed → "Artist - Title"` per file and a
  summary at the end.

The module has its own `go.mod` so the SDK module doesn't take on
`id3v2/v2` and `x/sync` as deps.
