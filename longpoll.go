package audd

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	glpclient "github.com/jcuga/golongpoll/client"
	"net/url"
	"strconv"
	"time"
)

const longPollingUrl = MainAPIEndpoint + "longpoll/"

func getMD5Hash(text string) string {
	hasher := md5.New()
	hasher.Write([]byte(text))
	return hex.EncodeToString(hasher.Sum(nil))
}

func (c *Client) getLongPollChannel(RadioID int) string {
	return getMD5Hash(getMD5Hash(c.ApiToken) + strconv.Itoa(RadioID))[0:9]
}

type LongPoll struct {
	stop        chan interface{}
	ResultsChan chan StreamCallback
}

// Stops the LongPoll connection
func (lp *LongPoll) Stop() {
	lp.stop <- struct{}{}
}

// Opens a LongPoll connection to the AudD API and receives the callbacks via LongPoll.
// The callbacks will be sent to both the callback URL and all the LongPoll listeners.
// Won't work unless some URL is set as the URL for callbacks. More info: docs.audd.io/streams/#longpoll
func (c *Client) NewLongPoll(RadioID int) LongPoll {
	u, _ := url.Parse(longPollingUrl)
	lpC, _ := glpclient.NewClient(glpclient.ClientOptions{
		SubscribeUrl:   *u,
		Category:       c.getLongPollChannel(RadioID),
		LoggingEnabled: false,
	})
	lp := LongPoll{
		stop:        make(chan interface{}, 1),
		ResultsChan: make(chan StreamCallback, 1),
	}
	go func() {
		EventsChan := lpC.Start(time.Now())
		for {
			select {
			case e := <-EventsChan:
				fmt.Println("test4")
				data, _ := json.Marshal(e.Data)
				fmt.Println("test5")
				var song StreamCallback
				fmt.Println("test6")
				err := json.Unmarshal(data, &song)
				fmt.Println("test7")
				if err == nil {
					lp.ResultsChan <- song
				}
			case <-lp.stop:
				lpC.Stop()
				return
			}
		}
	}()
	return lp
}
