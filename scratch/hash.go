package main

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
)

func main() {
	orders := []string{
		"882909095737083",
		"882815136646675",
		"882113947207620",
		"882021663565301",
		"881631784449331",
		"881969704436290",
		"881685094481349",
		"881666819984445",
		"881601567885383",
		"881537155893258",
	}

	for _, code := range orders {
		hash := md5.Sum([]byte(code))
		hashHex := hex.EncodeToString(hash[:])
		fmt.Printf("%s -> %s\n", code, hashHex)
	}
}
