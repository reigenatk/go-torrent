package torrentfile

import (
	"bytes"
	"crypto/sha1"
	"fmt"
	"main/peers"
	"math/rand"
	"net/http"
	"os"
	"time"
	"main/p2p"
	"github.com/jackpal/bencode-go"
)

// Port to listen on
const Port uint16 = 6881

// the third parameters are called struct tags
// https://stackoverflow.com/questions/25497375/what-is-the-third-parameter-of-a-go-struct-field
// the reason we need this is for bencode.Unmarshal- if you read its description it says
// that it uses the struct tags to figure out which fields correspon to what. So for example
// if it says "announce" in the torrent file, it will see that we have 'bencode:"announce"'
// and it will know to map it to the "Announce" field.
type bencodeInfo struct {
	Pieces      string `bencode:"pieces"`
	PieceLength int    `bencode:"piece length"`
	Name        string `bencode:"name"`
	Length      int    `bencode:"length"`
}

type bencodeTorrent struct {
	Announce string      `bencode:"announce"`
	Info     bencodeInfo `bencode:"info"`
}

// the same as the two struct above but in one struct?
type torrentFile struct {
	Announce    string
	InfoHash    [20]byte   // SHA1 hash of the bencodeInfo structure. Used to uniquely identify a torrent file
	PieceHash   [][20]byte // slice (of an byte array of size 20]). Reason is because each SHA-1 hash is 20 bytes or 160 bits
	PieceLength int
	Name        string
	Length      int
}

// unmarshal the .torrent file into our struct of type bencodeTorrent
// it first unmarshalls from the file to bencodeTorrent struct
// then to the flatter torrentFile struct which is nicer to work with in Go
// using our custom function toTorrentFile
func Open(path string) (torrentFile, error) {
	bto := bencodeTorrent{}

	// get io.Reader type for Unmarshal
	read, err := os.Open(path)
	if err != nil {
		return torrentFile{}, err
	}
	defer read.Close()

	// bencode -> structs
	err = bencode.Unmarshal(read, &bto)
	if err != nil {
		return torrentFile{}, err
	}
	return bto.toTorrentFile()
}

// convert from a bencodeTorrent struct to a torrentFile struct
func (bto *bencodeTorrent) toTorrentFile() (torrentFile, error) {
	// get the hash of info dictionary
	infoHash, err := bto.Info.hash()
	if err != nil {
		return torrentFile{}, err
	}

	// convert pieces string to [][20]byte
	pieceSlice, err := bto.Info.makeSlices()

	ret := torrentFile{
		Announce:    bto.Announce,
		InfoHash:    infoHash,
		PieceHash:   pieceSlice,
		PieceLength: bto.Info.PieceLength,
		Name:        bto.Info.Name,
		Length:      bto.Info.Length,
	}

	return ret, nil
}

// transform a string of pieces to [][20]byte, used in toTorrentFile
func (binfo *bencodeInfo) makeSlices() ([][20]byte, error) {
	hashLen := 20
	// convert string to one big []byte
	buf := []byte(binfo.Pieces)

	// check that it is multiple of 20
	if len(buf)%20 != 0 {
		err := fmt.Errorf("The number of bytes in pieces is not divisible by 20, length is %d", len(buf))
		return [][20]byte{}, err
	}
	// calculate how many hashes we have now that we know it divides 20
	numHashes := len(buf) / 20

	// dynamically makes "numHashes" arrays of type [20]byte
	ret := make([][20]byte, numHashes)
	for i := 0; i < numHashes; i++ {
		// and copy into the slice, each [20]byte
		copy(ret[i][:], buf[i*hashLen:(i+1)*hashLen])
	}

	return ret, nil

}

// hash a bencodeInfo struct to uniquely ID a torrentfile, used in toTorrentFile
func (binfo *bencodeInfo) hash() ([20]byte, error) {
	// bencodeInfo struct -> bencode
	var becodeInfoBencoded bytes.Buffer

	// just a go sidenote, Marshal expects something of type io.Writer as first arg
	// this is actually an interface, and bytes.Buffer implements it since it has a
	// function with signature "Write(p []byte) (n int, err error)" therefore this is valid
	// syntax!
	err := bencode.Marshal(&becodeInfoBencoded, *binfo)

	if err != nil {
		return [20]byte{}, err
	}
	// get the SHA1 hash, using sha1 library
	bencodeInfoHashed := sha1.Sum(becodeInfoBencoded.Bytes())

	return bencodeInfoHashed, nil
}

// this function make the first request to tracker and parses the response into
// a slice of Peer objects for returning
func (tf *torrentFile) requestPeers(peerID [20]byte, Port uint16) ([]peers.Peer, error) {

	// get final URL with query params already in
	requestURL, err := tf.buildTrackerURL(peerID, Port)
	if err != nil {
		return nil, err
	}

	// setup HTTP stuff
	httpClient := &http.Client{Timeout: 15 * time.Second}
	response, err := httpClient.Get(requestURL)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	// parse http response into struct
	responseStruct := peers.PeersResponse{}
	err = bencode.Unmarshal(response.Body, responseStruct)
	if err != nil {
		return nil, err
	}

	// parse the string of peers into []Peer
	return peers.Unmarshal(responseStruct.Peers)
}

// this function gets called from main, and calls a bunch of sub functions
func (tf *torrentFile) DownloadToFile(path string) error {
	// create peer ID
	var peerID [20]byte
	_, err := rand.Read(peerID[:])
	if err != nil {
		return err
	}

	// make URL request based on info in the torrent file
	// peersArray holds the IP/port of all the peers we need to connect to!
	peersArray, err := tf.requestPeers(peerID, Port)

	// store it in a Torrent struct
	torrent := p2p.Torrent {
		Peers: peersArray,
		PeerID: peerID,
		InfoHash: tf.InfoHash,
		PieceHash: tf.PieceHash,
		PieceLength: tf.PieceLength,
		Length: tf.Length,
		Name: tf.Name,
	}

	buf, err := torrent.Download()
	if err != nil {
		return err
	}

}
