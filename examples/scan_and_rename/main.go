// Walk a folder of audio files, recognize each via the AudD API, write the
// recognition into the file's tags (mp3 today), and rename to
// "Artist - Title.ext". Defaults to dry-run; pass --apply to actually write.
//
//	go run . /path/to/folder
//	go run . /path/to/folder --apply --concurrency 8
//
// Reads the API token from the AUDD_API_TOKEN environment variable.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

	audd "github.com/AudDMusic/audd-go"
	id3 "github.com/bogem/id3v2/v2"
	"golang.org/x/sync/errgroup"
)

var audioExts = map[string]bool{
	".mp3":  true,
	".flac": true,
	".ogg":  true,
	".opus": true,
	".m4a":  true,
	".mp4":  true,
	".wav":  true,
	".aac":  true,
}

const maxBaseLen = 200

type result struct {
	path     string
	rec      *audd.Recognition
	err      error
	skipNote string // non-fatal explanation (e.g. collision, no match)
}

func main() {
	apply := flag.Bool("apply", false, "Actually write tags and rename. Default is dry-run.")
	concurrency := flag.Int("concurrency", 4, "Parallel recognize calls.")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [--apply] [--concurrency N] <folder>\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(2)
	}
	root := flag.Arg(0)
	if *concurrency < 1 {
		*concurrency = 1
	}

	files, err := collectAudioFiles(root)
	if err != nil {
		log.Fatalf("walk %q: %v", root, err)
	}
	if len(files) == 0 {
		fmt.Println("no audio files found.")
		return
	}
	fmt.Printf("found %d audio file(s) under %s\n", len(files), root)
	if !*apply {
		fmt.Println("dry-run: no tags written, no files renamed. Pass --apply to commit changes.")
	}

	client := audd.NewClient(os.Getenv("AUDD_API_TOKEN"))
	defer func() { _ = client.Close() }()

	var (
		matched, unmatched, failed, skipped int64
		mu                                  sync.Mutex // serializes printing so progress lines don't interleave
		done                                int64
	)
	total := int64(len(files))

	g := new(errgroup.Group)
	g.SetLimit(*concurrency)

	for _, path := range files {
		path := path
		g.Go(func() error {
			rec, recErr := client.Recognize(path, nil)
			n := atomic.AddInt64(&done, 1)

			mu.Lock()
			defer mu.Unlock()

			prefix := fmt.Sprintf("[%d/%d] %s", n, total, filepath.Base(path))

			if recErr != nil {
				atomic.AddInt64(&failed, 1)
				fmt.Printf("%s  recognize error: %v\n", prefix, recErr)
				return nil
			}
			if rec == nil || (rec.Artist == "" && rec.Title == "") {
				atomic.AddInt64(&unmatched, 1)
				fmt.Printf("%s  no match\n", prefix)
				return nil
			}

			label := fmt.Sprintf("%q", rec.Artist+" - "+rec.Title)
			if !*apply {
				atomic.AddInt64(&matched, 1)
				fmt.Printf("%s  would tag + rename to %s\n", prefix, label)
				return nil
			}

			newPath, action, applyErr := applyRecognition(path, rec)
			if applyErr != nil {
				if errors.Is(applyErr, errCollision) {
					atomic.AddInt64(&skipped, 1)
					fmt.Printf("%s  skipped (target exists): %s\n", prefix, newPath)
					return nil
				}
				atomic.AddInt64(&failed, 1)
				fmt.Printf("%s  apply error: %v\n", prefix, applyErr)
				return nil
			}
			atomic.AddInt64(&matched, 1)
			fmt.Printf("%s  %s → %s\n", prefix, action, label)
			return nil
		})
	}
	_ = g.Wait()

	fmt.Println()
	fmt.Printf("summary: %d matched, %d unmatched, %d skipped, %d failed (of %d total)\n",
		matched, unmatched, skipped, failed, total)
	if !*apply && matched > 0 {
		fmt.Println("re-run with --apply to write tags and rename.")
	}
}

// collectAudioFiles walks root recursively and returns every regular file with
// a known audio extension.
func collectAudioFiles(root string) ([]string, error) {
	var out []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if audioExts[ext] {
			out = append(out, path)
		}
		return nil
	})
	return out, err
}

var errCollision = errors.New("rename target already exists")

// applyRecognition writes tags (mp3 only, for now) and renames the file to
// "Artist - Title.ext" in its current directory. Returns the final path and a
// short action string ("tagged + renamed", "renamed (tag-write skipped)").
func applyRecognition(path string, rec *audd.Recognition) (string, string, error) {
	ext := strings.ToLower(filepath.Ext(path))
	tagAction := "renamed (tag-write skipped, mp3 only)"
	if ext == ".mp3" {
		if err := writeMP3Tags(path, rec); err != nil {
			return "", "", fmt.Errorf("write tags: %w", err)
		}
		tagAction = "tagged + renamed"
	}

	dir := filepath.Dir(path)
	base := sanitizeBase(rec.Artist + " - " + rec.Title)
	newPath := filepath.Join(dir, base+ext)
	if newPath == path {
		return newPath, tagAction, nil
	}
	if _, err := os.Stat(newPath); err == nil {
		return newPath, "", errCollision
	}
	if err := os.Rename(path, newPath); err != nil {
		return "", "", fmt.Errorf("rename: %w", err)
	}
	return newPath, tagAction, nil
}

// writeMP3Tags writes ID3v2.3 frames (artist, title, album, year) onto an mp3
// in place.
func writeMP3Tags(path string, rec *audd.Recognition) error {
	tag, err := id3.Open(path, id3.Options{Parse: true})
	if err != nil {
		return err
	}
	defer func() { _ = tag.Close() }()

	if rec.Artist != "" {
		tag.SetArtist(rec.Artist)
	}
	if rec.Title != "" {
		tag.SetTitle(rec.Title)
	}
	if rec.Album != "" {
		tag.SetAlbum(rec.Album)
	}
	if rec.ReleaseDate != "" {
		// ReleaseDate is "YYYY-MM-DD"; ID3 TYER wants the 4-digit year.
		if len(rec.ReleaseDate) >= 4 {
			tag.SetYear(rec.ReleaseDate[:4])
		}
	}
	return tag.Save()
}

// sanitizeBase replaces filesystem-unsafe characters and trims to maxBaseLen.
// The unsafe set is the union of restrictions across Linux/macOS/Windows so
// renamed files survive on any of them.
func sanitizeBase(s string) string {
	const unsafe = `/\:*?"<>|`
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if r < 0x20 || strings.ContainsRune(unsafe, r) {
			b.WriteByte('_')
			continue
		}
		b.WriteRune(r)
	}
	out := strings.TrimSpace(b.String())
	if len(out) > maxBaseLen {
		out = out[:maxBaseLen]
	}
	if out == "" {
		out = "untitled"
	}
	return out
}
