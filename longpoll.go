package audd

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"github.com/Mihonarium/golongpoll/go-client/glpclient"
	"net/url"
	"strconv"
)

const longPollingUrl = MainAPIEndpoint + "longpoll/"

func getMD5Hash(text string) string {
	hasher := md5.New()
	hasher.Write([]byte(text))
	return hex.EncodeToString(hasher.Sum(nil))
}

func (c *Client) getLongPollCategory(RadioID int) string {
	return getMD5Hash(getMD5Hash(c.ApiToken) + strconv.Itoa(RadioID))[0:9]
}

type LongPoll struct {
	stop chan interface{}
	ResultsChan chan StreamCallback
}

func (lp *LongPoll) Stop() {
	lp.stop <- struct {}{}
}

func (c *Client) ConnectToLongPoll(RadioID int) LongPoll {
	u, _ := url.Parse(longPollingUrl)
	lpC := glpclient.NewClient(u, c.getLongPollCategory(RadioID))
	lpC.LoggingEnabled = false
	lp := LongPoll{
		stop:    make(chan interface{}, 1),
		ResultsChan: make(chan StreamCallback, 1),
	}
	go func() {
		lpC.Start()
		for {
			select {
			case e := <-lpC.EventsChan:
				var song StreamCallback
				err := json.Unmarshal(e.Data, &song)
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
