package audd

import (
	"bytes"
	"encoding/json"
	"io"
	"io/ioutil"
	"mime/multipart"
	"net/http"
	"net/url"
)

// API Endpoints
const (
	APIEndpoint string = "https://api.audd.io/"
	//EnterpriseEndpoint string = "https://enterprise.audd.io/"
)

// FileRequest posts a file to AudD
func FileRequest(file io.Reader, ApiToken string, data map[string]string) ([]byte, error) {
	data["api_token"] = ApiToken
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", "file")
	if err != nil {
		return nil, err
	}
	_, err = io.Copy(part, file)
	if err != nil {
		return nil, err
	}
	for key, value := range data {
		_ = writer.WriteField(key, value)
	}
	err = writer.Close()
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest("POST", APIEndpoint, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	client := &http.Client{}
	response, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	return getResponse(response)
}

// Request makes an API request
func Request(data map[string]string, ApiToken string) ([]byte, error) {
	data["api_token"] = ApiToken
	fields := url.Values{}
	for key, value := range data {
		fields.Add(key, value)
	}
	response, err := http.PostForm(APIEndpoint, fields)
	if err != nil {
		return nil, err
	}
	return getResponse(response)
}

func getResponse(response *http.Response) ([]byte, error) {
	responseBody, err := ioutil.ReadAll(response.Body)
	err = response.Body.Close()
	if err != nil {
		return responseBody, err
	}
	return responseBody, nil
}

func fileRequest(file io.Reader, ApiToken string, parameters map[string]string, method string) ([]byte, error) {
	if parameters == nil {
		parameters = make(map[string]string)
	}
	parameters["method"] = method
	return FileRequest(file, ApiToken, parameters)
}

func urlRequest(url string, ApiToken string, parameters map[string]string, method string) ([]byte, error) {
	if parameters == nil {
		parameters = make(map[string]string)
	}
	parameters["url"] = url
	parameters["method"] = method
	return Request(parameters, ApiToken)
}

func handleRecognitionResponse(requestResult []byte, err error) (RecognitionResponse, error) {
	var response RecognitionResponse
	err = handleResponse(requestResult, err, &response)
	return response, err
}
func handleRecognitionAllMatchesResponse(requestResult []byte, err error) (RecognitionResponseAllMatches, error) {
	var response RecognitionResponseAllMatches
	err = handleResponse(requestResult, err, &response)
	return response, err
}
func handleHummingRecognitionResponse(requestResult []byte, err error) (HummingRecognitionResponse, error) {
	var response HummingRecognitionResponse
	err = handleResponse(requestResult, err, &response)
	return response, err
}
func handleFindLyricsResponse(requestResult []byte, err error) (FindLyricsResponse, error) {
	var response FindLyricsResponse
	err = handleResponse(requestResult, err, &response)
	return response, err
}
func handleResponse(requestResult []byte, err error, v interface{}) error {
	if err != nil {
		return err
	}
	err = json.Unmarshal(requestResult, v)
	if err != nil {
		return err
	}
	return nil
}
