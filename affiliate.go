package main

import (
	"fmt"
	"net/url"
	"os"
)

// affiliateTag returns the configured affiliate partner tag.
func affiliateTag() string {
	return os.Getenv("AFFILIATE_PARTNER_TAG")
}

// tagURL appends an affiliate parameter to a deal URL based on the vendor.
// Returns the original URL unchanged if no tag is configured or the vendor is unrecognized.
func tagURL(rawURL, vendor string) string {
	tag := affiliateTag()
	if tag == "" {
		return rawURL
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}

	q := u.Query()
	switch vendor {
	case "humble":
		q.Set("partner", tag)
	case "fanatical":
		q.Set("aff", tag)
	case "gmg": // Green Man Gaming
		q.Set("mst", tag)
	case "steam":
		// Steam's affiliate program is widget-based; no query param available.
		return rawURL
	default:
		q.Set("ref", tag)
	}

	u.RawQuery = q.Encode()
	return u.String()
}

// itadSearchURL returns an IsThereAnyDeal search URL for a game title with affiliate tag.
func itadSearchURL(title string) string {
	base := fmt.Sprintf("https://isthereanydeal.com/search/?q=%s", url.QueryEscape(title))
	return tagURL(base, "itad")
}
