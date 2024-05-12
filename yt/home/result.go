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

type Result struct {
	VideoId  string `json:"videoId"`
	Title    string `json:"title"`
	Year     string `json:"year"`
	BrowseId string `json:"browseId"`
	Type     string `json:"type"`
}

func (s *Result) GetYear() string {
	return s.Year
}

func (s *Result) GetBrowseId() string {
	return s.BrowseId
}

// func (s *SongResult) Type() ResultType {
// 	return s.ResultData.Type
// }

type ResultList struct {
	Title    string
	Contents []Result
}
type Results []ResultList

// 	Result struct {
// 		Title string
// 		// I don't really know why this is called year, it seems to be used for displaying one of the artists
// 		Year       string   `json:"year"`
// 		BrowseId   string   `json:"browseId"`
// 		Thumbnails []string `json:"thumbnails"`
// 	}
// )
