[![AudD](https://audd.io/images/1.png)](https://audd.io/)

# Table of Contents

* [Installation](#installation)
* [Quick Start](#quick-start)
* [Use Cases](#use-cases)
* [License](#license)

<a name="installation"></a>
## Installation
`go get github.com/AudDMusic/audd-go`

## API Token
To use the AudD Music Recognition API, obtain an api_token from [our Telegram bot](https://t.me/auddbot?start=api).

<a name="quick-start"></a>
# Quick Start
For `recognize` and `recognizeWithOffset` API methods you have to send a file for recognition. There are two ways to send files to our API, you can either
- 🔗 provide an HTTP URL of the file (our server will download and recognize the music from the file), or
- 📤 post the file using multipart/form-data in the usual way that files are uploaded via the browser.

## Recognize music from a file with URL
It's really easy.
```
package main

import (
	"fmt"
	"github.com/AudDMusic/audd-go"
)

func main()  {
	parameters := map[string]string{"return": "timecode,apple_music,deezer,spotify"}
	result, err := audd.Recognize("https://audd.tech/example1.mp3", "test", parameters)
	if err != nil {
		panic(err)
	}
	if result.Status == "error" {
		fmt.Printf("Error: %s (#%d)\n", result.Error.ErrorMessage, result.Error.ErrorCode)
		return
	}
	if result.IsNull() {
		fmt.Println("AudD can't recognize any music in this file")
		return
	}
	song := result.Result
	fmt.Printf("%s - %s.\nTimecode: %s, album: %s. ℗ %s, %s\n\n" +
		"Listen on:\nApple Music: %s, \nSpotify: %s,\nDeezer: %s.",
		song.Artist, song.Title, song.Timecode, song.Album, song.Label, song.ReleaseDate,
		song.AppleMusic.URL, song.Spotify.ExternalUrls.Spotify, song.Deezer.Link)
	if len(song.AppleMusic.Previews) > 0 {
		fmt.Printf("\n\nPreview: %s", song.AppleMusic.Previews[0].URL)
	}
}
```
</br>

If you run this code, you should see a result like

```
Imagine Dragons - Warriors.
Timecode: 00:40, album: Warriors. ℗ Universal Music, 2014-09-18

Listen on:
Apple Music: https://music.apple.com/us/album/warriors/1440831203?app=music&at=1000l33QU&i=1440831624&mt=1,
Spotify: https://open.spotify.com/track/1lgN0A2Vki2FTON5PYq42m,
Deezer: https://www.deezer.com/track/85963521.

Preview: https://audio-ssl.itunes.apple.com/itunes-assets/AudioPreview118/v4/65/07/f5/6507f5c5-dba8-f2d5-d56b-39dbb62a5f60/mzaf_1124211745011045566.plus.aac.p.m4a
```

## Recognize music from a local file
You can also send your local files to the API.
```
	parameters := map[string]string{"return": "timecode,apple_music,deezer,spotify"}
	file, _ := os.Open("/path/to/example.mp3/path/to/example.mp3")
	result, err := audd.RecognizeByFile(file, "test", parameters)
	file.Close()
```
`file` in `audd.RecognizeByFile(file, "test", parameters)` could be any variable that implemeents io.Reader interface. If you have e.g. `var data []byte`, you can use `bytes.NewReader(data)` as `file`.

## Searching the lyrics
It's also really easy with `audd.FindLyrics` function.
```
package main

import (
	"fmt"
	"github.com/AudDMusic/audd-go"
)

func main()  {
	result, err := audd.FindLyrics("You were the shadow to my light", "test")
	if err != nil {
		panic(err)
	}
	if result.Status == "error" {
		fmt.Printf("Error: %s (#%d)\n", result.Error.ErrorMessage, result.Error.ErrorCode)
		return
	}
	if len(result.Result) == 0 {
		fmt.Println("AudD can't find any lyrics by this query")
		return
	}
	firstSong := result.Result[0]
	fmt.Printf("First match: %s - %s\n\n%s", firstSong.Artist, firstSong.Title, firstSong.Lyrics)
}
```
<a name="use-cases"></a>
# Use Cases
How you can use the AudD [Music Recognition API](https://audd.io/):
## UGC
Detect music and identify songs from user-generated content in your apps. Create upload filters when the law requires. Use the metadata AudD Music Recognition API returns in your recommendation systems.
## In-app music recognition and lyrics searching
Detect and recognize music in your apps using AudD Music Recognition API. Identify songs for your users and display the lyrics of the recognized songs. Or just search lyrics by text.
## Offline background music
Calculate stats of offline music plays. Send files or audio streams from multiple devices in the real world into our Music Recognition API.
## Audio streams
Recognize the music that plays on radio stations with AudD real-time music recognition service for audio streams. Use the AudD Music DB, or upload your own songs DB. (Currently not available with this library, contact api@audd.io if you're interested in the live muisic recognition.)
<a name="license"></a>
# License
[The MIT License (MIT)](LICENSE)
