package audd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"mime/multipart"
	"net/http"
	"net/url"
)

// API Endpoints
const (
	MainAPIEndpoint       string = "https://api.audd.io/"
	EnterpriseAPIEndpoint string = "https://enterprise.audd.io/"
)

type Client struct {
	ApiToken     string
	Endpoint     string
	Experimental bool
}

func NewClient(apiToken string) *Client {
	return &Client{
		ApiToken: apiToken,
		Endpoint: MainAPIEndpoint,
	}
}

// Sets the endpoint used
func (c *Client) SetEndpoint(APIEndpoint string) {
	c.Endpoint = APIEndpoint
}

// Call this if you want to actually start sending files without completely loading them in the memory
// Can lead to unexpected issues, like if the io.Reader returns an error while uploading is in progress, it can possibly still be counted as a request
func (c *Client) UseExperimentalUploading() {
	c.Experimental = true
}

// Sends a file request to the API
func (c *Client) SendFile(file io.Reader, parameters map[string]string) ([]byte, error) {
	if parameters == nil {
		parameters = map[string]string{}
	}
	parameters["api_token"] = c.ApiToken
	if c.Experimental {
		errCh := make(chan error, 1)
		r, w := io.Pipe()
		writer := multipart.NewWriter(w)
		ctx, cancel := context.WithCancel(context.Background())
		req, err := http.NewRequestWithContext(ctx, "POST", c.Endpoint, r)
		if err != nil {
			return nil, err
		}
		req.TransferEncoding = []string{"chunked"}
		req.Header.Set("Content-Type", writer.FormDataContentType())
		go func() {
			var part io.Writer
			part, err := writer.CreateFormFile("file", "file")
			if err != nil {
				errCh <- err
				cancel()
				return
			}
			_, err = io.Copy(part, file)
			if err != nil {
				errCh <- err
				cancel()
				return
			}
			for key, value := range parameters {
				err = writer.WriteField(key, value)
				if err != nil {
					errCh <- err
					cancel()
					return
				}
			}
			if err = writer.Close(); err != nil {
				errCh <- err
				cancel()
				return
			}
			close(errCh)
			if err = w.Close(); err != nil {
				fmt.Println(err)
			}
		}()
		response, err := http.DefaultClient.Do(req)
		defer closeBody(response)
		if err, any := <-errCh; any {
			if err != nil {
				return nil, err
			}
		}
		if err != nil {
			return nil, err
		}
		return getResponse(response)
	}
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
	for key, value := range parameters {
		_ = writer.WriteField(key, value)
	}
	err = writer.Close()
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest("POST", c.Endpoint, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	response, err := http.DefaultClient.Do(req)
	defer closeBody(response)
	if err != nil {
		return nil, err
	}
	return getResponse(response)
}

// Sends a request with the URL specified
func (c *Client) SendUrl(URL string, parameters map[string]string) ([]byte, error) {
	parameters["url"] = URL
	return c.Send(parameters)
}

// Sends a requests to the API
func (c *Client) Send(parameters map[string]string) ([]byte, error) {
	if parameters == nil {
		parameters = map[string]string{}
	}
	parameters["api_token"] = c.ApiToken
	fields := url.Values{}
	for key, value := range parameters {
		fields.Add(key, value)
	}
	response, err := http.PostForm(c.Endpoint, fields)
	defer closeBody(response)
	if err != nil {
		return nil, err
	}
	return getResponse(response)
}

// Sends a request returns the result into the v
func (c *Client) SendRequest(parameters map[string]string, v interface{}) error {
	result, err := c.Send(parameters)
	return handleApiResponse(result, err, v)
}

// Sends a request with a file and returns the result into the v
func (c *Client) SendFileRequest(file io.Reader, parameters map[string]string, v interface{}) error {
	result, err := c.SendFile(file, parameters)
	return handleApiResponse(result, err, v)
}

// Sends a request with a file URL and returns the result into the v
func (c *Client) SendUrlRequest(url string, parameters map[string]string, v interface{}) error {
	result, err := c.SendUrl(url, parameters)
	return handleApiResponse(result, err, v)
}
func getResponse(response *http.Response) ([]byte, error) {
	return ioutil.ReadAll(response.Body)
}
func closeBody(resp *http.Response) {
	if resp == nil {
		return
	}
	if resp.Body == nil {
		return
	}
	_ = resp.Body.Close()
}

func handleApiResponse(requestResult []byte, err error, v interface{}) error {
	if err != nil {
		return err
	}
	err = json.Unmarshal(requestResult, v)
	if err != nil {
		return err
	}
	return nil
}
