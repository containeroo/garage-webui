package utils

import "net/url"

// HasValidScheme prüft, ob das Schema http oder https ist.
func HasValidScheme(origin *url.URL) bool {
	return origin.Scheme == "http" || origin.Scheme == "https"
}
