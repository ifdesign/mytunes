package main

import (
	"bufio"
	"bytes"
	"fmt"
	"html"
	"io/ioutil"
	"net/url"
	"os"
	"os/exec"
	"path"
	"reflect"
	"regexp"
	"runtime"
	"strconv"
	"strings"

	"github.com/dhowden/itl"
	"github.com/fatih/color"
	"github.com/ifdesign/mytunes/internal"
)

type meta struct {
	Artist        string
	AlbumArtist   string
	Title         string
	Feat          string // `ft` hardcoded in buildFilePath
	Mix           string
	Genre         string
	Style         string
	Year          int
	Album         string
	IsCompilation bool
	TrackNumber   int
	Rating        int
	Grouping      string
	Comments      string
	Extension     string
}

type album struct {
	ID            string
	Artist        string
	Title         string
	TotalTracks   int
	Location      string
	IsCompilation bool
}

func main() {
	// Define console colors
	red := color.New(color.FgRed).SprintFunc()
	green := color.New(color.FgGreen).SprintFunc()

	// Load config
	config := internal.GetConfig()

	// Load library
	itlFile, err := os.Open(config.Lib)
	if err != nil {
		panic(err)
	}

	library, err := itl.ReadFromXML(itlFile)
	if err != nil {
		panic(err)
	}

	// Feed tracks from playlists
	var tracks []string
	var albums []album

	for _, playlist := range library.Playlists {
		if !listContainsString(config.Playlists, playlist.Name) {
			continue
		}

		for _, t := range playlist.PlaylistItems {
			tracks = append(tracks, strconv.Itoa(t.TrackID))
		}
	}

	// Build filename fields
	fields, optionalFields := buildFileNameFields(config.Filename)

	// Loop through tracks
	for _, trackID := range tracks {
		track := library.Tracks[trackID]
		fileSrc, err := getFileSrc(strings.Replace(track.Location, "file://", "", 1))

		// Skip non-audio track
		if !strings.Contains(strings.ToLower(track.Kind), "audio") {
			continue
		}

		// Skip "file not found" track
		if !fileExists(fileSrc) {
			fmt.Printf("%s %s\n", red("-> Not found:"), fileSrc)
			continue
		}

		// Build artist, title, feat, mix and rating field values
		feat, artist, title := getFeaturing(track.Artist, track.Name)
		mix, title := getMix(title)
		rating := track.Rating / 20

		// Create meta object
		meta := meta{
			Artist:        html.UnescapeString(artist),
			AlbumArtist:   html.UnescapeString(track.AlbumArtist),
			Feat:          html.UnescapeString(feat),
			Title:         html.UnescapeString(title),
			Mix:           html.UnescapeString(mix),
			Genre:         getGenre(track, config.Genres),
			Year:          track.Year,
			Album:         html.UnescapeString(track.Album),
			IsCompilation: track.Compilation,
			TrackNumber:   track.TrackNumber,
			Rating:        rating,
			Grouping:      strconv.Itoa(rating) + "*", // Using <1-5>* inside Grouping field as universal rating system
			Comments:      html.UnescapeString(track.Comments),
			Extension:     path.Ext(track.Location),
		}

		// Unset unrated track
		if config.UseUniversalRating && meta.Grouping == "0*" {
			meta.Grouping = ""
		}

		// Build destination path
		filePath := buildFilePath(config.CompilationArtist, config.Filename, meta, fields, optionalFields)
		albumDst, fileDst, err := prepareDst(config.Out, filePath)

		if err != nil {
			continue
		}

		// Register album
		if albumDst == "" {
			continue
		}

		albumID := meta.Artist + " - " + meta.Album
		albumExists := false

		for _, album := range albums {
			if album.ID == albumID {
				albumExists = true
				break
			}
		}

		if !albumExists {
			albums = append(albums, album{
				ID:          albumID,
				Artist:      meta.Artist,
				Title:       meta.Album,
				TotalTracks: track.TrackCount,
				Location:    albumDst,
			})
		}

		// Skip if destination file already exists
		if fileExists(fileDst) {
			fmt.Printf("%s %s\n", red("-> Destination file already exists:"), fileDst)
			continue
		}

		// Copy file to new location
		err = writeMeta(config.Mp4tagCmd, meta, fileSrc, fileDst)
		if err != nil {
			panic(err)
		}

		fmt.Printf("%s %s\n", green("-> Done:"), fileDst)
	}

	orderFilesByAlbum(albums, config.AlbumThreshold, config.MinimumAlbumTracks)
}

func fileExists(filePath string) bool {
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return false
	}
	return true
}

func listContainsString(list []string, s string) bool {
	for _, item := range list {
		if item == s {
			return true
		}
	}
	return false
}

func getGenre(track itl.Track, allGenres []map[string]string) string {
	var genre string
	var matches int
	var genres []string
	var matchedGenres []string
	stdin := bufio.NewReader(os.Stdin)
	yellow := color.New(color.FgYellow).SprintFunc()
	blue := color.New(color.FgCyan).SprintFunc()

	printGenres := func(genres []string, matchedGenres []string) string {
		for i, genre := range genres {
			if listContainsString(matchedGenres, genre) {
				fmt.Printf("%s %s\n", yellow(strconv.Itoa(i+1)+")"), yellow(genre))
			} else {
				fmt.Printf("%s %s\n", blue(strconv.Itoa(i+1)+")"), blue(genre))
			}
		}

		var userI int

		for {
			_, err := fmt.Fscan(stdin, &userI)
			if err == nil {
				break
			}

			stdin.ReadString('\n')
			fmt.Println("Sorry, invalid input. Please enter an integer: ")
		}

		return genres[userI-1]
	}

	for _, genredef := range allGenres {
		for name, rule := range genredef {
			genres = append(genres, name)
			matched, err := regexp.MatchString(rule, track.Genre)
			if err != nil {
				panic(err)
			}

			if matched {
				matches++
				matchedGenres = append(matchedGenres, name)
			}
		}
	}

	switch matches {
	case 0:
		fmt.Printf("No genre found for %s\n", yellow(track.Artist+" - "+track.Name))
		fmt.Printf("Original genre: %s\n", blue(track.Genre))
		fmt.Println("Please choose one:")

		genre = printGenres(genres, matchedGenres)

	case 1:
		genre = matchedGenres[0]

	case 2:
		fmt.Printf("More than 1 genre found for %s\n",
			yellow(track.Artist+" - "+track.Name))
		fmt.Println("Please select one:")

		genre = printGenres(genres, matchedGenres)
	}

	return genre
}

func getFeaturing(artist string, title string) (feat string, cleanedArtist string, cleanedTitle string) {
	re := regexp.MustCompile("(?i)\\(?f(ea)?t(\\.|uring)?\\s([^)]*)\\)?")

	titleMatch := re.FindAllStringSubmatch(title, -1)
	artistMatch := re.FindAllStringSubmatch(artist, -1)

	if len(titleMatch) > 0 {
		feat = titleMatch[0][3]
		cleanedTitle = strings.TrimSpace(strings.Replace(title, titleMatch[0][0], "", -1))
	} else {
		cleanedTitle = title
	}

	if len(artistMatch) > 0 {
		feat = artistMatch[0][3]
		cleanedArtist = strings.TrimSpace(strings.Replace(artist, artistMatch[0][0], "", -1))
	} else {
		cleanedArtist = artist
	}

	return
}

func getMix(title string) (mix string, cleanedTitle string) {
	re := regexp.MustCompile("(?i)[\\(\\[]\\s?([^\\(\\[\\)\\]]*mix|dub)\\s?[\\)\\]]")

	match := re.FindAllStringSubmatch(title, -1)

	if len(match) > 0 {
		mix = match[0][1]
		cleanedTitle = strings.TrimSpace(strings.Replace(title, match[0][0], "", -1))
	} else {
		cleanedTitle = title
	}

	fmt.Println("MIX", mix)

	return
}

func buildFileNameFields(fileNamePattern string) (fields []string, optionalFields [][]string) {
	fieldsRe := regexp.MustCompile("(?i)%(\\w+)")
	optionalFieldsRe := regexp.MustCompile("(?i)(\\[[^\\]]+\\])")
	optionalFieldNameRE := regexp.MustCompile("(?i)%(\\w+)")

	for _, match := range fieldsRe.FindAllStringSubmatch(fileNamePattern, -1) {
		fields = append(fields, match[1])
	}

	for _, match := range optionalFieldsRe.FindAllStringSubmatch(fileNamePattern, -1) {
		placeholder := match[0]
		fieldName := optionalFieldNameRE.FindAllStringSubmatch(placeholder, -1)[0][1]
		optionalFields = append(optionalFields, []string{fieldName, placeholder})
	}

	return
}

func getFieldValue(m meta, fieldName string) string {
	r := reflect.ValueOf(m)
	f := reflect.Indirect(r).FieldByName(strings.Title(fieldName))
	return fmt.Sprintf("%v", f)
}

func buildFilePath(compilationArtist string, pattern string, meta meta, fields []string, optionalFields [][]string) string {
	var fieldReplaces []string
	fieldValueReplacer := strings.NewReplacer(":", " -", "/", "-", "\\", "-")

	// Replace placeholders with value
	for _, field := range fields {
		fieldValue := getFieldValue(meta, field)
		if field == "albumArtist" && compilationArtist != "" {
			fieldValue = compilationArtist
		}
		fieldValue = fieldValueReplacer.Replace(fieldValue)

		if fieldValue != "" {
			fieldReplaces = append(fieldReplaces, "%"+field, fieldValue)
		}
	}

	fieldReplacer := strings.NewReplacer(fieldReplaces...)
	fileName := fieldReplacer.Replace(pattern)

	// Remove empty optional placeholders
	fieldReplaces = []string{}
	for _, fieldAndPlaceholder := range optionalFields {
		fieldName := fieldAndPlaceholder[0]
		placeholder := fieldAndPlaceholder[1]
		fieldValue := getFieldValue(meta, fieldName)

		if fieldValue == "" {
			fieldReplaces = append(fieldReplaces, placeholder, "")
		}
	}

	fieldReplacer = strings.NewReplacer(fieldReplaces...)
	fileName = fieldReplacer.Replace(fileName)

	// Remove brackets on optional fields
	fileName = strings.Replace(fileName, "\\[", "__BRACKETO__", -1)
	fileName = strings.Replace(fileName, "\\]", "__BRACKETC__", -1)
	fileName = strings.Replace(fileName, "[", "", -1)
	fileName = strings.Replace(fileName, "]", "", -1)
	fileName = strings.Replace(fileName, "__BRACKETO__", string('['), -1)
	fileName = strings.Replace(fileName, "__BRACKETC__", string(']'), -1)

	return fileName
}

func getFileSrc(src string) (fileSrc string, e error) {
	s, err := url.QueryUnescape(strings.Replace(src, "+", "__PLUS__", -1))
	if err != nil {
		e = err
	}

	fileSrc = html.UnescapeString(strings.Replace(s, "__PLUS__", "+", -1))
	return
}

func prepareDst(dstRoot string, dst string) (albumDst string, fileDst string, e error) {
	dst = path.Join(dstRoot, dst)
	fileDst = dst

	// Create dir
	dir := path.Dir(dst)
	err := os.MkdirAll(dir, 0755)
	if err != nil {
		e = err
	}

	albumDst = dir
	return
}

func writeMeta(mp4tagCmd string, meta meta, src string, dst string) error {
	red := color.New(color.FgRed).SprintFunc()
	var args []string
	var cmd *exec.Cmd
	var stderr bytes.Buffer
	name := meta.Title
	if meta.Feat != "" {
		name += " ft " + meta.Feat
	}

	if meta.Mix != "" {
		name += " (" + meta.Feat + ")"
	}

	if meta.Extension == ".m4a" || meta.Extension == ".m4p" {
		args = append(args, "--set", "Artist:S:"+meta.Artist)
		args = append(args, "--set", "Name:S:"+name)
		args = append(args, "--set", "Album:S:"+meta.Album)
		args = append(args, "--set", "Track:S:"+strconv.Itoa(meta.TrackNumber))
		args = append(args, "--set", "Year:S:"+strconv.Itoa(meta.Year))
		args = append(args, "--set", "GenreName:S:"+meta.Genre)
		args = append(args, "--set", "Grouping:S:"+meta.Grouping)
		args = append(args, "--set", "Comment:S:"+meta.Comments)
		args = append(args, src, dst)
		cmd = exec.Command(mp4tagCmd, args...)

		cmd.Stderr = &stderr
		err := cmd.Run()
		if err != nil {
			fmt.Printf("%s %s %s\n", red("-> Error writing meta:"), dst, stderr.String())
			return err
		}
	}

	if runtime.GOOS == "darwin" && fileExists(dst) {
		var tags []string

		if meta.Grouping != "" {
			tags = append(tags, meta.Grouping)
		}

		if meta.Comments != "" {
			tags = append(tags, strings.Split(meta.Comments, ";")...)
		}

		tagsStr := ""
		for i, tag := range tags {
			tagsStr += "\"" + tag + "\""

			if i < len(tags)-1 {
				tagsStr += ","
			}
		}

		args = nil
		args = append(args, "-w")
		args = append(args, "com.apple.metadata:_kMDItemUserTags")
		args = append(args, "("+tagsStr+")")
		args = append(args, dst)
		cmd = exec.Command("xattr", args...)

		cmd.Stderr = &stderr
		err := cmd.Run()
		if err != nil {
			fmt.Printf("%s %s %s\n", red("-> Error writing macOs Finder flag:"), dst, stderr.String())
			return err
		}
	}

	return nil
}

func orderFilesByAlbum(albums []album, albumThreshold float32, minimumAlbumTracks int) {
	red := color.New(color.FgRed).SprintFunc()

	for _, album := range albums {
		if album.TotalTracks == 0 {
			continue
		}

		count := 0

		files, err := ioutil.ReadDir(album.Location)
		if err != nil {
			fmt.Printf("%s %s\n", red("-> Could not read content of directory"), album.Location)
		}

		for _, file := range files {
			if !file.IsDir() {
				count++
			}
		}

		albumTracksThreshold := float32(count) / float32(album.TotalTracks)

		if (albumTracksThreshold < albumThreshold &&
			album.TotalTracks >= minimumAlbumTracks) ||
			album.TotalTracks < minimumAlbumTracks {
			for _, file := range files {
				fileName := file.Name()
				oldDir := album.Location
				newDir := path.Dir(album.Location)
				os.Rename(path.Join(oldDir, fileName), path.Join(newDir, fileName))
				os.Remove(oldDir)
			}
		}
	}
}
