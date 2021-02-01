
/*
 * Copyright (c) 2020 AudD, LLC. All rights reserved.
 * Copyright (c) 2020 Mikhail Samin. All rights reserved.
 *
 * Redistribution and use in source and binary forms, with or without
 * modification, are permitted provided that the following conditions are
 * met:
 *
 *    * Redistributions of source code must retain the above copyright
 * notice, this list of conditions and the following disclaimer.
 *    * Redistributions in binary form must reproduce the above
 * copyright notice, this list of conditions and the following disclaimer
 * in the documentation and/or other materials provided with the
 * distribution.
 *    * Neither the name of Mikhail Samin, AudD, nor the names of its
 * contributors may be used to endorse or promote products derived from
 * this software without specific prior written permission.
 *
 * THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS
 * "AS IS" AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT
 * LIMITED TO, THE IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR
 * A PARTICULAR PURPOSE ARE DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT
 * OWNER OR CONTRIBUTORS BE LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL,
 * SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT
 * LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES; LOSS OF USE,
 * DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND ON ANY
 * THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT
 * (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE
 * OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.
 */

package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"github.com/AudDMusic/audd-go"
	"github.com/bogem/id3v2"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

func RecognizeMultipleFiles(dir string, client *audd.Client, setID3, short bool, skip int) (map[string][]audd.RecognitionResult, error) {
	files, _ := ioutil.ReadDir(dir)
	if len(files) == 0 {
		return nil, fmt.Errorf("no audio files found")
	}
	results := make(map[string][]audd.RecognitionResult)
	var mu = &sync.Mutex{}
	wg := &sync.WaitGroup{}
	for i, fileInfo := range files {
		if fileInfo.IsDir() { // Skip folders. As an option, you can implement a recursion
			continue
		}
		if i % 15 == 0 { // Send files by bunches
			wg.Wait()
		}
		wg.Add(1)
		go func(fileInfo os.FileInfo, i int, dir string) {
			defer wg.Done()
			fileName := dir + string(os.PathSeparator) + fileInfo.Name()
			file, err := os.Open(fileName)
			if err != nil {
				fmt.Println("Error:", err)
				return
			}
			params := map[string]string{"skip": strconv.Itoa(skip), "every": "1"} // see docs-e.audd.io
			recognitionResult, err := client.RecognizeLongAudio(file, params)
			if err != nil {
				fmt.Println("Error:", err)
				return
			}
			file.Close()
			result := make([]audd.RecognitionResult, 0)
			for i := range recognitionResult {
				for j := range recognitionResult[i].Songs {
					// an unused field here is not the best way to keep the information, but it was convenient
					recognitionResult[i].Songs[j].SongLength = recognitionResult[i].Offset
				}
				if len(recognitionResult[i].Songs) > 0 {
					if short {
						result = append(result, recognitionResult[i].Songs[0])
					} else {
						result = append(result, recognitionResult[i].Songs...)
					}
				}
			}
			mu.Lock()
			results[fileInfo.Name()] = result
			mu.Unlock()
			if setID3 && len(result) > 0 {
				i := 0
				lastTitle := ""
				for j, song := range result {
					if song.Title == lastTitle {
						i = j
						break
					}
					lastTitle = song.Title
				}
				mp3File, err := id3v2.Open(fileName, id3v2.Options{Parse: true})
				if err != nil {
					fmt.Println("Error:", err)
					return
				}
				defer mp3File.Close()
				mp3File.SetArtist(result[i].Artist)
				mp3File.SetTitle(result[i].Title)
				mp3File.SetAlbum(result[i].Album)
				cover := result[i].SongLink + "?thumb"
				if strings.Contains(result[i].SongLink, "youtu.be/"){
					cover = "https://i3.ytimg.com/vi/"+strings.ReplaceAll(result[i].SongLink, "https://youtu.be/", "")+"/maxresdefault.jpg"
				}
				response, err := http.Get(cover)
				defer closeBody(response)
				if err != nil {
					fmt.Println("Error: can't get the cover for", fileName, err)
				} else {
					b, err := ioutil.ReadAll(response.Body)
					if err != nil {
						fmt.Println("Error: can't get the cover for", fileName, err)
					} else {
						frontCover := id3v2.PictureFrame{
							Encoding:    id3v2.EncodingUTF8,
							MimeType:    response.Header.Get("content-type"),
							PictureType: id3v2.PTFrontCover,
							Picture:     b,
						}
						mp3File.AddAttachedPicture(frontCover)
					}
				}
				releaseDate := strings.Split(result[i].ReleaseDate, "-")
				if len(releaseDate) > 0 {
					mp3File.SetYear(releaseDate[0])
				}
				err = mp3File.Save()
				if err != nil {
					fmt.Println("Error: can't save the ID3 for", fileName, err)
				}
			}
		}(fileInfo, i, dir)
	}
	wg.Wait()
	return results, nil
}

func CreateCSV(songs map[string][]audd.RecognitionResult, path string, short bool) {
	records := make([][]string, 0)
	for name, songResults := range songs {
		titles := map[string]struct{}{}
		for _, song := range songResults {
			if short {
				if _, seen := titles[song.Title]; seen {
					continue
				}
				titles[song.Title] = struct{}{}
				records = append(records, []string{name, song.Artist, song.Title,
					song.Album, song.Label, song.ReleaseDate, song.SongLink})
			} else {
				records = append(records, []string{name, song.SongLength, song.Timecode, song.Artist, song.Title,
					song.Album,	song.Label, song.ReleaseDate, song.SongLink})
			}
		}
	}
	if len(records) == 0 {
		fmt.Println("No songs are recognized. Quitting.")
		return
	}
	file, _ := os.Create(path)
	w := csv.NewWriter(file)
	if short {
		w.Write([]string{"File name", "Artist", "Title", "Album", "Label", "Release date", "Listen online"})
	} else {
		w.Write([]string{"File name", "Plays On [file]", "Plays On [song]", "Artist", "Title", "Album", "Label", "Release date", "Listen online"})
	}
	for _, record := range records {
		if err := w.Write(record); err != nil {
			log.Fatalln("error writing a line to the csv:", err)
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		log.Fatal(err)
	}
	fmt.Println("Created a csv:", path)
}

func getCurrentDir() string {
	currentFile, _ := os.Executable()
	return filepath.Dir(currentFile)
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

func main() {
	dir := flag.String("dir", getCurrentDir(), "The directory with the files")
	apiToken := flag.String("api_token", "test", "AudD API token")
	id3Flag := flag.Bool("id3", true, "Change ID3 tags of audio files")
	short := flag.Bool("short", false, "Short version of the CSV without duplicates")
	skip := flag.Int("skip", 2, "How many 18-secodns chunks to skip in every file (see docs-e.audd.io)")
	flag.Parse()
	client := audd.NewClient(*apiToken)
	client.SetEndpoint(audd.EnterpriseAPIEndpoint)
	client.UseExperimentalUploading()
	fmt.Println("Sending files to the AudD API...")
	songs, err := RecognizeMultipleFiles(*dir, client, *id3Flag, *short, *skip)
	if err != nil {
		panic(err)
	}
	fmt.Println("Creating csv...")
	pathToCSV := *dir + string(os.PathSeparator) + "audd.csv"
	CreateCSV(songs, pathToCSV, *short)
	time.Sleep(time.Second * 2)
}
