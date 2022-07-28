package main

import (
	"log"
	"main/torrentfile"
	"os"
)

func main() {
	torrentFile := os.Args[1]
	downloadPath := os.Args[2]

	tf, err := torrentfile.Open(torrentFile)
	if err != nil {
		log.Fatal(err)
	}

	err = tf.DownloadToFile(downloadPath)
	if err != nil {
		log.Fatal(err)
	}
}
