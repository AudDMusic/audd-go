package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/AudDMusic/audd-go"
	//"github.com/getsentry/raven-go"
	"github.com/zmb3/spotify"
	"golang.org/x/oauth2"
	spotifyAuth "golang.org/x/oauth2/spotify"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"runtime"
	"sync"
	"time"
)


var SpotifyClientID string
var SpotifyClientSecret string
var MinScore int
var PlaylistId string
var PlaylistNewFirst bool
var secretCallbackToken string

var spotifyClient spotifyClients
var clientCh = make(chan spotifyClients, 0)
var state string
var stateMu = &sync.Mutex{}

func main(){
	clientId := flag.String("client_id", "", "Spotify Client ID")
	clientSecret := flag.String("client_secret", "", "Spotify Client Secret")
	playlistId := flag.String("playlist_id", "", "The ID of the Spotify playlist")
	newFirst := flag.Bool("new_first", false,
		"Append new songs to the beginning of the playlist instead of the end")
	address := flag.String("address", "", "specify the server's public address (e.g. an IP)")
	port := flag.String("port", "3022", "the port to listen on")
	apiToken := flag.String("api_token", "", "the AudD API token")
	minScore := flag.Int("min_score", 85,
		"The minimum score (if a result has score below specified, it won't be processed)")
	StreamUrl := flag.String("stream_url", "",
		"If you haven't added the stream to AudD already using the addStream API method, " +
			"you can specify the stream URL here")
	RadioID := flag.Int("radio_id", 1,
		"If you haven't added the stream to AudD already using the addStream API method, " +
			"you can specify the stream ID here")
	UseLongPoll := flag.Bool("longpoll", false,
		"Use this if you don't want the script to change the callback URL")
	flag.Parse()

	if *clientId == "" || *clientSecret == "" {
		fmt.Println("Create an app on https://developer.spotify.com/dashboard/applications, " +
			"and specify the client ID and client secret in the flags or SPOTIFY_ID and SPOTIFY_SECRET env vars")
		fmt.Println("Run with -h to see all the flags")
	}

	if !*UseLongPoll && (*address == "" || *apiToken == "") {
		fmt.Println("Please set the server's public address and the AudD API token")
		fmt.Println("Run with -h to see all the flags")
		return
	}

	secretCallbackToken = RandString(10)
	MinScore = *minScore
	PlaylistId = *playlistId
	PlaylistNewFirst = *newFirst
	SpotifyClientID = *clientId
	SpotifyClientSecret = *clientSecret

	addr := *address+":"+*port

	var oauthConfig = oauth2.Config{
		ClientID: SpotifyClientID,
		ClientSecret: SpotifyClientSecret,
		Scopes: []string{"playlist-modify-public", "playlist-modify-private"},
		Endpoint: spotifyAuth.Endpoint,
		RedirectURL: "http://" + addr + "/auth/",
	}


	fmt.Println("Starting server at", addr)

	go func() {
		fmt.Println("1. Go to https://developer.spotify.com/dashboard/applications/"+SpotifyClientID)
		fmt.Println(`2. Open "Edit Settings"`)
		fmt.Println("3. Add a new Redirect URI:", oauthConfig.RedirectURL)
		newState := RandString(10)
		stateMu.Lock()
		state = newState
		stateMu.Unlock()
		authUrl := oauthConfig.AuthCodeURL(newState)
		fmt.Println("4. Authorize on", authUrl)
		spotifyClient = <- clientCh

		CallbackAddr := "http://" + addr + "/?return=spotify&secret="+secretCallbackToken
		auddClient := audd.NewClient(*apiToken)
		if !*UseLongPoll {
			err := auddClient.SetCallbackUrl(CallbackAddr, nil)
			capture(err)
		} else {
			lp := auddClient.NewLongPoll(*RadioID)
			go func() {
				for {
					select {
					case e := <-lp.ResultsChan:
						if len(e.Result.Results) > 0 {
							writeResult(e.Result.Results[0])
						}
					}
				}
				// lp.Stop()
			}()
		}
		fmt.Println("Authorized!")
		if *StreamUrl != "" {
			err := auddClient.AddStream(*StreamUrl, *RadioID, "before", nil)
			if !capture(err) {
				fmt.Println("Added the stream to AudD")
			}
		}
		fmt.Println("Listening for callbacks...")
	}()
	http.HandleFunc("/", processCallback)
	http.HandleFunc("/auth/", spotifyAuthenticator(oauthConfig).authHandler)
	log.Fatal(http.ListenAndServe(addr, nil))
}

type spotifyAuthenticator oauth2.Config
type spotifyClients struct {
	s *spotify.Client
	h *http.Client
}

var seededRand = rand.New(
	rand.NewSource(time.Now().UnixNano()))
func RandString(length int) string {
	charset := "abcdefghijklmnopqrstuvwxyz" +
		"ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[seededRand.Intn(len(charset))]
	}
	return string(b)
}

func (v spotifyAuthenticator) authHandler(w http.ResponseWriter, r *http.Request) {
	oauthConfig := oauth2.Config(v)
	auth := spotify.NewAuthenticator(oauthConfig.RedirectURL, oauthConfig.Scopes...)
	auth.SetAuthInfo(oauthConfig.ClientID, oauthConfig.ClientSecret)
	stateMu.Lock()
	getState := state
	stateMu.Unlock()
	token, err := auth.Token(getState, r)
	if err != nil {
		http.Error(w, "Couldn't get token", http.StatusNotFound)
		return
	}
	sClient := auth.NewClient(token)
	clients := spotifyClients{
		s: &sClient,
		h: oauthConfig.Client(context.Background(), token),
	}
	clientCh <- clients
	_, err = fmt.Fprint(w, "Authorized! Go back to CLI")
	capture(err)
}

func processCallback(w http.ResponseWriter, r *http.Request) {
	b, err := ioutil.ReadAll(r.Body)
	defer captureFunc(r.Body.Close)
	if capture(err) {
		return
	}
	if r.URL.Query().Get("secret") != secretCallbackToken {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	var msg audd.StreamCallback
	err = json.Unmarshal(b, &msg)
	if capture(err) {
		return
	}
	if len(msg.Result.Results) == 0 {
		fmt.Println("got a notification", string(b))
		return
	}
	if msg.Result.Results[0].Score < MinScore {
		fmt.Printf("skipping a result because of the low score (%d): %s - %s, %s\n", msg.Result.Results[0].Score,
			msg.Result.Results[0].Artist, msg.Result.Results[0].Title, msg.Result.Results[0].SongLink)
		return
	}
	writeResult(msg.Result.Results[0])
}

func writeResult(song audd.RecognitionResult) {
	if song.Spotify == nil {
		fmt.Printf("Got a result without the Spotify data (%s - %s, %s)\n", song.Artist, song.Title, song.SongLink)
		return
	}
	if song.Spotify.ID == "" {
		fmt.Printf("Got a result without the Spotify data (%s - %s, %s)\n", song.Artist, song.Title, song.SongLink)
		return
	}

	if !PlaylistNewFirst {
		_, err := spotifyClient.s.AddTracksToPlaylist(spotify.ID(PlaylistId), spotify.ID(song.Spotify.ID))
		if capture(err) {
			return
		}
	} else {
		spotifyURL := fmt.Sprintf("https://api.spotify.com/v1/playlists/%s/tracks", url.PathEscape(PlaylistId))

		uris := make([]string, 1)
		uris[0] = fmt.Sprintf("spotify:track:%s", song.Spotify.ID)

		m := make(map[string]interface{})
		m["uris"] = uris
		m["position"] = "0"
		body, err := json.Marshal(m)
		if capture(err) {
			return
		}
		req, err := http.NewRequest("POST", spotifyURL, bytes.NewReader(body))
		if capture(err) {
			return
		}
		req.Header.Set("Content-Type", "application/json")
		_, err = spotifyClient.h.Do(req)
		if capture(err) {
			return
		}
	}

	fmt.Printf("Added a song to the playlist (%s - %s, %s)\n", song.Artist, song.Title, song.SongLink)
}

func capture(err error) bool {
	if err == nil {
		return false
	}
	_, file, no, ok := runtime.Caller(1)
	if ok {
		err = fmt.Errorf("%v from %s#%d", err, file, no)
	}
	//go raven.CaptureError(err, nil)
	go fmt.Println(err)
	return true
}
func captureFunc(f func() error) (r bool) {
	err := f()
	if r = err != nil; r {
		_, file, no, ok := runtime.Caller(1)
		if ok {
			err = fmt.Errorf("%v from %s#%d", err, file, no)
		}
		//go raven.CaptureError(err, nil)
		go fmt.Println(err)
	}
	return
}
func init() {
	/*err := raven.SetDSN("")
	if err != nil {
		panic(err)
	}*/
}

