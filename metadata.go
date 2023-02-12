package audd

type RecognitionResult struct {
	Artist      string                  `json:"artist,omitempty"`
	Title       string                  `json:"title,omitempty"`
	Album       string                  `json:"album,omitempty"`
	ReleaseDate string                  `json:"release_date,omitempty"`
	Label       string                  `json:"label,omitempty"`
	Timecode    string                  `json:"timecode,omitempty"`
	SongLink    string                  `json:"song_link,omitempty"`
	Lyrics      *LyricsResult           `json:"lyrics,omitempty"`
	AppleMusic  *AppleMusicResult       `json:"apple_music,omitempty"`
	Deezer      *DeezerResult           `json:"deezer,omitempty"`
	MusicBrainz []MusicbrainzRecordings `json:"musicbrainz,omitempty"`
	Napster     *NapsterResult          `json:"napster,omitempty"`
	Spotify     *SpotifyResult          `json:"spotify,omitempty"`
	ISRC        string                  `json:"isrc,omitempty"`
	UPC         string                  `json:"upc,omitempty"`
	Score       int                     `json:"score,omitempty"`
	SongLength  string                  `json:"song_length,omitempty"`
	AudioID     int                     `json:"audio_id,omitempty"`
	StartOffset int                     `json:"start_offset,omitempty"`
	EndOffset   int                     `json:"end_offset,omitempty"`
}

type RecognitionEnterpriseResult struct {
	Songs  []RecognitionResult `json:"songs"`
	Offset string              `json:"offset"`
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

type Stream struct {
	RadioID       int    `json:"radio_id"`
	URL           string `json:"url"`
	StreamRunning bool   `json:"stream_running"`
}

type StreamCallback struct {
	Status       string                   `json:"status"`
	Notification *StreamNotification      `json:"notification"`
	Result       *StreamRecognitionResult `json:"result"`
	Time         int64                    `json:"time"`
}

type StreamRecognitionResult struct {
	RadioID    int                 `json:"radio_id"`
	Timestamp  string              `json:"timestamp"`
	PlayLength int                 `json:"play_length,omitempty"`
	Results    []RecognitionResult `json:"results"`
}

type StreamNotification struct {
	RadioID       int    `json:"radio_id"`
	StreamRunning bool   `json:"stream_running"`
	Code          int    `json:"notification_code"`
	Message       string `json:"notification_message"`
}

type NapsterResult struct {
	Type               string        `json:"type"`
	ID                 string        `json:"id"`
	Index              int           `json:"index"`
	Disc               int           `json:"disc"`
	Href               string        `json:"href"`
	PlaybackSeconds    int           `json:"playbackSeconds"`
	IsExplicit         bool          `json:"isExplicit"`
	IsStreamable       bool          `json:"isStreamable"`
	IsAvailableInHiRes bool          `json:"isAvailableInHiRes"`
	Name               string        `json:"name"`
	Isrc               string        `json:"isrc"`
	Shortcut           string        `json:"shortcut"`
	Blurbs             []interface{} `json:"blurbs"`
	ArtistID           string        `json:"artistId"`
	ArtistName         string        `json:"artistName"`
	AlbumName          string        `json:"albumName"`
	Formats            []struct {
		Type       string `json:"type"`
		Bitrate    int    `json:"bitrate"`
		Name       string `json:"name"`
		SampleBits int    `json:"sampleBits"`
		SampleRate int    `json:"sampleRate"`
	} `json:"formats"`
	LosslessFormats []interface{} `json:"losslessFormats"`
	AlbumID         string        `json:"albumId"`
	Contributors    struct {
		PrimaryArtist string `json:"primaryArtist"`
	} `json:"contributors"`
	Links struct {
		Artists struct {
			Ids  []string `json:"ids"`
			Href string   `json:"href"`
		} `json:"artists"`
		Albums struct {
			Ids  []string `json:"ids"`
			Href string   `json:"href"`
		} `json:"albums"`
		Genres struct {
			Ids  []string `json:"ids"`
			Href string   `json:"href"`
		} `json:"genres"`
		Tags struct {
			Ids  []string `json:"ids"`
			Href string   `json:"href"`
		} `json:"tags"`
	} `json:"links"`
	PreviewURL string `json:"previewURL"`
}

type MusicbrainzRecordings struct {
	ID             string      `json:"id"`
	Score          int         `json:"score"`
	Title          string      `json:"title"`
	Length         int         `json:"length"`
	Disambiguation string      `json:"disambiguation"`
	Video          interface{} `json:"video"`
	ArtistCredit   []struct {
		Name   string `json:"name"`
		Artist struct {
			ID       string `json:"id"`
			Name     string `json:"name"`
			SortName string `json:"sort-name"`
		} `json:"artist"`
	} `json:"artist-credit"`
	Releases []struct {
		ID             string `json:"id"`
		Count          int    `json:"count"`
		Title          string `json:"title"`
		Status         string `json:"status"`
		Disambiguation string `json:"disambiguation,omitempty"`
		Date           string `json:"date"`
		Country        string `json:"country"`
		ReleaseEvents  []struct {
			Date string `json:"date"`
			Area struct {
				ID            string   `json:"id"`
				Name          string   `json:"name"`
				SortName      string   `json:"sort-name"`
				Iso31661Codes []string `json:"iso-3166-1-codes"`
			} `json:"area"`
		} `json:"release-events"`
		TrackCount int `json:"track-count"`
		Media      []struct {
			Position int    `json:"position"`
			Format   string `json:"format"`
			Track    []struct {
				ID     string `json:"id"`
				Number string `json:"number"`
				Title  string `json:"title"`
				Length int    `json:"length"`
			} `json:"track"`
			TrackCount  int `json:"track-count"`
			TrackOffset int `json:"track-offset"`
		} `json:"media"`
		ArtistCredit []struct {
			Name   string `json:"name"`
			Artist struct {
				ID             string `json:"id"`
				Name           string `json:"name"`
				SortName       string `json:"sort-name"`
				Disambiguation string `json:"disambiguation"`
			} `json:"artist"`
		} `json:"artist-credit,omitempty"`
		ReleaseGroup struct {
			ID             string   `json:"id"`
			TypeID         string   `json:"type-id"`
			Title          string   `json:"title"`
			PrimaryType    string   `json:"primary-type"`
			SecondaryTypes []string `json:"secondary-types"`
		} `json:"release-group,omitempty"`
	} `json:"releases"`
	Isrcs []string `json:"isrcs"`
	Tags  []struct {
		Count int    `json:"count"`
		Name  string `json:"name"`
	} `json:"tags"`
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
