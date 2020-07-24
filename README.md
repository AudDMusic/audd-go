[![PkgGoDev](https://pkg.go.dev/badge/github.com/AudDMusic/audd-go@v0.1.0)](https://pkg.go.dev/github.com/AudDMusic/audd-go@v0.1.0)
[![MIT licensed](https://img.shields.io/badge/license-MIT-blue.svg?style=flat-square)](./LICENSE)
[![Twitter Follow](https://img.shields.io/twitter/follow/helloAudD.svg?style=social&label=Follow)](https://twitter.com/helloAudD)

## Table of Contents

* [Quick Start](#quick-start)
* [Use Cases](#use-cases)
* [License](#license)

<a name="quick-start"></a>
# Quick Start

## Installation
`go get github.com/AudDMusic/audd-go`

## API Token
To send >10 requests, obtain an api_token from [our Telegram bot](https://t.me/auddbot?start=api) and change "test" to the obtained token in `NewClient("test")`.

## Send the files
For `recognize` and `recognizeWithOffset` API methods, as well as for using the enterprise endpoint (which accepts long audio files, usually useful for hours-long mixes), you have to send a file for recognition. There are two ways to send files to our API, you can either
- 🔗 provide an HTTP URL of the file (our server will download and recognize the music from the file), or
- 📤 post the file using multipart/form-data in the usual way that files are uploaded via the browser.

There are functions for the both ways. `RecognizeByFile` and `RecognizeByUrl` for the `recognize` method, `RecognizeLongAudioByFile` and `RecognizeLongAudioByUrl` for the enterprise endpoint, `RecognizeHumming` and `RecognizeHummingByUrl` for the `recognizeWithOffset` method. The In the pairs, the first functions accept `io.Reader`, the second accept `string` with an URL.

There are also `Recognize` and `RecognizeLongAudio` functions that accept an interface that could be `io.Reader` or `[]byte` for files, `string` or `url.URL` for URLs.

### Recognize music from a file with URL
It's really easy.
```
package main

import (
	"fmt"
	"github.com/AudDMusic/audd-go"
)

func main()  {
    // initialize the client with "test" as a token
	client := audd.NewClient("test")
    // recognize music in audd.tech/example1.mp3 and return Apple Music, Deezer and Spotify metadata
	song, err := client.RecognizeByUrl("https://audd.tech/example1.mp3", "apple_music,deezer,spotify", nil)
	if err != nil {
		panic(err)
	}
	fmt.Printf("%s - %s.\nTimecode: %s, album: %s. ℗ %s, %s\n\n"+
		"Listen: %s\nOr directly on:\n- Apple Music: %s, \n- Spotify: %s,\n- Deezer: %s.",
		song.Artist, song.Title, song.Timecode, song.Album, song.Label, song.ReleaseDate,
		song.SongLink, song.AppleMusic.URL, song.Spotify.ExternalUrls.Spotify, song.Deezer.Link)
	if len(song.AppleMusic.Previews) > 0 {
		fmt.Printf("\n\nPreview: %s", song.AppleMusic.Previews[0].URL)
	}
}
```
<br></br>

If you run this code, you should see a result like

```
Imagine Dragons - Warriors.
Timecode: 00:40, album: Warriors. ℗ Universal Music, 2014-09-18

Listen: https://lis.tn/Warriors
Or directly on:
- Apple Music: https://music.apple.com/us/album/warriors/1440831203?app=music&at=1000l33QU&i=1440831624&mt=1,
- Spotify: https://open.spotify.com/track/1lgN0A2Vki2FTON5PYq42m,
- Deezer: https://www.deezer.com/track/85963521.

Preview: https://audio-ssl.itunes.apple.com/itunes-assets/AudioPreview118/v4/65/07/f5/6507f5c5-dba8-f2d5-d56b-39dbb62a5f60/mzaf_1124211745011045566.plus.aac.p.m4a
```

### Recognize music from a local file
You can also send your local files to the API.
```
file, _ := os.Open("/path/to/example.mp3/path/to/example.mp3")
result, err := client.RecognizeByFile(file, "apple_music,deezer,spotify", nil)
file.Close()
```

`file` in `client.RecognizeByFile` could be any variable that implements the io.Reader interface.

### Search lyrics
It's easy with the `FindLyrics` function.
```
result, err := client.FindLyrics("You were the shadow to my light", nil)
if len(result) == 0 {
	fmt.Println("AudD can't find any lyrics by this query")
	return
}
firstSong := result[0]
fmt.Printf("First match: %s - %s\n\n%s", firstSong.Artist, firstSong.Title, firstSong.Lyrics)
```

### Recognize music from a hour-long mix
There are the enterprise endpoint that accepts long audio files (see [docs-e.audd.io](https://docs-e.audd.io)).
```
client.SetEndpoint(audd.EnterpriseAPIEndpoint)
data := map[string]string{"skip": "3", "every": "1"}
file := "https://audd.tech/djatwork_example.mp3"
result, _ := client.RecognizeLongAudio(file, data)
b, _ := json.Marshall(result)
fmt.Println(string(b))
```

Also, there's `UseExperimentalUploading()` that allows to start sending a file before it's loaded in the memory. Useful for large local files sent to the enterprise endpoint (see [the example](examples/scanFiles)). 

## Use other API methods

See the [package docs](https://pkg.go.dev/github.com/AudDMusic/audd-go@v0.1.0). Most of the public API methods are available via functions with the same names. 

For other methods, there are `Send`, `SendFile`, `SendUrl` functions that may help you with sending the API requests.

<a name="use-cases"></a>
# Use Cases
How you can use the AudD [Music Recognition API](https://audd.io/):
### Content analysis
Use our Music Recognition API to detect and identify songs in any audio content.

Create upload filters for UGC when the law requires. Find out what songs are used in any content on the Internet – or in the videos uploaded to your server. Recognize music from hours-long DJ mixes. Analyze the trends. Display the recognition results to your users. Use the metadata the API returns for your recommendation systems.
### In-app music recognition and lyrics searching
Make your own music recognition application using AudD Music Recognition API. Detect and recognize music. Identify the songs your users are listening to and display the lyrics of the recognized songs. Or just let your users search lyrics by text.
### Audio streams
Use AudD real-time music recognition service for audio streams to identify the songs that are being played on radio stations (or any other streams).

Monitor radio airplay, create radio songs charts. Get real-time insights. If you have your own content DB, we can recognize songs that you upload (it's cheaper).

See https://streams.audd.io/ for more info.
<a name="license"></a>
# License
[The MIT License (MIT)](LICENSE)
