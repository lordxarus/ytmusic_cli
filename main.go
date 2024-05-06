package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/faiface/beep"
	"github.com/faiface/beep/effects"
	"github.com/faiface/beep/speaker"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/zergon321/reisen"
)

var (
	isPlaying = false
	// Keep paths clean with no trailing slash
	// TODO Could use filepath.Clean()
	homePath  string
	cachePath string
	wdPath    string
)

const (
	sampleRate                        = 44100
	channelCount                      = 2
	bitDepth                          = 8
	sampleBufferSize                  = 32 * channelCount * bitDepth * 1024
	SpeakerSampleRate beep.SampleRate = 44100
)

// TODO Can use struct tags here? https://www.digitalocean.com/community/tutorials/how-to-use-struct-tags-in-go

func main() {
	var ok bool
	var err error

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

	songResults := search("Little Big")

	DownloadVideo(songResults[0].VideoId, cachePath, true)

	var selectedSong Song

	if len(songResults) > 0 {
		selectedSong = songResults[0]
	}

	var killDecoder chan<- bool = make(chan<- bool, 1)

	app := tview.NewApplication()
	var playButton *tview.Button

	songList := tview.NewList()

	playLabel := func() string {
		if isPlaying {
			return "Pause"
		} else {
			return "Play"
		}
	}

	// Add songs
	for _, song := range songResults {
		// Collect the artist names for this song
		var artistNames []string
		for _, artist := range song.Artists {
			artistNames = append(artistNames, artist.Name)
		}
		songList.AddItem(song.Title, strings.Join(artistNames, ", "), '•', func() {
			selectedSong = song
			killDecoder <- true
			killDecoder = play(song)
			isPlaying = true
			playButton.SetLabel(playLabel())
		})
	}

	playButton = tview.NewButton("Play").SetSelectedFunc(
		func() {
			log.Println("playButton:", isPlaying)
			if isPlaying {
				killDecoder <- true
				speaker.Clear()
				isPlaying = false
			} else {
				isPlaying = true
				killDecoder = play(selectedSong)
			}
			playButton.SetLabel(playLabel())
		})

	flex := tview.NewGrid().
		AddItem(songList, 0, 0, 3, 2, 0, 0, true).
		AddItem(playButton, 5, 0, 2, 2, 0, 0, true)

	flex.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyEsc {
			app.Stop()
		}
		return event
	})

	frame := tview.NewFrame(flex).AddText("Youtube Music CLI", true, tview.AlignCenter, tcell.ColorHotPink)

	if err := app.SetRoot(frame, true).EnableMouse(true).Run(); err != nil {
		log.Fatalf("Couldn't set root %s", err)
	}
}

func search(query string) []Song {
	// Search
	cmd := exec.Command(
		"python3.11",
		fmt.Sprintf("%s/ytmusiccli.py", wdPath),
		"'"+query+"'",
	)

	stdout, err := cmd.Output()
	if err != nil {
		log.Fatalf("Err: '%s' while running command: %s", err.(*exec.ExitError).Stderr, cmd.String())
	}

	songResults := make([]Song, 200)

	err = json.Unmarshal(stdout, &songResults) // https://betterstack.com/community/guides/scaling-go/json-in-go/
	if err != nil {
		log.Fatalf("Unable to marshal JSON due to %s", err)
	}
	return songResults
}

func play(song Song) chan<- bool {
	log.Printf("Starting download of %s, ID: %s", song.Title, song.VideoId)
	DownloadVideo(song.VideoId, cachePath, true)
	log.Println("Download complete.")
	_, streamer, killDecoder := loadAudio(filepath.Join(cachePath, song.VideoId+".mp4"))
	speaker.Clear()
	vol := effects.Volume{
		Streamer: streamer,
		Base:     2,
		Volume:   -10,
		Silent:   false,
	}
	speaker.Play(vol.Streamer)
	return killDecoder
}

func loadAudio(path string) (*reisen.Media, beep.Streamer, chan<- bool) {
	media, err := reisen.NewMedia(path)
	if err != nil {
		log.Fatalf("Unable to create new media %s", err)
	}

	var sampleSource <-chan [2]float64
	sampleSource, killDecoder, _, err := readVideoAndAudio(media)
	// go func(errs chan error) {
	// 	for {
	// 		err := <-errs
	// 		if err != nil {
	// 			log.Println(err)
	// 		}
	// 	}
	// }(errs)
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
func readVideoAndAudio(media *reisen.Media) (<-chan [2]float64, chan<- bool, chan error, error) {
	sampleBuffer := make(chan [2]float64, sampleBufferSize)
	errs := make(chan error)
	killDecoder := make(chan bool, 2)

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
