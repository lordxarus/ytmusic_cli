package yt

type (
	Song struct {
		Album            Album
		Artists          []Artist
		Category         string
		Duration         string
		Duration_Seconds int
		FeedbackTokens   struct {
			add    string
			remove string
		}
		InLibrary  bool
		IsExplicit bool
		ResultType string
		Thumbnails []Thumbnail
		Title      string
		VideoId    string
		VideoType  string
		Year       int
	}

	Album struct {
		ID   string
		Name string
	}

	Artist struct {
		ID   string
		Name string
	}

	Thumbnail struct {
		height int
		width  int
		url    string
	}
)
