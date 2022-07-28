package torrentfile

import (
	"net/url"
	"strconv"
)

// prepare the url used to make a request to the announce address for the list of peers
// expects a peerID and a port number, makes a map, then URL encodes it as query params
func (t *torrentFile) buildTrackerURL(peerID [20]byte, port uint16) (string, error) {
	// parse string into *url.URL
	base, err := url.Parse(t.Announce)
	if err != nil {
		return "", err
	}

	// Values is a map from string to string slice (we know because map is inside first bracket
	// and everything that comes after is the type it maps to)
	// the : means grab the whole array, that returns byte slice
	// then we call string() on byte slice to convert to string
	// finally we initialize the slice with one value []string{value goes here}

	// Some info on fields...
	// - peer_id is a 20 byte name to identify ourselves to trackers and peers
	// which is just gonna be random 20 bytes
	// - port is commonly 6881
	// - uploaded/downloaded is total amount of that done so far
	// - left is how many bytes we still have to download in base 10
	// - compact = 1 means to request that the tracker returns compact format
	// which is basically to ignore the peer id? and to use the 4 byte ip + 2 byte port to identify peers instead?
	// https://wiki.theory.org/BitTorrentSpecification

	// the official explanation for the fields are here http://www.bittorrent.org/beps/bep_0003.html
	params := url.Values{
		"info_hash":  []string{string(t.InfoHash[:])},
		"peer_id":    []string{string(peerID[:])},
		"port":       []string{strconv.Itoa(int(Port))},
		"uploaded":   []string{"0"},
		"downloaded": []string{"0"},
		"compact":    []string{"1"},
		"left":       []string{strconv.Itoa(t.Length)},
	}

	// create a raw query from the params
	base.RawQuery = params.Encode()
	// return the full URL request string
	return base.String(), err
}
