package main

import (
	"bufio"
	"encoding/csv"
	"flag"
	"fmt"
	"github.com/AudDMusic/audd-go"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
)

var fileW *csv.Writer
var fileMu = &sync.Mutex{}
var MinScore int

func main() {
	pathToCSV := flag.String("csv", "audd.csv", "Path to the .csv file")
	apiToken := flag.String("api_token", "", "the AudD API token")
	minScore := flag.Int("min_score", 85, "The minimum score (if a result has score below specified, it won't be processed)")
	flag.Parse()
	reader := bufio.NewReader(os.Stdin)
	if *apiToken == "" {
		fmt.Println("Note that you can paste from the clipboard using the right click")
		fmt.Println("Please enter the api_token (you can copy it from https://dashboard.audd.io):")
		text, _ := reader.ReadString('\n')
		*apiToken = strings.TrimSpace(text)
	}
	MinScore = *minScore
	var writeHeaders bool
	if _, err := os.Stat(*pathToCSV); os.IsNotExist(err) {
		writeHeaders = true
	}
	file, err := os.OpenFile(*pathToCSV, os.O_RDWR|os.O_APPEND|os.O_CREATE, 0666)
	if err != nil {
		fmt.Println("Can't open the file", *pathToCSV)
		panic(err)
	}
	fileW = csv.NewWriter(file)
	if writeHeaders {
		capture(fileW.Write([]string{"Time", "Stream ID", "ISRC", "UPC",
			"Artist", "Title", "Album", "Label", "ReleaseDate", "Score", "Played length"}))
		fileW.Flush()
	}

	auddClient := audd.NewClient(*apiToken)
	streams, err := auddClient.GetStreams(nil)
	if capture(err) {
		panic(err)
	}
	ResultChan := merge(make(chan audd.StreamCallback))
	for _, s := range streams {
		lp := auddClient.NewLongPoll(s.RadioID)
		ResultChan = merge(ResultChan, lp.ResultsChan)
		defer lp.Stop()
	}
	go func() {
		for {
			select {
			case e := <-ResultChan:
				processCallback(e)
			}
		}
	}()
	fmt.Println("Type `exit` to exit")
	for {
		text, _ := reader.ReadString('\n')
		text = strings.TrimSpace(text)
		if text == "exit" {
			return
		}
	}
}

func merge(cs ...<-chan audd.StreamCallback) <-chan audd.StreamCallback {
	out := make(chan audd.StreamCallback, 5)
	var wg sync.WaitGroup
	wg.Add(len(cs))
	for _, c := range cs {
		go func(c <-chan audd.StreamCallback) {
			for v := range c {
				out <- v
			}
			wg.Done()
		}(c)
	}
	go func() {
		wg.Wait()
		close(out)
	}()
	return out
}

func processCallback(result audd.StreamCallback) {
	if len(result.Result.Results) == 0 {
		fmt.Println("got a notification", result.Notification.Message)
		return
	}
	if result.Result.Results[0].Score < MinScore {
		fmt.Printf("skipping a result because of the low score (%d): %s - %s, %s", result.Result.Results[0].Score,
			result.Result.Results[0].Artist, result.Result.Results[0].Title, result.Result.Results[0].SongLink)
		return
	}
	writeResult(result.Result)
}
func writeResult(r *audd.StreamRecognitionResult) {
	fileMu.Lock()
	defer fileMu.Unlock()
	song := r.Results[0]
	capture(fileW.Write([]string{r.Timestamp, strconv.Itoa(r.RadioID), song.ISRC, song.UPC, song.Artist, song.Title,
		song.Album, song.Label, song.ReleaseDate, strconv.Itoa(song.Score), strconv.Itoa(r.PlayLength)}))
	fileW.Flush()
}

func capture(err error) bool {
	if err == nil {
		return false
	}
	_, file, no, ok := runtime.Caller(1)
	if ok {
		err = fmt.Errorf("%v from %s#%d", err, file, no)
	}
	// go raven.CaptureError(err, nil)
	go fmt.Println(err)
	return true
}
func init() {
	/* err := raven.SetDSN("")
	if err != nil {
		panic(err)
	} */
}
