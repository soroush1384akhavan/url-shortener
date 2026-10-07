package link

// for fixig import cycle problem
type Store interface {
	FindByURL(string) (*ShortLink, bool)
	FindByCode(string) (*ShortLink, bool)
	SaveIfNotExist(*ShortLink) (*ShortLink, error)
	Save(*ShortLink) (error)
}
	