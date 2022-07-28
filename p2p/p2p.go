package p2p

import (
	"fmt"
	"log"
	"main/client"
	"main/peers"
)

// this struct is more or less the same as torrentFile
// but with the additional info of peers and peerID
type Torrent struct {
	Peers       []peers.Peer
	PeerID      [20]byte
	InfoHash    [20]byte
	PieceHash   [][20]byte
	PieceLength int
	Name        string
	Length      int
}

// a struct to represent all the info we need about a piece
type PieceWork struct {
	index     int
	length    int
	pieceHash [20]byte
}

// a struct to represent the result of a piece transfer
// the index of the piece and the actual contents of the piece
type pieceResult struct {
	index    int
	contents []byte
}

// perform the handshake
func (t *Torrent) Download() ([]byte, error) {
	log.Println("Starting torrent for %s", t.Name)

	// make a channel with a buffer length of the # of pieces we need to download
	// and which passes thru the channel, values of type pieceWork and pieceResult respectively
	workQueue := make(chan *PieceWork, len(t.PieceHash))
	results := make(chan *pieceResult)

	// for each piece we need to download...
	for idx, pieceHash := range t.PieceHash {
		// make new pieceWork struct and put into the workQueue channel
		newWork := PieceWork{
			index:     idx,
			length:    t.PieceLength,
			pieceHash: pieceHash,
		}
		workQueue <- &newWork
	}

	// start workers for each of the # of peers available to us
	// this number doesn't need to equal the # of pieces necessarily. It's not
	// one worker per piece, in fact, bittorrent caps # of peers at 30 usually
	// so one peer will give you many pieces

	numPieces := len(t.PieceHash)
	for _, peer := range t.Peers {
		go t.startPeer(peer, workQueue, results, numPieces)
	}
}

// this function operates on ONE peer and will be invoked many times using goroutines
func (t *Torrent) startPeer(p peers.Peer, workqueue chan *PieceWork, results chan *pieceResult, numPieces int) {

	// create client struct for this specific peer
	// this actually goes ahead and makes the TCP connection to the peer
	peerClient, err := client.New(p, t.PeerID, t.InfoHash, numPieces)
	if err != nil {
		log.Printf("Could not handshake and/or get bitfield for peer %s", p.String())
		return
	}
	// close the connection eventually
	defer peerClient.Conn.Close()
	fmt.Println("Handshake and bitfield received for peer %s successfully", p.String())

	// send unchoke and interested message to this peer
	peerClient.UnchokePeer()
	peerClient.SendInterestedPeer()

	// decide which pieces from this peer we want to download
	// we're gonna use a range loop over the channel (which only works if there is
	// still something in the channel) to repeatedly get piece requests
	// which we filled in Download(), and try to use this peer to get said piece

	// oh yeah important detail, RANGE LOOPS OVER CHANNEL ACTUALLY GRABS THE OBJECT
	// FROM THE CHANNEL. So here 'pieceToGet' is taken from the channel, that is,
	// its equivalent to 'pieceToGet := <- workqueue', just infinitely of course (until
	// the channel is empty)
	for pieceToGet := range workqueue {

		// check if our peer has this piece
		if peerClient.Bitfield.HasPiece(pieceToGet.index) == false {
			// if it doesnt have this piece, then put this piece back into the queue
			// for another peer to get and move on to another one
			workqueue <- pieceToGet
			continue
		}

		// by this point we know that the peer has this piece
		// so try to download it
		tryDownloadPiece(peerClient, pieceToGet)
	}
}

func tryDownloadPiece(client *client.Client, piece *PieceWork) error {

}
