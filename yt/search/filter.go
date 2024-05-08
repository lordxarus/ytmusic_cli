package search

type Filter int

const (
	Songs Filter = iota
	Videos
	Albums
	Artists
	Playlists
	CommunityPlaylists
	FeaturedPlaylists
	Uploads
)

func (f Filter) String() string {
	switch f {
	case Songs:
		return "songs"

	case Videos:
		return "videos"

	case Artists:
		return "artists"

	case Playlists:
		return "playlists"

	case CommunityPlaylists:
		return "community_playlists"

	case FeaturedPlaylists:
		return "featured_playlists"

	case Uploads:
		return "uploads"

	}
	return ""
}
