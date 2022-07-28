package p2p

import (
	"fmt"
	"io"
	"log"
	"main/client"
	"main/message"
	"main/peers"
	"time"
)

const NormalBlockSize int = 16384  // 2^14 aka 16KB
const NormalPieceSize int = 262144 // 2^18 aka 256KB

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

// a struct to represent all the info we need about a piece that is in need of download
type PieceWork struct {
	// the index of the piece in terms of the whole file
	Index int

	// the length of the piece in bytes
	Length int

	// the SHA1 hash
	PieceHash [20]byte
}

// a struct to represent the result of a piece transfer
// the index of the piece and the actual contents of the piece
type pieceResult struct {
	index    int
	contents []byte
}

// a strict to keep track of the progress for a specific peer connection
type PieceProgress struct {
	Index  int
	Client *client.Client
	// the actual contents of the piece
	PieceContents []byte
	Downloaded    int
	// how many bytes have been requested. We need this to know which offset into the piece
	// we need to start our next block request at
	Requested int
	Backlog   int
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
			Index:     idx,
			Length:    t.PieceLength,
			PieceHash: pieceHash,
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
		if peerClient.Bitfield.HasPiece(pieceToGet.Index) == false {
			// if it doesnt have this piece, then put this piece back into the queue
			// for another peer to get and move on to another one
			workqueue <- pieceToGet
			continue
		}

		// by this point we know that the peer has this piece
		// so try to download it
		tryDownloadBlock(peerClient, pieceToGet)
	}
}

func tryDownloadBlock(client *client.Client, piece *PieceWork) error {
	progress := PieceProgress{
		Index:         piece.Index,
		Client:        client,
		PieceContents: make([]byte, piece.Length),
	}

	// Setting a deadline helps get unresponsive peers unstuck.
	// 30 seconds is more than enough time to download a 262 KB piece
	client.Conn.SetDeadline(time.Now().Add(30 * time.Second))
	defer client.Conn.SetDeadline(time.Time{}) // Disable the deadline

	// first, send the request for the block
	err := client.SendRequest(piece.Index, progress.Requested, NormalBlockSize)
	if err != nil {
		return err
	}

	// now try to read it
	err = progress.tryReadBlock()
	if err != nil {
		return err
	}
}

// we define this on PieceProgress because we wanna directly change both the requested
// and
func (p *PieceProgress) tryReadBlock() error {
	// read the response from peer (should be exact same as the one we sent to peer)
	// using io.ReadFull, not io.ReadAll since that doesnt give us control over length
	// also IDK why we cant just read the whole response in at once but OK
	firstByte := make([]byte, 1)
	_, err := io.ReadFull(p.Client.Conn, firstByte)
	if err != nil {
		return err
	}
	responseLen := int(firstByte[0])

	// read in the rest of the peer handshake response
	restOfResponse := make([]byte, responseLen)
	_, err = io.ReadFull(p.Client.Conn, restOfResponse)
	if err != nil {
		return err
	}
	// check that the id = 7 for piece
	if restOfResponse[0] != message.Piece {
		err := fmt.Errorf("Expected Piece message, got message with id %d instead", restOfResponse[0])
		return err
	}

}
