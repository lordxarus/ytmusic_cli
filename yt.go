package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"github.com/kkdai/youtube/v2"
)

func DownloadVideo(videoId string, cacheDir string, useCache bool) {
	fullPath := cacheDir + "/" + videoId + ".mp4"
	if useCache {
		_, err := os.Open(fullPath)
		if err == nil {
			return
		}
	}
	client := youtube.Client{}

	video, err := client.GetVideo(videoId)
	if err != nil {
		panic(err)
	}

	formats := video.Formats.WithAudioChannels() // only get videos with audio
	stream, _, err := client.GetStream(video, &formats[0])
	if err != nil {
		panic(err)
	}
	defer stream.Close()

	file, err := os.Create(fullPath)
	if err != nil {
		log.Fatalf("Couldn't cache %s. Err: %s", fullPath, err)
	}
	defer file.Close()

	_, err = io.Copy(file, stream)
	if err != nil {
		panic(err)
	}
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
