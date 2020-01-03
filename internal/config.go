package internal // import "github.com/ifdesign/mytunes/internal"

import (
	"io/ioutil"
	"os"

	"gopkg.in/yaml.v2"
)

// Config represents the typed yaml config file
type Config struct {
	Mp4tagCmd          string `yaml:"mp4tag-cmd"`
	Lib                string
	Out                string
	Roots              []string
	ExtractPlaylist    string `yaml:"extract-playlist"`
	ExtractOut         string `yaml:"extract-out"`
	Filename           string
	Feat               string
	Playlists          []string
	Genres             []map[string]string
	AlbumThreshold     float32 `yaml:"album-threshold"`
	MinimumAlbumTracks int     `yaml:"minimum-album-tracks"`
	UseUniversalRating bool    `yaml:"use-universal-rating"`
	CompilationArtist  string  `yaml:"compilation-artist"`
}

// GetConfig parses the yaml config file and returns a Config
func GetConfig() Config {
	var config Config
	filename := os.Args[1]
	source, err := ioutil.ReadFile(filename)
	if err != nil {
		panic(err)
	}

	err = yaml.Unmarshal(source, &config)
	if err != nil {
		panic(err)
	}

	return config
}
