package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/ifdesign/mytunes/internal"
	"github.com/jamesnetherton/m3u"
)

func main() {
	// Load config
	config := internal.GetConfig()

	fmt.Println(config.ExtractPlaylist)

	playlist, err := m3u.Parse(config.ExtractPlaylist)

	if err != nil {
		fmt.Println(err)
	}

	for _, track := range playlist.Tracks {
		var srcfd *os.File
		var dstfd *os.File
		var dstDir string
		src := filepath.Join(filepath.Dir(config.ExtractPlaylist), track.URI)
		dst := config.ExtractOut

		if _, err := os.Stat(src); err != nil {
			fmt.Println("Could not find", src)
			continue
		}

		for _, root := range config.Roots {
			if strings.HasPrefix(src, root) {
				dst = filepath.Join(dst, src[len(root):])
				dstDir = filepath.Dir(dst)
			}
		}

		if dst == config.ExtractOut {
			fmt.Println("No filename found for", src)
			continue
		}

		if srcfd, err = os.Open(src); err != nil {
			fmt.Println("Could not open file", src)
			continue
		}
		defer srcfd.Close()

		if err = os.MkdirAll(dstDir, 0775); err != nil {
			fmt.Println("Could not create dir", dstDir)
			continue
		}

		if dstfd, err = os.Create(dst); err != nil {
			fmt.Println("Could not create file", dst)
			continue
		}
		defer dstfd.Close()

		if _, err = io.Copy(dstfd, srcfd); err != nil {
			fmt.Println("Could not copy", src, "to", dst)
			continue
		}
	}
}
