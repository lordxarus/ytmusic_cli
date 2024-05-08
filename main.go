package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"code.rocketnine.space/tslocum/cview"
	"github.com/faiface/beep"
	"github.com/faiface/beep/effects"
	"github.com/faiface/beep/speaker"
	"github.com/gdamore/tcell/v2"
	"github.com/joho/godotenv"
	"github.com/kluctl/go-embed-python/embed_util"
	"github.com/kluctl/go-embed-python/python"
	"github.com/lordxarus/ytmusic_cli/internal/python-libs/data"
	"github.com/lordxarus/ytmusic_cli/yt"
	"github.com/lordxarus/ytmusic_cli/yt/search"
	"github.com/zergon321/reisen"
)

var (
	// Keep paths clean with no trailing slash
	// TODO Could use filepath.Clean()
	homePath   string
	cachePath  string
	wdPath     string
	app        = cview.NewApplication()
	oauthToken string
	logging    = false
	// TODO songListDimensions So sorry....
	songListDimensions = [6]int{1, 0, 5, 5, 1, 4}
)

const (
	sampleRate                             = 44100
	channelCount                           = 2
	bitDepth                               = 8
	sampleBufferSize                       = 32 * channelCount * bitDepth * 1024
	SpeakerSampleRate      beep.SampleRate = 44100
	gKillDecoderBufferSize                 = 50
)

// TODO Can use struct tags here? https://www.digitalocean.com/community/tutorials/how-to-use-struct-tags-in-go

// TODO Create and hold a streamer in global scope?
// TODO We can rewrite this to only use beep
// TODO Beep supports playing and pausing
// TODO Notify the user if there isn't a valid token to use
func main() {
	if err := godotenv.Load(); err != nil {
		log.Fatal("For now you have to set your oauth token through a .env file. Bye-bye.")
	} else {
		oauthToken = os.Getenv("OAUTH_TOKEN")
		if oauthToken == "" {
			log.Fatalf("No OAUTH_TOKEN in .env")
		}
		logVar := os.Getenv("LOGGING")
		if logVar != "" {
			logging, err = strconv.ParseBool(logVar)
			if err != nil {
				if !logging {
					log.Println("Disabling logging. Bye-bye.")
					log.SetOutput(io.Discard)
				}
			}
		}
	}

	var ok bool
	var err error

	var selectedSong yt.Song
	var songResults []yt.Song

	var grid *cview.Grid
	var searchBox *cview.InputField
	var playButton *cview.Button
	var songList *cview.List

	var volumeEffect *effects.Volume = &effects.Volume{
		Streamer: nil,
		Base:     2,
		Volume:   -1.0,
		Silent:   false,
	}
	var volumeText *cview.TextView

	if homePath, ok = os.LookupEnv("HOME"); !ok {
		log.Fatalf("Couldn't lookup home directory in env")
	}

	cachePath = homePath + "/.cache/ytmusic_cli"

	if wdPath, err = os.Getwd(); err != nil {
		log.Fatalf("Couldn't find working directory in env %s", err)
	}

	log.Printf("Home: %s", homePath)
	log.Printf("Cache: %s", cachePath)
	log.Printf("Working Dir: %s", wdPath)

	if err = os.MkdirAll(cachePath, 0o755); err != nil {
		log.Fatalf("Couldn't create cache directory. Err: %s", err)
	}

	// Init speaker
	err = speaker.Init(sampleRate, SpeakerSampleRate.N(time.Second/10))
	if err != nil {
		log.Fatalf("Unable to init speaker %s", err)
	}

	// TODO Temp
	if len(os.Args) == 1 {
		songResults = query("Little Big", search.Songs)
	} else {
		songResults = query(os.Args[1], search.Songs)
	}

	yt.DownloadVideo(songResults[0].VideoId, cachePath, true)

	if len(songResults) > 0 {
		selectedSong = songResults[0]
	}

	var killDecoder chan<- bool = make(chan<- bool, gKillDecoderBufferSize)

	// Build CUI
	playButton = cview.NewButton("Play")
	playButton.SetSelectedFunc(
		func() {
			switch playButton.GetLabel() {
			case "Play":
				log.Printf("playButton: selected song: %+v", selectedSong)
				killDecoder = play(selectedSong, volumeEffect, false)
				playButton.SetLabel("Pause")
			case "Pause":
				killDecoder <- true
				speaker.Clear()
				playButton.SetLabel("Play")
			}
		})

	songList = createSongList(songResults, &selectedSong, killDecoder, volumeEffect, playButton)
	// Add songs

	searchBox = cview.NewInputField()
	searchBox.SetLabel("Search: ")
	searchBox.SetDoneFunc(func(key tcell.Key) {
		if key == tcell.KeyEnter {
			log.Println("enter")
			newList := createSongList(query(searchBox.GetText(), search.Songs), &selectedSong, killDecoder, volumeEffect, playButton)
			app.Lock()
			grid.RemoveItem(songList)
			addSongList(grid, newList, &songListDimensions, false)
			songList = newList
			app.Unlock()
			app.Draw()
			log.Printf("%+v", songList)
		}
	})

	volumeText = cview.NewTextView()
	volumeText.SetText("Volume: 1.0")
	volumeText.SetBorder(true)
	volumeText.SetMouseCapture(func(action cview.MouseAction, event *tcell.EventMouse) (cview.MouseAction, *tcell.EventMouse) {
		if action == cview.MouseScrollUp {
			volumeEffect.Volume += .1
		} else if action == cview.MouseScrollDown {
			volumeEffect.Volume -= .1
		}

		volStr := strconv.FormatFloat(volumeEffect.Volume, 'f', 2, 64)
		volumeText.SetText(fmt.Sprintf("Volume: %s", volStr))

		return action, event
	})

	grid = cview.NewGrid()
	grid.AddItem(searchBox, 0, 0, 1, 3, 1, 3, false)
	addSongList(grid, songList, &songListDimensions, false)
	grid.AddItem(playButton, 5, 0, 1, 5, 1, 5, false)
	grid.AddItem(volumeText, 5, 5, 1, 1, 0, 0, false)

	// Keyboard input
	grid.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyEsc {
			app.Stop()
		}
		return event
	})

	frame := cview.NewFrame(grid)
	frame.AddText("Youtube Music CLI", true, cview.AlignCenter, tcell.ColorHotPink)

	app.SetRoot(frame, true)
	app.EnableMouse(true)
	if err := app.Run(); err != nil {
		log.Fatalf("Failed to run app: %s", err)
	}
}

// TODO REMOVE THIS
func addSongList(grid *cview.Grid, list *cview.List, dim *[6]int, focus bool) {
	grid.AddItem(list, dim[0], dim[1], dim[2], dim[3], dim[4], dim[5], focus)
}

func createSongList(songs []yt.Song, selectedSong *yt.Song,
	killDecoder chan<- bool, volumeEffect *effects.Volume,
	playButton *cview.Button,
) *cview.List {
	songList := cview.NewList()
	for _, song := range songs {
		// Collect the artist names for this song
		var artistNames []string
		for _, artist := range song.Artists {
			artistNames = append(artistNames, artist.Name)
		}
		li := cview.NewListItem(song.Title)
		li.SetSecondaryText(fmt.Sprintf("%s - %s", strings.Join(artistNames, ", "), song.Duration))
		li.SetSelectedFunc(
			func() {
				*selectedSong = song
				killDecoder <- true
				killDecoder = play(song, volumeEffect, false)
				playButton.SetLabel("Pause")
			})
		songList.AddItem(li)
	}
	return songList
}

func query(query string, filter search.Filter) []yt.Song {
	// Search

	// TODO EMBEDDED PYTHON VERSION
	fs, err := embed_util.NewEmbeddedFiles(data.Data, "ytmusicapi")
	if err != nil {
		log.Printf("Failed to new embedded files %s", err)
	}
	ep, err := python.NewEmbeddedPython("ytmusicapi")
	if err != nil {
		log.Printf("Failed to created embedded python %s", err)
	}
	ep.AddPythonPath(fs.GetExtractedPath())

	pyCmd := ep.PythonCmd("-c", fmt.Sprintf(
		`
from ytmusicapi import YTMusic
import sys
	
ytmusic = YTMusic('%s')
	
res = ytmusic.search("%s", filter="%s")
	
import json
	
# https://stackoverflow.com/questions/36021332/how-to-prettyprint-human-readably-print-a-python-dict-in-json-format-double-q
print(json.dumps(
	res,
	sort_keys=True,
	indent=4,
	separators=(',', ': ')
))`,
		oauthToken, query, filter))

	// 	log.Printf(
	// 		`
	// from ytmusicapi import YTMusic
	// import sys

	// ytmusic = YTMusic('%s')

	// res = ytmusic.search("%s", filter="%s")

	// import json

	// # https://stackoverflow.com/questions/36021332/how-to-prettyprint-human-readably-print-a-python-dict-in-json-format-double-q
	// print(json.dumps(
	// 	res,
	// 	sort_keys=True,
	// 	indent=4,
	// 	separators=(',', ': ')
	// ))`,
	// 		oauthToken, query, filter)

	stdout, _ := pyCmd.Output()

	songResults := make([]yt.Song, 0, 200)

	err = json.Unmarshal(stdout, &songResults) // https://betterstack.com/community/guides/scaling-go/json-in-go/
	if err != nil {
		log.Fatalf("Unable to marshal JSON due to %s", err)
	}
	return songResults
}

func play(song yt.Song, volume *effects.Volume, silent bool) chan<- bool {
	log.Printf("Starting download of %s, ID: %s", song.Title, song.VideoId)
	yt.DownloadVideo(song.VideoId, cachePath, true)
	_, streamer, killDecoder := loadAudio(filepath.Join(cachePath, song.VideoId+".mp4"))
	volume.Streamer = streamer
	speaker.Clear()
	speaker.Play(volume)
	return killDecoder
}

func loadAudio(path string) (*reisen.Media, beep.Streamer, chan<- bool) {
	media, err := reisen.NewMedia(path)
	if err != nil {
		log.Fatalf("Unable to create new media %s", err)
	}

	var sampleSource <-chan [2]float64
	sampleSource, killDecoder, errs, err := readVideoAndAudio(media, gKillDecoderBufferSize)
	go func(errs chan error) {
		for {
			err, ok := <-errs
			if !ok {
				break
			} else if err != nil {
				log.Printf("Decoding error: %s", err)
			}
		}
	}(errs)

	streamer := createStreamer(sampleSource)

	if err != nil {
		log.Fatalf("Can't readVideoAndAudio %s", err)
	}

	return media, streamer, killDecoder
}

// https://medium.com/@maximgradan/playing-videos-with-golang-83e67447b111
// readVideoAndAudio reads video and audio frames
// from the opened media and sends the decoded
// data to che channels to be played.
func readVideoAndAudio(media *reisen.Media, killDecoderBufferSize int) (<-chan [2]float64, chan<- bool, chan error, error) {
	sampleBuffer := make(chan [2]float64, sampleBufferSize)
	errs := make(chan error)
	killDecoder := make(chan bool, killDecoderBufferSize)

	err := media.OpenDecode()
	if err != nil {
		return nil, nil, nil, err
	}

	var audioStream *reisen.AudioStream = nil
	log.Printf("Found %d stream(s)", media.StreamCount())
	for _, stream := range media.Streams() {
		if stream.Type() == reisen.StreamAudio {
			audioStream = stream.(*reisen.AudioStream)
			break
		}
	}

	if audioStream == nil {
		log.Println("Failed to find an audio stream")
	}

	err = audioStream.Open()
	if err != nil {
		return nil, nil, nil, err
	}

	go func() {
	Loop:
		for {
			select {
			case <-killDecoder:
				log.Println("Killing decoder!")
				break Loop
			default:
				packet, gotPacket, err := media.ReadPacket()
				if err != nil {
					go func(err error) {
						errs <- err
					}(err)
				}

				if !gotPacket {
					break
				}

				switch packet.Type() {
				case reisen.StreamAudio:
					s := media.Streams()[packet.StreamIndex()].(*reisen.AudioStream)
					audioFrame, gotFrame, err := s.ReadAudioFrame()
					if err != nil {
						go func(err error) {
							errs <- err
						}(err)
					}

					if !gotFrame {
						break
					}

					if audioFrame == nil {
						continue
					}

					// Turn the raw byte data into
					// audio samples of type [2]float64.
					reader := bytes.NewReader(audioFrame.Data())

					// See the README.md file for
					// detailed scheme of the sample structure.
					for reader.Len() > 0 {
						sample := [2]float64{0, 0}
						var result float64
						err = binary.Read(reader, binary.LittleEndian, &result)
						if err != nil {
							go func(err error) {
								errs <- err
							}(err)
						}

						sample[0] = result

						err = binary.Read(reader, binary.LittleEndian, &result)
						if err != nil {
							go func(err error) {
								errs <- err
							}(err)
						}

						sample[1] = result
						sampleBuffer <- sample
					}
				}
			}
		}

		audioStream.Close()
		media.CloseDecode()
		close(sampleBuffer)
		close(errs)
	}()

	return sampleBuffer, killDecoder, errs, nil
}

// streamSamples creates a new custom streamer for
// playing audio samples provided by the source channel.
//
// See https://github.com/faiface/beep/wiki/Making-own-streamers
// for reference.
func createStreamer(sampleSource <-chan [2]float64) beep.Streamer {
	return beep.StreamerFunc(func(samples [][2]float64) (n int, ok bool) {
		numRead := 0

		for i := 0; i < len(samples); i++ {
			sample, ok := <-sampleSource

			if !ok {
				numRead = i + 1
				break
			}

			samples[i] = sample
			numRead++
		}

		if numRead < len(samples) {
			return numRead, false
		}

		return numRead, true
	})
}
