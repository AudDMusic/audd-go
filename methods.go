package audd

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/url"
	"strconv"
)

const (
	recognizeMethod        string = "recognize" // this is the default method, not necessary to specify this method
	recognizeHummingMethod string = "recognizeWithOffset"
	findLyricsMethod       string = "findLyrics"
	setCallbackUrlMethod   string = "setCallbackUrl"
	getCallbackUrlMethod   string = "getCallbackUrl"
	addStreamMethod        string = "addStream"
	setStreamUrlMethod     string = "setStreamUrl"
	getStreamsMethod       string = "getStreams"
	deleteStreamMethod     string = "deleteStream"
)

type Error struct {
	ErrorCode    int    `json:"error_code"`
	ErrorMessage string `json:"error_message"`
}
type Warning struct {
	ErrorCode    int    `json:"error_code"`
	ErrorMessage string `json:"error_message"`
}

func (e Error) Error() string {
	return fmt.Sprintf("the API returned an error: code %d, message %s", e.ErrorCode, e.ErrorMessage)
}
func (e Warning) Error() string {
	return fmt.Sprintf("the API returned a warning: code %d, message %s", e.ErrorCode, e.ErrorMessage)
}

type Response struct {
	Status string `json:"status"`
	Error  *Error `json:"error"`
}

type RecognitionResponse struct {
	Response
	Result  RecognitionResult `json:"result"`
	Warning *Warning          `json:"warning"`
}

type RecognitionEnterpriseResponse struct {
	Response
	Result []RecognitionEnterpriseResult `json:"result"`
	ExecutionTime string                 `json:"execution_time"`
}

type HummingRecognitionResponse struct {
	Response
	Result HummingRecognitionResult `json:"result"`
}

type FindLyricsResponse struct {
	Response
	Result []LyricsResult `json:"result"`
}

type GetCallbackUrlResponse struct {
	Response
	Result string `json:"result"`
}

type GetStreamsResponse struct {
	Response
	Result []Stream `json:"result"`
}


// Recognizes the music in the file
func (c *Client) RecognizeByFile(file io.Reader, Return string, additionalParameters map[string]string) (RecognitionResult, error) {
	return c.recognize(func(i interface{}, m map[string]string) error { return c.SendFileRequest(file, m, i) },
		Return, additionalParameters)
}

// Recognizes the music in the file available by the Url
func (c *Client) RecognizeByUrl(Url string, Return string, additionalParameters map[string]string) (RecognitionResult, error) {
	return c.recognize(func(i interface{}, m map[string]string) error { return c.SendUrlRequest(Url, m, i) },
		Return, additionalParameters)
}

// Recognizes the music. Accepts files as io.Reader or []byte and file URLs as string or url.URL
func (c *Client) Recognize(v interface{}, Return string, additionalParameters map[string]string) (RecognitionResult, error) {
	switch t := v.(type) {
	case io.Reader:
		return c.RecognizeByFile(t, Return, additionalParameters)
	case []byte:
		return c.RecognizeByFile(bytes.NewReader(t), Return, additionalParameters)
	case string:
		return c.RecognizeByUrl(t, Return, additionalParameters)
	case url.URL:
		return c.RecognizeByUrl(t.String(), Return, additionalParameters)
	default:
		return RecognitionResult{},
			fmt.Errorf("expected a file as io.Reader or []byte, or a file URL as string or url.URL")
	}
}

type call func (interface{}, map[string]string) error

func (c *Client) recognize(call call, Return string, additionalParameters map[string]string) (RecognitionResult, error) {
	if additionalParameters == nil {
		additionalParameters = map[string]string{}
	}
	additionalParameters["method"] = recognizeMethod
	additionalParameters["return"] = Return
	var response RecognitionResponse
	err := call(&response, additionalParameters)
	if err != nil {
		return RecognitionResult{}, err
	}
	if response.Warning != nil  && additionalParameters["warnings"]  != "off" {
		log.Println(response.Warning.Error())
	}
	if response.Error != nil {
		return RecognitionResult{}, response.Error
	}
	return response.Result, nil
}

// Recognizes the music in long (even hours-long or days-long) audio files
func (c *Client) RecognizeLongAudioByFile(file io.Reader, additionalParameters map[string]string) ([]RecognitionEnterpriseResult, error) {
	return c.recognizeLongAudio(func(i interface{}, m map[string]string) error { return c.SendFileRequest(file, m, i) },
		additionalParameters)
}

// Recognizes the music in long (even hours-long or days-long) audio files available by the Url
func (c *Client) RecognizeLongAudioByUrl(Url string,  additionalParameters map[string]string) ([]RecognitionEnterpriseResult, error) {
	return c.recognizeLongAudio(func(i interface{}, m map[string]string) error { return c.SendUrlRequest(Url, m, i) },
		additionalParameters)
}

// Recognizes the music in long (even hours-long or days-long) audio files
// Accepts files as io.Reader or []byte and file URLs as string or url.URL
func (c *Client) RecognizeLongAudio(v interface{}, additionalParameters map[string]string) ([]RecognitionEnterpriseResult, error) {
	switch t := v.(type) {
	case io.Reader:
		return c.RecognizeLongAudioByFile(t, additionalParameters)
	case []byte:
		return c.RecognizeLongAudioByFile(bytes.NewReader(t), additionalParameters)
	case string:
		return c.RecognizeLongAudioByUrl(t, additionalParameters)
	case url.URL:
		return c.RecognizeLongAudioByUrl(t.String(), additionalParameters)
	default:
		return nil,	fmt.Errorf("expected a file as io.Reader or []byte, or a file URL as string or url.URL")
	}
}

func (c *Client) recognizeLongAudio(call call, additionalParameters map[string]string) ([]RecognitionEnterpriseResult, error) {
	if additionalParameters == nil {
		additionalParameters = map[string]string{}
	}
	if _, methodSet := additionalParameters["method"]; c.Endpoint == MainAPIEndpoint && !methodSet {
		return nil, fmt.Errorf("can't send long audio files to the main endpoint, consider changing to audd.EnterpriseAPIEndpoint (enterprise.audd.io)")
	}
	if c.Endpoint != EnterpriseAPIEndpoint && additionalParameters["warnings"]  != "off" {
		log.Println("warning: the endpoint used is not audd.EnterpriseAPIEndpoint (enterprise.audd.io)")
	}
	var response RecognitionEnterpriseResponse
	err := call(&response, additionalParameters)
	if err != nil {
		return nil, err
	}
	if response.Error != nil {
		return nil, response.Error
	}
	return response.Result, nil
}

// [test feature] Recognizes the music in the file by humming
func (c *Client) RecognizeHumming(file io.Reader) ([]HummingResult, error) {
	parameters := map[string]string{"method": "recognizeWithOffset"}
	var response HummingRecognitionResponse
	err := c.SendFileRequest(file, parameters, &response)
	if err != nil {
		return nil, err
	}
	if response.Error != nil {
		return nil, response.Error
	}
	return response.Result.List, nil
}

// [test feature] Recognizes the music in the file available by the Url by humming
func (c *Client) RecognizeHummingByUrl(Url string) ([]HummingResult, error) {
	parameters := map[string]string{"method": recognizeHummingMethod}
	var response HummingRecognitionResponse
	err := c.SendUrlRequest(Url, parameters, &response)
	if err != nil {
		return nil, err
	}
	if response.Error != nil {
		return nil, response.Error
	}
	return response.Result.List, nil
}

// Finds the lyrics by the query
func (c *Client) FindLyrics(q string, additionalParameters map[string]string) ([]LyricsResult, error) {
	if additionalParameters == nil {
		additionalParameters = map[string]string{}
	}
	additionalParameters["method"] = findLyricsMethod
	additionalParameters["q"] = q
	var response FindLyricsResponse
	err := c.SendRequest(additionalParameters, &response)
	if err != nil {
		return nil, err
	}
	if response.Error != nil {
		return nil, response.Error
	}
	return response.Result, nil
}

// Sets the URL for callbacks.
// The callbacks with the information about songs recognized in your streams will be sent to the specified URL
func (c *Client) SetCallbackUrl(Url string, additionalParameters map[string]string) error {
	if additionalParameters == nil {
		additionalParameters = map[string]string{}
	}
	additionalParameters["method"] = setCallbackUrlMethod
	var response Response
	err := c.SendUrlRequest(Url, additionalParameters, &response)
	if err != nil {
		return err
	}
	if response.Error != nil {
		return response.Error
	}
	return nil
}

// Adds a stream
// Send empty callbacks parameter for the default mode (callbacks will be sent after the song ends),
// send callbacks='before' for receiving callbacks when new songs just start playing on the stream
func (c *Client) AddStream(Url string, RadioID int, callbacks string, additionalParameters map[string]string) error {
	if additionalParameters == nil {
		additionalParameters = map[string]string{}
	}
	if callbacks != "" && callbacks != "before" {
		return fmt.Errorf("the callbacks parameter should be either empty or 'before'")
	}
	additionalParameters["method"] = addStreamMethod
	additionalParameters["radio_id"] = strconv.Itoa(RadioID)
	additionalParameters["callbacks"] = callbacks
	var response Response
	err := c.SendUrlRequest(Url, additionalParameters, &response)
	if err != nil {
		return err
	}
	if response.Error != nil {
		return response.Error
	}
	return nil
}

// Sets the url of a stream
func (c *Client) SetStreamUrl(Url string, RadioID int, additionalParameters map[string]string) error {
	if additionalParameters == nil {
		additionalParameters = map[string]string{}
	}
	additionalParameters["method"] = setStreamUrlMethod
	additionalParameters["radio_id"] = strconv.Itoa(RadioID)
	var response Response
	err := c.SendUrlRequest(Url, additionalParameters, &response)
	if err != nil {
		return err
	}
	if response.Error != nil {
		return response.Error
	}
	return nil

}

// Returns the URL the callbacks are sent to
func (c *Client) GetCallbackUrl(additionalParameters map[string]string) (string, error) {
	if additionalParameters == nil {
		additionalParameters = map[string]string{}
	}
	additionalParameters["method"] = getCallbackUrlMethod
	var response GetCallbackUrlResponse
	err := c.SendRequest(additionalParameters, &response)
	if err != nil {
		return "", err
	}
	if response.Error != nil {
		return "", response.Error
	}
	return response.Result, nil
}

// Returns all the streams
func (c *Client) GetStreams(additionalParameters map[string]string) ([]Stream, error) {
	if additionalParameters == nil {
		additionalParameters = map[string]string{}
	}
	additionalParameters["method"] = getStreamsMethod
	var response GetStreamsResponse
	err := c.SendRequest(additionalParameters, &response)
	if err != nil {
		return nil, err
	}
	if response.Error != nil {
		return nil, response.Error
	}
	return response.Result, nil
}

// Deletes a stream
func (c *Client) DeleteStream(RadioID int, additionalParameters map[string]string) error {
	if additionalParameters == nil {
		additionalParameters = map[string]string{}
	}
	additionalParameters["method"] = deleteStreamMethod
	additionalParameters["radio_id"] = strconv.Itoa(RadioID)
	var response Response
	err := c.SendRequest(additionalParameters, &response)
	if err != nil {
		return err
	}
	if response.Error != nil {
		return response.Error
	}
	return nil
}