This is an example of an interaction with the AudD API.

## What it does

It's a tool for streams that once there a new song on a stream, adds it to a Spotify playlist.

It works with YouTube streams and Spotify broadcasts, as well as with online radio stations. HLS, DASH, RTMP, M3U, other formats are also supported.

## How does it work

It utilizes the AudD® [Music Recognition API](https://audd.io/): adds the stream to the music recognition service and listens for callbacks with results.

See the [Music Recognition API docs for streams](https://docs.audd.io/streams/) for more info.

It also uses the [Spotify API](https://developer.spotify.com/documentation/web-api/reference/playlists/add-tracks-to-playlist/) for adding new tracks to the playlist. 

## How to use it

You'll need a server with a static IP address. 

1. Obtain a token from the [Music Recognition API Dashboard](https://dashboard.audd.io/).
2. Subscribe to or the music recognition for streams or contact us so we add a stream for free.
3. Create an App on the [Spotify for Developers Dashboard](https://developer.spotify.com/dashboard/applications) and get the credentials: Client ID and Client Secret.
4. Run the binary and follow the instructions

```
$ ./callbacksToSpotify
Create an app on https://developer.spotify.com/dashboard/applications, and specify the client ID and client secret in the flags or 
SPOTIFY_ID and SPOTIFY_SECRET env vars
Run with -h to see all the flags
Please set the server's public address and the AudD API token
Run with -h to see all the flags

$ ./callbacksToSpotify -h
Usage of ./callbacksToSpotify:
  -address string
        specify the server's public address (e.g. an IP)
  -api_token string
        the AudD API token
  -client_id string
        Spotify Client ID
  -client_secret string
        Spotify Client Secret
  -longpoll
        Use this if you don't want the script to change the callback URL
  -min_score int
        The minimum score (if a result has score below specified, it won't be processed) (default 85)
  -new_first
        Append new songs to the beginning of the playlist instead of the end
  -playlist_id string
        The ID of the Spotify playlist
  -port string
        the port to listen on (default "3022")
  -radio_id int
        If you haven't added the stream to AudD already using the addStream API method, you can specify the stream ID here (default 1)
  -stream_url string
        If you haven't added the stream to AudD already using the addStream API method, you can specify the stream URL here

./callbacksToSpotify -api_token yourApiTokenfffffffff -playlist_id theSpotifyPlaylistId \
-address serverIP -client_id spotifyClientId -client_secret spotifyClientSecret
```

## How to build it

The built binary for internal use is located at https://audd.tech/bin/callbacksToSpotify. 
Please do not use it (and never run stuff from the internet from root!), it's easy build the binary on your own instead:

* Install Golang if you haven't already
```
wget https://golang.org/dl/go1.15.6.linux-amd64.tar.gz
tar -C /usr/local -xzf go1.15.6.linux-amd64.tar.gz
export PATH=$PATH:/usr/local/go/bin
# Please also add the previous line to $HOME/.profile or /etc/profile
# Verify the installation:
go version
```

* Make a new directory and download main.go
```
mkdir callbacksToSpotify && cd callbacksToSpotify
wget https://raw.githubusercontent.com/AudDMusic/audd-go/master/examples/StreamTracksToSpotifyPlaylist/main.go
```

* Install the dependencies:
```
go get -d ./...
```

* Build the binary:
```
go build
```

