package audd

import (
	"io"
)

const (
	recognizeMethod        string = "recognize"
	recognizeHummingMethod string = "recognizeWithOffset"
	findLyricsMethod       string = "findLyrics"
)

type RecognitionResponse struct {
	Status string `json:"status"`
	Error  struct {
		ErrorCode    int    `json:"error_code"`
		ErrorMessage string `json:"error_message"`
	} `json:"error"`
	Result  RecognitionResult `json:"result"`
	Warning struct {
		ErrorCode    int    `json:"error_code"`
		ErrorMessage string `json:"error_message"`
	} `json:"warning"`
}

type RecognitionResponseAllMatches struct {
	Status string `json:"status"`
	Error  struct {
		ErrorCode    int    `json:"error_code"`
		ErrorMessage string `json:"error_message"`
	} `json:"error"`
	Result []RecognitionResult `json:"result"`
}

type HummingRecognitionResponse struct {
	Status string `json:"status"`
	Error  struct {
		ErrorCode    int    `json:"error_code"`
		ErrorMessage string `json:"error_message"`
	} `json:"error"`
	Result HummingRecognitionResult `json:"result"`
}

type FindLyricsResponse struct {
	Status string `json:"status"`
	Error  struct {
		ErrorCode    int    `json:"error_code"`
		ErrorMessage string `json:"error_message"`
	} `json:"error"`
	Result []LyricsResult `json:"result"`
}

func RecognizeByFile(file io.Reader, ApiToken string, parameters map[string]string) (RecognitionResponse, error) {
	return handleRecognitionResponse(fileRequest(file, ApiToken, parameters, recognizeMethod))
}

func Recognize(url, ApiToken string, parameters map[string]string) (RecognitionResponse, error) {
	return handleRecognitionResponse(urlRequest(url, ApiToken, parameters, recognizeMethod))
}

// RecognizeAllMatches returns all the matched songs
func RecognizeAllMatches(url, ApiToken string) (RecognitionResponseAllMatches, error) {
	parameters := map[string]string{"all": "true"}
	return handleRecognitionAllMatchesResponse(urlRequest(url, ApiToken, parameters, recognizeMethod))
}

// RecognizeAllMatchesByFile returns all the matched songs
func RecognizeAllMatchesByFile(file io.Reader, ApiToken string) (RecognitionResponseAllMatches, error) {
	parameters := map[string]string{"all": "true"}
	return handleRecognitionAllMatchesResponse(fileRequest(file, ApiToken, parameters, recognizeMethod))
}

func RecognizeHumming(url string, ApiToken string) (HummingRecognitionResponse, error) {
	return handleHummingRecognitionResponse(urlRequest(url, ApiToken, nil, recognizeHummingMethod))
}

func RecognizeHummingByFile(file io.Reader, ApiToken string) (HummingRecognitionResponse, error) {
	return handleHummingRecognitionResponse(fileRequest(file, ApiToken, nil, recognizeHummingMethod))
}

func FindLyrics(query, ApiToken string) (FindLyricsResponse, error) {
	data := map[string]string{
		"method": findLyricsMethod,
		"q":      query,
	}
	return handleFindLyricsResponse(Request(data, ApiToken))
}
