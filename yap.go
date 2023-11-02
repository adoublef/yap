package yap

import (
	"errors"
	"slices"

	"github.com/rs/xid"
)

type Yap struct {
	ID      xid.ID
	Content Content
	Region  Region
	Score   int
}

type Content string

func ParseContent(s string) (Content, error) {
	if ok := len(s) <= 240; !ok {
		return Content(""), errors.New("content: value too long")
	}
	// NOTE other parsing can be handled here
	return Content(s), nil
}

// Region must be either 'lhr', 'syd' or 'iad' in correspondence to the deployed regions
type Region string

func ParseRegion(s string) (Region, error) {
	// lhr, syd & iad
	if ok := slices.Contains([]string{"lhr", "syd", "iad"}, s); !ok {
		return "", errors.New("region: invalid region")
	}
	return Region(s), nil
}
