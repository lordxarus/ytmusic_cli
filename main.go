package main

import (
	"bytes"
	"encoding/binary"
	"errors"
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
	brandId    string
	logging    = false
	ytm        *yt.YTMClient
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

// TODO We can rewrite this to only use beep
// TODO Beep supports playing and pausing
// TODO I should probably create my own types for use in my own internal stuff.
// I currently have the Song result that you get from the search end point implemented.
// But when I go to, for example, implement the Song result type that you would get from
// the home screen it will be a completely different Song type. So I need to namespace them
// similarly to ytmusicapi. Based on which endpoint they come from. yt.search.Song and yt.home.Song or something.
// And then I can have my own Song type for my GUI which will include only what I need.
// TODO Use ffplay instead of beep
func main() {
	var ok bool
	var err error

	var songResults []yt.Song

	var rootFlex *cview.Flex
	var songFlex *cview.Flex
	var controlsFlex *cview.Flex

	var searchBox *cview.InputField
	var playButton *cview.Button
	var songList *cview.List
	var progressBar *cview.ProgressBar

	var volumeEffect *effects.Volume = &effects.Volume{
		Streamer: nil,
		Base:     2,
		Volume:   -1.0,
		Silent:   false,
	}
	var volumeText *cview.TextView

	if err := godotenv.Load(); err != nil {
		log.Fatal("set your oauth token through a .env file")
	} else {
		oauthToken = os.Getenv("OAUTH_TOKEN")
		brandId = os.Getenv("BRAND_ID")
		logVar := os.Getenv("LOGGING")

		switch {
		case oauthToken == "":
			log.Fatalf("no field OAUTH_TOKEN in .env")
		case brandId == "":
			log.Println("no brand ID in .env using default")
		case logVar != "":
			logging, err = strconv.ParseBool(logVar)
			if err != nil && !logging {
				log.Println("disabling logging")
				log.SetOutput(io.Discard)
			}
		}

	}

	if homePath, ok = os.LookupEnv("HOME"); !ok {
		log.Fatalf("main() couldn't find home directory")
	}

	cachePath = homePath + "/.cache/ytmusic_cli"

	if wdPath, err = os.Getwd(); err != nil {
		log.Fatalf("main() couldn't find working directory: %s", err)
	}

	log.Printf("Home: %s", homePath)
	log.Printf("Cache: %s", cachePath)
	log.Printf("Working Dir: %s", wdPath)

	if err = os.MkdirAll(cachePath, 0o750); err != nil {
		log.Fatalf("main() couldn't create cache directory: %s", err)
	}

	ytm = yt.New(oauthToken, brandId)

	// Init speaker
	err = speaker.Init(sampleRate, SpeakerSampleRate.N(time.Second/10))
	if err != nil {
		log.Fatalf("main() unable to init speaker: %s", err)
	}

	query := "Best Classical Music"
	// TODO Temp
	if len(os.Args) > 2 {
		query = os.Args[1]
	}

	songResults, err = ytm.Query(query, search.Songs)
	if err != nil {
		log.Fatalf("main() initial query failed: %s", err)
	}

	killDecoder := make(chan bool, 10)

	err = yt.DownloadVideo(songResults[0].VideoId, cachePath, true)
	if err != nil {
		log.Fatalf("main() failed to download video: %s", err)
	}

	songSelected := func() {
		// *selectedSong = song
		ss, ok := songList.GetCurrentItem().GetReference().(yt.Song)
		if !ok {
			log.Printf("playButton: no song selected, skipping")
			return
		}
		now := time.Now()
		done := now.Add(time.Second * time.Duration(ss.Duration_Seconds))
		log.Printf("playButton: starting at: %s. expected song end: %s",
			now.Format(time.Stamp), done.Format(time.Stamp))
		err = play(ss, volumeEffect, killDecoder)
		if err != nil {
			log.Fatalf("playButton: failed to play: %s", err)
		}
		playButton.SetLabel("Pause")
	}

	// Build CUI

	// Play button
	// Can actually use VolumeEffect to control play/pause
	// when you set silent to true it will pause the stream
	playButton = cview.NewButton("Play")
	playButton.SetSelectedFunc(
		func() {
			switch playButton.GetLabel() {
			case "Play":
				songSelected()
			case "Pause":
				killDecoder <- true
				playButton.SetLabel("Play")
			}
		})

	// Song list

	songList = createSongList(songResults, songSelected)

	// Search box
	searchBox = cview.NewInputField()
	searchBox.SetLabel("Search: ")
	searchBox.SetDoneFunc(func(key tcell.Key) {
		if key == tcell.KeyEnter {
			query, err := ytm.Query(searchBox.GetText(), search.Songs)
			if err != nil {
				log.Fatalf("failed to search: %s", err)
			}
			newList := createSongList(query, songSelected)
			app.Lock()
			songFlex.RemoveItem(songList)
			songFlex.AddItem(newList, 0, 6, false)
			songList = newList
			app.Unlock()
			app.Draw()
		}
	})

	// Volume text
	volumeText = cview.NewTextView()
	volumeText.SetText("1.0")
	volumeText.SetBorder(true)
	volumeText.SetMouseCapture(func(action cview.MouseAction, event *tcell.EventMouse) (cview.MouseAction, *tcell.EventMouse) {
		if action == cview.MouseScrollUp {
			volumeEffect.Volume += .1
		} else if action == cview.MouseScrollDown {
			volumeEffect.Volume -= .1
		} else if action == cview.MouseMiddleClick {
			if volumeEffect.Silent {
				volumeEffect.Silent = false
				volumeText.SetTextColor(tcell.ColorDefault)
			} else {
				volumeEffect.Silent = true
				volumeText.SetTextColor(tcell.ColorGray)
			}
		}

		volStr := strconv.FormatFloat(volumeEffect.Volume, 'f', 2, 64)
		volumeText.SetText(volStr)
		app.Draw(volumeText)

		return action, event
	})

	// Progress bar

	progressBar = cview.NewProgressBar()
	progressBar.SetProgress(10)

	// go func() {
	// 	for now := range time.Tick(100 * time.Millisecond) {
	// 		log.Printf("BUCKSHOT %d", now.Unix())
	// 		log.Default()
	// 	}
	// }()

	// Grid

	songFlex = cview.NewFlex()
	songFlex.AddItem(searchBox, 0, 1, false)
	songFlex.AddItem(songList, 0, 6, false)

	controlsFlex = cview.NewFlex()
	controlsFlex.AddItem(playButton, 0, 1, false)
	controlsFlex.AddItem(progressBar, 0, 1, false)
	controlsFlex.AddItem(volumeText, 0, 1, false)

	rootFlex = cview.NewFlex()
	rootFlex.AddItem(songFlex, 0, 1, false)
	rootFlex.AddItem(controlsFlex, 0, 1, false)

	// Keyboard input
	rootFlex.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyEsc {
			app.Stop()
		}
		return event
	})

	frame := cview.NewFrame(rootFlex)
	frame.AddText("Youtube Music CLI", true, cview.AlignCenter, tcell.ColorHotPink)

	app.SetRoot(frame, true)
	app.EnableMouse(true)
	if err := app.Run(); err != nil {
		log.Fatalf("Failed to run app: %s", err)
	}
}

func createSongList(songs []yt.Song,
	selectedFunc func(),
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
		li.SetReference(song)
		li.SetSelectedFunc(selectedFunc)
		songList.AddItem(li)
	}
	return songList
}

// func query(query string, filter search.Filter) []yt.Song {
// 	// Search

// 	// // TODO EMBEDDED PYTHON VERSION
// 	// fs, err := embed_util.NewEmbeddedFiles(data.Data, "ytmusicapi")
// 	// if err != nil {
// 	// 	log.Printf("Failed to new embedded files %s", err)
// 	// }
// 	// ep, err := python.NewEmbeddedPython("ytmusicapi")
// 	// if err != nil {
// 	// 	log.Printf("Failed to created embedded python %s", err)
// 	// }
// 	// ep.AddPythonPath(fs.GetExtractedPath())

// 	cmd := exec.Command("python3", "-c", fmt.Sprintf(
// 		`
// from ytmusicapi import YTMusic
// import sys

// ytmusic = YTMusic('%s', '%s')

// res = ytmusic.search('%s', filter='%s')

// import json

// # https://stackoverflow.com/questions/36021332/how-to-prettyprint-human-readably-print-a-python-dict-in-json-format-double-q
// print(json.dumps(
// 	res,
// 	sort_keys=True,
// 	indent=4,
// 	separators=(',', ': ')
// ))`,
// 		oauthToken, brandId, query, filter))

// 	// pyCmd := ep.PythonCmd

// 	stdout, err := cmd.Output()
// 	if err != nil {
// 		log.Fatalln(errors.Wrap(err, "Failed to run query."))
// 	}

// 	songResults := make([]yt.Song, 50)

// 	err = json.Unmarshal(stdout, &songResults) // https://betterstack.com/community/guides/scaling-go/json-in-go/
// 	if err != nil {
// 		log.Fatalf("Unable to marshal JSON due to %s", err)
// 	}
// 	return songResults
// }

func play(song yt.Song, volume *effects.Volume, kill <-chan bool) error {
	log.Printf("starting download of %s, ID: %s", song.Title, song.VideoId)
	err := yt.DownloadVideo(song.VideoId, cachePath, true)
	if err != nil {
		return fmt.Errorf("play(): %w", err)
	}
	_, streamer, err := loadAudio(filepath.Join(cachePath, song.VideoId+".mp4"), kill)
	if err != nil {
		return fmt.Errorf("play(): %w", err)
	}
	volume.Streamer = streamer
	speaker.Clear()
	speaker.Play(volume)
	return nil
}

func loadAudio(path string, killDecoder <-chan bool) (*reisen.Media, beep.Streamer, error) {
	media, err := reisen.NewMedia(path)
	if err != nil {
		return nil, nil, fmt.Errorf("loadAudio(): Unable to create new media %w", err)
	}

	var sampleSource <-chan [2]float64
	sampleSource, errs, err := readVideoAndAudio(media, killDecoder)
	if err != nil {
		return nil, nil, fmt.Errorf("loadAudio(): %w", err)
	}
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

	// See https://github.com/faiface/beep/wiki/Making-own-streamers
	// for reference.
	// beep.StreamerFunc(func) is a type cast
	streamer := beep.StreamerFunc(func(samples [][2]float64) (n int, ok bool) {
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

	return media, streamer, nil
}

// https://medium.com/@maximgradan/playing-videos-with-golang-83e67447b111
// readVideoAndAudio reads video and audio frames
// from the opened media and sends the decoded
// data to che channels to be played.
func readVideoAndAudio(media *reisen.Media, isRunning <-chan bool) (<-chan [2]float64, chan error, error) {
	sampleBuffer := make(chan [2]float64, sampleBufferSize)
	errs := make(chan error, 50)

	err := media.OpenDecode()
	if err != nil {
		return nil, nil, fmt.Errorf("readVideoAndAudio(): open decode failed: %w", err)
	}

	var audioStream *reisen.AudioStream
	log.Printf("found %d stream(s)", media.StreamCount())
	for _, stream := range media.Streams() {
		if stream.Type() == reisen.StreamAudio {
			audioStream = stream.(*reisen.AudioStream)
			break
		}
	}

	if audioStream == nil {
		return nil, nil, errors.New("readVideoAndAudio(): no audio stream")
	}

	err = audioStream.Open()
	if err != nil {
		return nil, nil, fmt.Errorf("readVideoAndAudio(): failed to open audio stream: %w", err)
	}

	// Clear kill channel
	func() {
	L:
		for {
			select {
			case <-isRunning:
			default:
				break L
			}
		}
	}()

	// Start decoding routine
	go func() {
	Loop:
		for {
			select {
			case <-isRunning:
				break Loop
			default:
			}

			packet, gotPacket, err := media.ReadPacket()
			if err != nil {
				go func(err error) {
					errs <- fmt.Errorf("readVideoAndAudio(): failed to read media packet: %w", err)
				}(err)
			}

			if !gotPacket {
				log.Printf("readVideoAndAudio(): no packet")
				break Loop
			}

			if packet.Type() == reisen.StreamAudio {
				s := media.Streams()[packet.StreamIndex()].(*reisen.AudioStream)

				audioFrame, gotFrame, err := s.ReadAudioFrame()
				if err != nil {
					errs <- fmt.Errorf("readVideoAndAudio(): failed to read audio frame: %w, skipping", err)
					break Loop
				}

				if !gotFrame {
					errs <- fmt.Errorf("readVideoAndAudio(): gotFrame is false. skipping")
					break Loop
				}
				if audioFrame == nil {
					errs <- fmt.Errorf("readVideoAndAudio(): audio frame is nil. skipping")
					break Loop
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
							errs <- fmt.Errorf("readVideoAndAudio(): error reading first channel: %w)", err)
						}(err)
					}

					sample[0] = result

					err = binary.Read(reader, binary.LittleEndian, &result)
					if err != nil {
						go func(err error) {
							errs <- fmt.Errorf("readVideoAndAudio(): error reading second channel: %w)", err)
						}(err)
					}

					sample[1] = result
					sampleBuffer <- sample
				}
			}

		}

		log.Printf("readVideoAndAudio(): decoder goroutine: exiting")
		audioStream.Close()
		media.CloseDecode()
		close(sampleBuffer)
		close(errs)
		speaker.Clear()
	}()

	return sampleBuffer, errs, nil
}
