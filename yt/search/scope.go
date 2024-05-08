package search

type Scope int

const (
	LibraryScope Scope = iota
	UploadScope
)

func (s Scope) String() string {
	switch s {
	case LibraryScope:
		return "library"
	case UploadScope:
		return "uploads"
	}
	return ""
}
