package audd

type RecognitionResult struct {
	Artist      string           `json:"artist"`
	Title       string           `json:"title"`
	Album       string           `json:"album"`
	ReleaseDate string           `json:"release_date"`
	Label       string           `json:"label"`
	Timecode    string           `json:"timecode"`
	ISRC        string           `json:"isrc"`
	UPC         string           `json:"upc"`
	Lyrics      LyricsResult     `json:"lyrics"`
	AppleMusic  AppleMusicResult `json:"apple_music"`
	Deezer      DeezerResult     `json:"deezer"`
	Spotify     SpotifyResult    `json:"spotify"`
	TimeCode    string `json:"timecode"`
	SongLink    string `json:"song_link"`
}

type HummingRecognitionResult struct {
	Count int             `json:"count"`
	List  []HummingResult `json:"list"`
}

type HummingResult struct {
	Score  int    `json:"score"`
	Artist string `json:"artist"`
	Title  string `json:"title"`
}

type LyricsResult struct {
	SongId        int    `json:"song_id,string"`
	ArtistId      int    `json:"artist_id,string"`
	Title         string `json:"title"`
	TitleWithFeat string `json:"title_with_featured"`
	FullTitle     string `json:"full_title"`
	Artist        string `json:"artist"`
	Lyrics        string `json:"lyrics"`
	Media         string `json:"media"`
}

type AppleMusicResult struct {
	Previews []struct {
		URL string `json:"url"`
	} `json:"previews"`
	Artwork struct {
		Width      int    `json:"width"`
		Height     int    `json:"height"`
		URL        string `json:"url"`
		BgColor    string `json:"bgColor"`
		TextColor1 string `json:"textColor1"`
		TextColor2 string `json:"textColor2"`
		TextColor3 string `json:"textColor3"`
		TextColor4 string `json:"textColor4"`
	} `json:"artwork"`
	ArtistName       string   `json:"artistName"`
	URL              string   `json:"url"`
	DiscNumber       int      `json:"discNumber"`
	GenreNames       []string `json:"genreNames"`
	DurationInMillis int      `json:"durationInMillis"`
	ReleaseDate      string   `json:"releaseDate"`
	Name             string   `json:"name"`
	ISRC             string   `json:"isrc"`
	AlbumName        string   `json:"albumName"`
	PlayParams       struct {
		ID   string `json:"id"`
		Kind string `json:"kind"`
	} `json:"playParams"`
	TrackNumber  int    `json:"trackNumber"`
	ComposerName string `json:"composerName"`
}

type DeezerResult struct {
	ID             int    `json:"id"`
	Readable       bool   `json:"readable"`
	Title          string `json:"title"`
	TitleShort     string `json:"title_short"`
	TitleVersion   string `json:"title_version"`
	Link           string `json:"link"`
	Duration       int    `json:"duration"`
	Rank           int    `json:"rank"`
	ExplicitLyrics bool   `json:"explicit_lyrics"`
	Preview        string `json:"preview"`
	Artist         struct {
		ID            int    `json:"id"`
		Name          string `json:"name"`
		Link          string `json:"link"`
		Picture       string `json:"picture"`
		PictureSmall  string `json:"picture_small"`
		PictureMedium string `json:"picture_medium"`
		PictureBig    string `json:"picture_big"`
		PictureXl     string `json:"picture_xl"`
		Tracklist     string `json:"tracklist"`
		Type          string `json:"type"`
	} `json:"artist"`
	Album struct {
		ID          int    `json:"id"`
		Title       string `json:"title"`
		Cover       string `json:"cover"`
		CoverSmall  string `json:"cover_small"`
		CoverMedium string `json:"cover_medium"`
		CoverBig    string `json:"cover_big"`
		CoverXl     string `json:"cover_xl"`
		Tracklist   string `json:"tracklist"`
		Type        string `json:"type"`
	} `json:"album"`
	Type string `json:"type"`
}

type SpotifyResult struct {
	Album struct {
		AlbumType string `json:"album_type"`
		Artists   []struct {
			ExternalUrls struct {
				Spotify string `json:"spotify"`
			} `json:"external_urls"`
			Href string `json:"href"`
			ID   string `json:"id"`
			Name string `json:"name"`
			Type string `json:"type"`
			URI  string `json:"uri"`
		} `json:"artists"`
		AvailableMarkets []string `json:"available_markets"`
		ExternalUrls     struct {
			Spotify string `json:"spotify"`
		} `json:"external_urls"`
		Href   string `json:"href"`
		ID     string `json:"id"`
		Images []struct {
			Height int    `json:"height"`
			URL    string `json:"url"`
			Width  int    `json:"width"`
		} `json:"images"`
		Name                 string `json:"name"`
		ReleaseDate          string `json:"release_date"`
		ReleaseDatePrecision string `json:"release_date_precision"`
		TotalTracks          int    `json:"total_tracks"`
		Type                 string `json:"type"`
		URI                  string `json:"uri"`
	} `json:"album"`
	Artists []struct {
		ExternalUrls struct {
			Spotify string `json:"spotify"`
		} `json:"external_urls"`
		Href string `json:"href"`
		ID   string `json:"id"`
		Name string `json:"name"`
		Type string `json:"type"`
		URI  string `json:"uri"`
	} `json:"artists"`
	AvailableMarkets []string `json:"available_markets"`
	DiscNumber       int      `json:"disc_number"`
	DurationMs       int      `json:"duration_ms"`
	Explicit         bool     `json:"explicit"`
	ExternalIds      struct {
		Isrc string `json:"isrc"`
	} `json:"external_ids"`
	ExternalUrls struct {
		Spotify string `json:"spotify"`
	} `json:"external_urls"`
	Href        string `json:"href"`
	ID          string `json:"id"`
	IsLocal     bool   `json:"is_local"`
	Name        string `json:"name"`
	Popularity  int    `json:"popularity"`
	TrackNumber int    `json:"track_number"`
	Type        string `json:"type"`
	URI         string `json:"uri"`
}
