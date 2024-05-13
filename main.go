package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
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

func init() {
	var ok bool
	var err error

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
		log.Fatalf("init() couldn't find home directory")
	}

	cachePath = homePath + "/.cache/ytmusic_cli"

	if wdPath, err = os.Getwd(); err != nil {
		log.Fatalf("init() couldn't find working directory: %s", err)
	}

	log.Printf("init(): home: %s cache: %s wd: %s", homePath, cachePath, wdPath)

	if err = os.MkdirAll(cachePath, 0o750); err != nil {
		log.Fatalf("init() couldn't create cache directory: %s", err)
	}
}

// TODO We can rewrite this to only use beep
// TODO Beep supports playing and pausing
// TODO I should probably create my own types for use in my own internal stuff.
// I currently have the Song result that you get from the search end point implemented.
// But when I go to, for example, implement the Song result type that you would get from
// the home screen it will be a completely different Song type. So I need to namespace them
// similarly to ytmusicapi. Based on which endpoint they come from. yt.search.Song and yt.home.Song or something.
// And then I can have my own Song type for my GUI which will include only what I need.
// TODO Use ffplay instead of beep
// TODO Handling of a song's completion
func main() {
	// Flex boxes
	var rootFlex *cview.Flex
	var mainFlex *cview.Flex
	var navFlex *cview.Flex
	var controlsFlex *cview.Flex

	// Search field
	var searchField *cview.InputField

	// Play button
	var playButton *cview.Button
	pauseLabel := "⏸️"
	playLabel := "▶️"

	// Song list
	var songList *cview.List
	var songResults []yt.Song

	// Progress bar
	var progressBar *cview.ProgressBar
	var progressBarRunner *tickerBar

	// Volume bar
	var volumeBar *cview.ProgressBar
	var volumeEffect *effects.Volume = &effects.Volume{
		Streamer: nil,
		Base:     2,
		Volume:   -1.0,
		Silent:   false,
	}

	// Create YTM client
	ytm, err := yt.New(oauthToken, brandId, cachePath)
	if err != nil {
		log.Fatalf("main() failed to create ytm client: %s", err)
	}
	// Init speaker
	err = speaker.Init(sampleRate, SpeakerSampleRate.N(time.Second/10))
	if err != nil {
		log.Fatalf("main() unable to init speaker: %s", err)
	}

	query := "Best Classical Music"

	songResults, err = ytm.Query(query, search.Songs)
	if err != nil {
		log.Fatalf("main() initial query failed: %s", err)
	}

	killDecoder := make(chan bool, 10)

	err = ytm.DownloadVideo(songResults[0].VideoId)
	if err != nil {
		log.Fatalf("main() failed to download video: %s", err)
	}

	// Called when a song is selected on the songList or when play is pressed
	playSong := func() {
		progressBarRunner.stop()
		// *selectedSong = song
		song, ok := songList.GetCurrentItem().GetReference().(yt.Song)
		if !ok {
			log.Printf("playSong(): no song selected, skipping")
			return
		}

		// debug logging
		now := time.Now()
		done := now.Add(time.Second * time.Duration(song.Duration_Seconds))
		log.Printf("playButton: starting at: %s. expected song end: %s",
			now.Format(time.Stamp), done.Format(time.Stamp))

		// play song
		go func(callback func()) {
			err = play(song, volumeEffect, killDecoder)
			if err != nil {
				log.Fatalf("playSong(): failed to play: %s", err)
			}
			callback()
		}(func() {
			progressBarRunner.start(time.Second * time.Duration(song.Duration_Seconds))
			playButton.SetLabel(pauseLabel)
			app.Draw(playButton)
		})
	}

	// Build CUI

	// Play button
	// Can use VolumeEffect.Silent to play/pause
	playButton = cview.NewButton(playLabel)
	playButton.SetCursorRune(rune(0))
	playButton.SetPadding(0, 1, 0, 0)
	playButton.SetSelectedFunc(
		func() {
			switch playButton.GetLabel() {
			// We are paused
			case playLabel:
				playSong()
			// We are playing
			case pauseLabel:
				progressBarRunner.stop()
				killDecoder <- true
				// TODO Maybe VolumeEffect.Silent
				// should be the play/pause state. Because if a user mutes the volume
				// it will as of now pause anyway which isn't really the behavior I want
				// out of a mute button. Might as well just use VolumeEffect.Silent instead of playButton.GetLabel()
				// Remove the middle mouse handler code and add it here instead
				// need to read up on concurrency in go more
				// Although as I think about it, pausing on mute isn't a bad feature at all
				// https://golang.org/doc/effective_go.html#concurrency
				// volumeEffect.Silent = true
				playButton.SetLabel(playLabel)
			}
		})

	// Song list
	songList = createSongList(songResults, playSong)

	// Search box
	searchField = cview.NewInputField()
	searchField.SetLabel("Search: ")
	searchField.SetBorder(true)
	searchField.SetDoneFunc(func(key tcell.Key) {
		if key == tcell.KeyEnter {
			query, err := ytm.Query(searchField.GetText(), search.Songs)
			if err != nil {
				log.Fatalf("failed to search: %s", err)
			}
			newList := createSongList(query, playSong)
			app.Lock()
			mainFlex.RemoveItem(songList)
			mainFlex.AddItem(newList, 0, 6, false)
			songList = newList
			app.Unlock()
			app.Draw()

		}
	})

	// Volume bar
	volumeBar = cview.NewProgressBar()
	{
		maxVol := 1.0
		minVol := -5.0
		volStep := 0.1
		volStepBig := 0.25

		displayVolume := func() int { return int(100 * (math.Abs(minVol) + volumeEffect.Volume) / (maxVol + math.Abs(minVol))) }

		volumeBar.SetTitle(strconv.Itoa(displayVolume()))
		volumeBar.SetProgress(30)
		volumeBar.SetBorder(true)
		volumeBar.SetFilledColor(tcell.ColorGreen)

		// Mouse input
		volumeBar.SetMouseCapture(func(action cview.MouseAction, event *tcell.EventMouse) (cview.MouseAction, *tcell.EventMouse) {
			doStep := volStep
			vol := &volumeEffect.Volume
			if (event.Modifiers() & tcell.ModShift) != 0 {
				println("shift")
				doStep = volStepBig
			}

			switch action {
			case cview.MouseScrollUp:
				if *vol+doStep >= maxVol {
					*vol = maxVol
				} else {
					*vol += doStep
				}
			case cview.MouseScrollDown:
				// TODO This is a bit of a hack.
				// A better solution would be a function that increases
				// the step size based on how close it is to minVol. Calculus?
				// Instead of doing that we just double step size after arbitrary
				// point. Feels similar to me.
				if *vol <= -2.5 {
					doStep *= 2
				}
				if *vol-doStep <= minVol {
					*vol = minVol
				} else {
					*vol -= doStep
				}
			case cview.MouseMiddleClick:
				if volumeEffect.Silent {
					volumeEffect.Silent = false
					volumeBar.SetTitleColor(tcell.ColorDefault)
				} else {
					volumeEffect.Silent = true
					volumeBar.SetTitleColor(tcell.ColorGray)
				}

			}

			// TODO What happens on a 32 bit system with FormatFloat's bitness arg?
			// volStr := strconv.FormatFloat(volumeEffect.Volume, 'f', 2, 64)
			// volumeBar.SetTitle(volStr)

			volumeBar.SetProgress(displayVolume())
			volumeBar.SetTitle(strconv.Itoa(displayVolume()))

			app.QueueUpdateDraw(func() {}, volumeBar)

			return action, event
		})
	}

	// Progress bar

	progressBar = cview.NewProgressBar()
	progressBar.SetBorder(true)

	progressBarRunner = newTickerBar(app, progressBar)

	// // TODO songFlex is probably better named "contentFlex"
	// or it will be when I have other things to populate it with
	// I don't know how the page system works in cview though
	mainFlex = cview.NewFlex()
	mainFlex.SetBorder(true)
	mainFlex.AddItem(songList, 0, 3, false)

	navFlex = cview.NewFlex()
	navFlex.AddItem(searchField, 0, 1, true)

	controlsFlex = cview.NewFlex()
	// This fixedSize number is either rows or colums based on the direction of the flex, default is cols
	controlsFlex.AddItem(playButton, 0, 1, false)
	controlsFlex.AddItem(progressBar, 0, 5, false)
	controlsFlex.AddItem(volumeBar, 0, 2, false)
	controlsFlex.SetBorder(true)

	rootFlex = cview.NewFlex()
	rootFlex.SetDirection(cview.FlexRow)
	rootFlex.AddItem(navFlex, 0, 1, false)
	rootFlex.AddItem(mainFlex, 0, 8, false)
	rootFlex.AddItem(controlsFlex, 0, 1, false)

	// Keyboard input
	rootFlex.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyCtrlC, tcell.KeyEsc:
			app.Stop()
		}

		switch event.Rune() {
		case 'q':
			app.Stop()
		}

		return event
	})

	frame := cview.NewFrame(rootFlex)
	frame.AddText("Youtube Music CLI", true, cview.AlignCenter, tcell.ColorDarkGreen)

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

func play(song yt.Song, volume *effects.Volume, kill <-chan bool) error {
	log.Printf("starting download of %s, ID: %s", song.Title, song.VideoId)
	err := ytm.DownloadVideo(song.VideoId)
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
// data to the channels to be played.
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
