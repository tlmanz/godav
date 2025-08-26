package main

import (
	"fmt"
	"os"

	"github.com/tlmanz/godav"
)

func main() {
	client := godav.NewClient("https://nextcloud.example.com/remote.php/dav/", "username", "password")
	config := godav.DefaultConfig()
	config.Verbose = true

	// Show upload progress
	config.ProgressFunc = func(info godav.ProgressInfo) {
		fmt.Printf("Uploading %s: %.1f%%\n", info.Filename, info.Percentage)
	}

	// Upload a single file
	err := client.UploadFile("/path/to/local/file.txt", "remote/path/file.txt", config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "File upload error: %v\n", err)
	}

	// Upload a directory recursively
	err = client.UploadDir("/path/to/local/dir", "remote/path/dir", config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Directory upload error: %v\n", err)
	}
}
