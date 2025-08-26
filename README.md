
# godav

🚀 A Go library for WebDAV with full support for Nextcloud chunked uploads and advanced file operations.

## Features

- High-level client for Nextcloud WebDAV
- Chunked uploads (bypass proxy body-size limits)
- Recursive directory uploads
- Progress reporting and verbose logging
- Skips files that already exist with the same size

## Installation

```sh
go get github.com/tlmanz/godav
```

## Usage

```go
package main

import (
	"github.com/tlmanz/godav"
)

func main() {
	client := godav.NewClient("https://nextcloud.example.com/remote.php/dav/", "username", "password")
	config := godav.DefaultConfig()
	config.Verbose = true

	// Upload a single file
	err := client.UploadFile("/path/to/local/file.txt", "remote/path/file.txt", config)
	if err != nil {
		panic(err)
	}

	// Upload a directory recursively
	err = client.UploadDir("/path/to/local/dir", "remote/path/dir", config)
	if err != nil {
		panic(err)
	}
}
```

## Configuration

You can customize upload behavior using the `Config` struct:

```go
type Config struct {
	ChunkSize    int64                   // Chunk size in bytes (default 10MB)
	SkipExisting bool                    // Skip files that exist with same size
	Verbose      bool                    // Enable verbose logging
	ProgressFunc func(info ProgressInfo) // Progress callback with detailed info
}
```

Example: Show upload progress

```go
config.ProgressFunc = func(info godav.ProgressInfo) {
	fmt.Printf("Uploading %s: %.1f%%\n", info.Filename, info.Percentage)
}
```

## License

This project is licensed under the MIT License.
