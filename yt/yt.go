package yt

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/kkdai/youtube/v2"
	"github.com/lordxarus/ytmusic_cli/yt/home"
	"github.com/lordxarus/ytmusic_cli/yt/search"
	"github.com/sanity-io/litter"
)

var ErrAlreadyDownloaded = errors.New("found in cache")

type YTMClient struct {
	oauthToken string
	brandId    string
	cachePath  string
}

func New(token string, id string, cachePath string) (*YTMClient, error) {
	client := &YTMClient{
		token,
		id,
		cachePath,
	}
	_, err := client.Home()
	if err != nil {
		return nil, fmt.Errorf("New() sanity check failed: %w", err)
	}
	return client, nil
}

func (ytm *YTMClient) DownloadVideo(videoId string) error {
	var fullPath string
	if ytm.cachePath != "" {
		fullPath = filepath.Join(ytm.cachePath, videoId+".mp4")
		_, err := os.Open(fullPath)
		if err == nil {
			log.Println("DownloadVideo(): already downloaded", videoId)
			return ErrAlreadyDownloaded
		}
	}

	client := youtube.Client{}

	video, err := client.GetVideo(videoId)
	if err != nil {
		return fmt.Errorf("DownloadVideo(): failed to get video: %w", err)
	}

	formats := video.Formats.WithAudioChannels() // only get videos with audio

	stream, _, err := client.GetStream(video, &formats[0])
	if err != nil {
		return fmt.Errorf("DownloadVideo(): failed to get stream: %w", err)
	}
	defer stream.Close()

	file, err := os.Create(fullPath)
	if err != nil {
		return fmt.Errorf("DownloadVideo(): failed to create file, %s for %s: %w", fullPath, videoId, err)
	}
	defer file.Close()

	_, err = io.Copy(file, stream)
	if err != nil {
		return fmt.Errorf("DownloadVideo(): failed to copy stream to file, %s: %w", fullPath, err)
	}

	return nil
}

func (ytm *YTMClient) GetSong(videoId string) (map[string]any, error) {
	song := make(map[string]any)
	var returnErr error
	result, err := ytm.runPyScript(fmt.Sprintf("ytmusic.get_song('%s')", videoId))
	if err != nil {
		return nil, fmt.Errorf("GetSong() failed getting song: %w", err)
	}

	err = json.Unmarshal([]byte(result), &song)
	if err != nil {
		return nil, fmt.Errorf("GetSong() unable to unmarshal JSON")
	}
	return song, returnErr
}

func (ytm *YTMClient) Home() (home.Results, error) {
	results := make(home.Results, 3)
	var returnErr error

	result, err := ytm.runPyScript("ytmusic.get_home()")
	if err != nil {
		return nil, fmt.Errorf("Home() failed getting home results: %w", err)
	}

	err = json.Unmarshal([]byte(result), &results) // https://betterstack.com/community/guides/scaling-go/json-in-go/
	if err != nil {
		returnErr = fmt.Errorf("Home() unable to marshal JSON", err)
	}

	return results, returnErr
}

func (ytm *YTMClient) Query(query string, filter search.Filter) ([]Song, error) {
	var returnErr error

	result, err := ytm.runPyScript(fmt.Sprintf("ytmusic.search('%s', filter='%s')", query, filter))
	if err != nil {
		return nil, err
	}

	songResults := make([]Song, 50)

	err = json.Unmarshal([]byte(result), &songResults) // https://betterstack.com/community/guides/scaling-go/json-in-go/
	if err != nil {
		returnErr = fmt.Errorf("Query(): unable to marshal JSON: %w", err)
	}
	return songResults, returnErr
}

func (ytm *YTMClient) AddToHistory(videoId string) error {
	res, err := ytm.runPyScript(fmt.Sprintf("ytmusic.add_to_history('%s')", videoId))
	if err != nil {
		return fmt.Errorf("AddToHistory(): failed to add to history: %w", err)
	}
	log.Printf("AddToHistory(): %s", litter.Sdump(res))
	return nil
}

func (ytm *YTMClient) runPyScript(script string) (string, error) {
	var returnErr error
	if ytm.oauthToken == "" {
		return "", errors.New("no OAuth token provided. can't run python script")
	}
	// Search

	// // TODO EMBEDDED PYTHON VERSION
	// fs, err := embed_util.NewEmbeddedFiles(data.Data, "ytmusicapi")
	// if err != nil {
	// 	log.Printf("Failed to new embedded files %s", err)
	// }
	// ep, err := python.NewEmbeddedPython("ytmusicapi")
	// if err != nil {
	// 	log.Printf("Failed to created embedded python %s", err)
	// }
	// ep.AddPythonPath(fs.GetExtractedPath())

	cmd := exec.Command("python3", "-c", fmt.Sprintf(`
from ytmusicapi import YTMusic
import sys
	
ytmusic = YTMusic('%s', '%s')
	
res = %s
	
import json
			
# https://stackoverflow.com/questions/36021332/how-to-prettyprint-human-readably-print-a-python-dict-in-json-format-double-q
print(json.dumps(
	res,
	sort_keys=True,
	indent=4,
	separators=(',', ': ')
))`, ytm.oauthToken, ytm.brandId, script))

	stdout, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		ok := errors.As(err, &exitErr)
		if ok {
			returnErr = fmt.Errorf("runPyScript(): failed to run python script, stderr: %s, %w", exitErr.Stderr, err)
		} else {
			returnErr = fmt.Errorf("runPyScipt(): failed to run python script, err is not an ExitError?? %w", err)
		}
	}

	return string(stdout), returnErr
}

// Example usage for playlists: downloading and checking information.
func examplePlaylist() {
	playlistID := "PLQZgI7en5XEgM0L1_ZcKmEzxW1sCOVZwP"
	client := youtube.Client{}

	playlist, err := client.GetPlaylist(playlistID)
	if err != nil {
		panic(err)
	}

	/* ----- Enumerating playlist videos ----- */
	header := fmt.Sprintf("Playlist %s by %s", playlist.Title, playlist.Author)
	println(header)
	println(strings.Repeat("=", len(header)) + "\n")

	for k, v := range playlist.Videos {
		fmt.Printf("(%d) %s - '%s'\n", k+1, v.Author, v.Title)
	}

	/* ----- Downloading the 1st video ----- */
	entry := playlist.Videos[0]
	video, err := client.VideoFromPlaylistEntry(entry)
	if err != nil {
		panic(err)
	}
	// Now it's fully loaded.

	fmt.Printf("Downloading %s by '%s'!\n", video.Title, video.Author)

	stream, _, err := client.GetStream(video, &video.Formats[0])
	if err != nil {
		panic(err)
	}

	file, err := os.Create("video.mp4")
	if err != nil {
		panic(err)
	}

	defer file.Close()
	_, err = io.Copy(file, stream)
	if err != nil {
		panic(err)
	}

	println("Downloaded /video.mp4")
}
