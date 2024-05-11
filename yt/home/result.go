package home

// type ResultType int

// const (
// 	Song ResultType = iota
// 	Album
// 	Playlist
// 	Artist
// 	Video
// )

// func (r ResultType) String() string {
// 	switch r {
// 	case Song:
// 		return "Song"
// 	case Album:
// 		return "Album"
// 	case Playlist:
// 		return "Playlist"
// 	case Artist:
// 		return "Artist"
// 	case Video:
// 		return "Video"
// 	default:
// 		return "Unknown"
// 	}
// }

// func (r ResultType) UnmarshalJSON(b []byte) error {

// }

type (
	CommonResult interface {
		Year() string
		BrowseId() string
		// Type() ResultType
	}

	ResultData struct {
		Year     string `json:"year"`
		BrowseId string `json:"browseId"`
		Type     string `json:"type"`
	}
)

type SongResult struct {
	ResultData
}

func (s *SongResult) Year() string {
	return s.ResultData.Year
}

func (s *SongResult) BrowseId() string {
	return s.ResultData.BrowseId
}

// func (s *SongResult) Type() ResultType {
// 	return s.ResultData.Type
// }

type (
	HomeResults struct {
		Content []CommonResult
		Title   string
	}
)

// 	Result struct {
// 		Title string
// 		// I don't really know why this is called year, it seems to be used for displaying one of the artists
// 		Year       string   `json:"year"`
// 		BrowseId   string   `json:"browseId"`
// 		Thumbnails []string `json:"thumbnails"`
// 	}
// )
