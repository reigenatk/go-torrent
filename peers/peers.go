package peers

import (
	"encoding/binary"
	"fmt"
	"net"
	"strconv"
)

// a Peer returned in the tracker response consists of an 4-byte IP and a 2-byte port
type Peer struct {
	IP   net.IP
	Port uint16
}

type PeersResponse struct {
	Interval int    `bencode:"interval"`
	Peers    string `bencode:"peers"`
}

// transforms a string of peers info (6 byte ip+port chunks over and over again)
// into []Peer
func Unmarshal(s string) ([]Peer, error) {
	bytesrep := []byte(s)
	chunkSize := 6
	if len(bytesrep)%chunkSize != 0 {
		err := fmt.Errorf("Tracker response's peers list's size in bytes is not divide by 6, size is %d", len(bytesrep))
		return nil, err
	}
	numPeers := len(bytesrep) / chunkSize
	ret := make([]Peer, numPeers)
	for i := 0; i < numPeers; i++ {
		offset := i * chunkSize
		ret[i].IP = net.IP(bytesrep[offset : (offset)+4])
		ret[i].Port = binary.BigEndian.Uint16(bytesrep[(offset)+4 : (i+1)*chunkSize])
		// log.Println("Found peer", ret[i].String())
	}
	return ret, nil
}

// a quick function to convert this peer's IP and port info into a string
// like "213.23.121.94:80" or something
func (p *Peer) String() string {
	return p.IP.String() + ":" + strconv.Itoa(int(p.Port))
}
