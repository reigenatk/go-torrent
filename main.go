package main

import (
	"fmt"
	"log"
	"main/torrentfile"
	"os"
)

func main() {
	torrentFile := os.Args[1]
	downloadPath := os.Args[2]

	if len(os.Args) != 3 {
		fmt.Printf("Usage : torrent [path to .torrent file] [path to where you want file to download] \n ", os.Args[0]) // return the program name back to %s
		os.Exit(1)                                                                                                      // graceful exit
	}

	tf, err := torrentfile.Open(torrentFile)
	if err != nil {
		log.Fatal(err)
	}

	err = tf.DownloadToFile(downloadPath)
	if err != nil {
		log.Fatal(err)
	}
}
