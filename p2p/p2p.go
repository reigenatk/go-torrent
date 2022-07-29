package p2p

import (
	"bytes"
	"crypto/sha1"
	"fmt"
	"log"
	"main/client"
	"main/message"
	"main/peers"
	"time"
)

const NormalBlockSize int = 16384  // 2^14 aka 16KB
const NormalPieceSize int = 262144 // 2^18 aka 256KB

// MaxBacklog is the number of unfulfilled requests a client can have in its pipeline
const MaxBacklog = 5

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
	// which piece in the entire file is this?
	Index int
	// the client has the connection object, among other things
	Client *client.Client
	// the actual contents of the piece
	PieceContents []byte
	// how many bytes have been actually received from the peer
	Downloaded int
	// how many bytes have been requested. We need this to know which offset into the piece
	// we need to start our next block request at
	Requested int
	// how many requests are currently sent and haven't been responded to
	Backlog int
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
	// you can kinda think of this as a thread pool in C++

	// oh yeah important detail, RANGE LOOPS OVER CHANNEL ACTUALLY GRABS THE OBJECT
	// FROM THE CHANNEL. So here 'pieceToGet' is taken from the channel, that is,
	// its equivalent to 'pieceToGet := <- workqueue', therefore if somthing goes wrong
	// and you wanna abort, you should PUT IT BACK IN THE CHANNEL so that it can be
	// picked up by another worker (in this case, for example, if a peer gives us a piece
	// and that piece doesn't match up to our hash, then we want to put that piece back
	// into the channel, so another worker talking with a different peer can try it)
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
		pieceContents, err := tryDownloadPiece(peerClient, pieceToGet)
		if err != nil {
			return
		}

		// verify piece hash
		isHashGood := verifyPieceHash(pieceContents, pieceToGet.PieceHash[:])
		if isHashGood == false {
			log.Printf("Piece #%d failed integrity check, piece came from peer %s\n", pieceToGet.Index, p.String())
			workqueue <- pieceToGet
			continue
		}

		// hash looks OK. We now have the piece contents!

		// first tell peer the good news (that we have the piece)
		peerClient.SendHave(pieceToGet.Index)

		// then send the piece contents back up using the channel
		result := pieceResult{
			index:    pieceToGet.Index,
			contents: pieceContents,
		}
		results <- &result
	}
}

func tryDownloadPiece(client *client.Client, piece *PieceWork) ([]byte, error) {
	// initialize the state of this piece download
	// this is also where we allocate the byte slice to hold the eventual contents
	progress := PieceProgress{
		Index:         piece.Index,
		Client:        client,
		PieceContents: make([]byte, piece.Length),
	}

	// Setting a deadline helps get unresponsive peers unstuck.
	// 30 seconds is more than enough time to download a 262 KB piece
	client.Conn.SetDeadline(time.Now().Add(30 * time.Second))
	defer client.Conn.SetDeadline(time.Time{}) // Disable the deadline

	// check if we are done downloading this block
	for progress.Downloaded < piece.Length {
		// check if we are choked out by the peer, if we are don't bother sending a request
		if !progress.Client.Choked {
			// check if we have sent out requests for all the pieces at least
			// but don't send out any requests if we have sent out too many (backlog > 5)
			// btw there are no while loops in go, its just a for loop instead :P
			for progress.Backlog < MaxBacklog && progress.Requested < piece.Length {
				// change the blocksize requested if we are on the last block and
				// its not just the normal blocksize
				blockSize := NormalBlockSize
				if piece.Length-progress.Requested < NormalBlockSize {
					blockSize = piece.Length - progress.Requested
				}

				// first, send the request for the block
				err := client.SendRequest(piece.Index, progress.Requested, blockSize)
				if err != nil {
					return nil, err
				}

				// update the number of requests we have sent out
				progress.Backlog++

				// update the position that we need to request next
				progress.Requested += blockSize
			}
		}
		// ok so at this point we've sent out requests, check if anything came in back from peer
		err := progress.tryReadBlock()
		if err != nil {
			return nil, err
		}
	}

	// once we reach here, it means the entire piece has been downloaded successfully!
	// return the contents of the piece!
	return progress.PieceContents, nil
}

// we define this on PieceProgress because we wanna directly change the downloaded field
// this will be a general purpose message reader
func (p *PieceProgress) tryReadBlock() error {
	msg, err := p.Client.Read()
	if err != nil {
		return err
	}

	// check what kind of message it is
	switch msg.ID {
	case message.Piece:
		// copy into the piece buffer!
		bytesCopied, err := message.ParsePiece(p.PieceContents, p.Index, msg)
		if err != nil {
			return err
		}
		p.Downloaded += bytesCopied
		p.Backlog--
	case message.Choke:
		// we've been choked by peer :(
		p.Client.Choked = true
	case message.Unchoke:
		p.Client.Choked = false
	case message.Have:
		// peer can send us "have" messages which tell us which pieces it has
		// this is an alternative to the bitfield message. But we should be ready
		// to receive them if they come

		// get the index in question
		index, err := message.ParseHave(msg)
		if err != nil {
			return err
		}
		// set the bitfield such that it now marks the piece as owned for this peer
		p.Client.Bitfield.SetPiece(index)
	}
	return nil
}

func verifyPieceHash(pieceContents []byte, correctHashForThisPiece []byte) bool {
	hash := sha1.Sum(pieceContents)

	// check if the hashes are equal. Note we can't use != or ==, must use bytes.Equal
	if bytes.Equal(hash[:], correctHashForThisPiece[:]) {
		return false
	}
	return true
}
