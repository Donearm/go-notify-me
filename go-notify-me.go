/* Go-notify-me
*
* MPD notification status script
*
* © 2013-2020, Gianluca Fiore
*
 */

package main

import (
	"bufio"
	"fmt"
	mpd "github.com/fhs/gompd/mpd"
	notify "github.com/mqu/go-notify"
	resize "github.com/nfnt/resize"
	"image/jpeg"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// connectToServer connects to a MPD server
// address to the server and its password are required
func connectToServer(addr, pwd string) (cli *mpd.Client) {
	// first make sure that MPD server is running
	for s := checkMpdIsListening(addr); s != true; s = checkMpdIsListening(addr) {
		fmt.Println("Waiting for the MPD server to go up...")
		time.Sleep(10 * time.Second)
	}
	cli, err := mpd.DialAuthenticated("tcp", addr, pwd)
	if err != nil {
		fmt.Println("Couldn't connect to MPD server")
		os.Exit(2)
	}
	return cli
}

// checkMpdIsListening checks if the MPD server is ready (is listening for 
// connections)
func checkMpdIsListening(addr string) bool {
	conn, err := net.DialTimeout("tcp", addr, 1*time.Second)
	if err != nil {
		return false
	}
	defer conn.Close()

	return true
}

// getMusicDirectory obtains the path to the music directory from local's MPD 
// configuration file
func getMusicDirectory() string {
	var dir string
	var rxp = regexp.MustCompile(`^music_directory.*`)
	file, err := os.Open("/etc/mpd.conf")
	if err != nil {
		fmt.Println("Couldn't read MPD configuration, exiting...")
		os.Exit(2)
	}
	defer file.Close()
	s := bufio.NewScanner(file)
	for s.Scan() {
		l := strings.Trim(s.Text(), " ")
		matched := rxp.MatchString(l)
		if matched {
			// Split the music_directory line on ", get the second item
			// of the resulting slice
			dir = strings.Split(string(l), `"`)[1]
		}
	}
	return dir
}

// coverSearch looks for a matching image for the currently playing song in the 
// directory of the album to use as the album's cover
func coverSearch(path string) string {
	var patterns = []string{`.*[Ff]ront.*`, `.*[Ff]older.*`, `.*[Aa]lbumart.*`, `.*[Cc]over.*`, `.*[Tt]humb.*`, `.*[Ff]older.*`}

	dir, err := os.Open(path)
	if err != nil {
		fmt.Println("Couldn't access path of the current song")
		os.Exit(2)
	}
	defer dir.Close()

	files, rErr := dir.Readdir(0)
	if rErr != nil {
		fmt.Println("Couldn't browse the path of the current song")
		os.Exit(2)
	}
	for _, f := range files {
		for _, p := range patterns {
			rxp := regexp.MustCompile(p)
			cover := rxp.MatchString(f.Name())
			if cover {
				absPath := filepath.Join(path, f.Name())
				return absPath
			}
		}
	}
	return ""
}

// launchNotification shows a desktop notification with song's metatada and an 
// album cover's thumbnail
func launchNotification(name, txt, image string, delay int32) {
	notify.Init(name)

	coverartNotify := notify.NotificationNew(name, txt, image)
	if coverartNotify == nil {
		fmt.Fprintf(os.Stderr, "There was an error with the notification")
		return
	}

	// Set timeout
	notify.NotificationSetTimeout(coverartNotify, delay)

	// Show the notification!
	if err := notify.NotificationShow(coverartNotify); err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err.Message())
		return
	}

	notify.NotificationClose(coverartNotify)
	notify.UnInit()
}

// resizeImage resizes an image to an arbitrary widthXheight and saves it to a 
// temporary file
func resizeImage(image string, width, height uint) string {
	// thumbnail path and name
	var thumbName = filepath.Join(os.TempDir(), "mpdthumb.jpg")
	file, err := os.Open(image)

	if err != nil {
		fmt.Fprintf(os.Stderr, "Couldn't open the coverart file\n")
		return ""
	}

	img, err := jpeg.Decode(file)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Couldn't decode image to a compatible format\n")
		return ""
	}

	file.Close()

	// resize
	thumb := resize.Resize(width, height, img, resize.NearestNeighbor)

	// create the thumbnail file
	tmpfile, err := os.Create(thumbName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating the thumbnails\n")
		return ""
	}

	defer tmpfile.Close()

	// write image to the thumbnail temporary file
	jpeg.Encode(tmpfile, thumb, nil)

	return tmpfile.Name()
}

func main() {
	var address = "127.0.0.1:6600"		// MPD server address
	var originalID = 657932				// starting Id. An absurdly high number just
	// to be sure it's not the same as songID

	var originalStatus = ""             // starting MPD's status
	var songID int                      // Id of the current song
	var musicDir string                 // path of MPD music database
	var coverImg string                 // path of the image of the
	// the current song's album cover
	var thumbImage string                        // path of the thumbnail of coverImg
	var artist, title, album, file, state string // metadata info
	var statusStr string                         // string containing the status message
	// according to MPD's status
	var returnStr string // returning string to output alongside the cover

	c := connectToServer(address, "")

	defer c.Close()

	musicDir = getMusicDirectory()

	for {
		// check status
		status, err := c.Status()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error getting status\n")
			time.Sleep(30 * time.Second)
		}

		// get current song
		song, err := c.CurrentSong()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Couldn't get info on current song\n")
			time.Sleep(30 * time.Second)
		}

		songID, _ = strconv.Atoi(song["Id"]) // convert string to int
		artist = song["Artist"]
		title = song["Title"]
		album = song["Album"]
		file = song["file"]
		state = status["state"]

		// build the notification string
		returnStr = fmt.Sprintf("Artist: %s\nSong: %s\nAlbum: %s", artist, title, album)

		// have a different string on different MPD statuses
		if state == "play" {
			statusStr = "Now Playing"
		} else if state == "pause" {
			statusStr = "Now Paused"
			returnStr = fmt.Sprintf("Artist: %s\nSong: %s\nAlbum: %s", artist, title, album)
		} else if state == "stop" {
			statusStr = "Stopped"
			returnStr = fmt.Sprintf("Artist: %s\nSong: %s\nAlbum: %s", artist, title, album)
		} else {
			statusStr = "??"
			returnStr = fmt.Sprintf("Unknown state")
			// TODO: how to stop and restart the for loop when MPD comes
			// down while go-notify-me is running?
		}

		// if id of song or status changed, emit the notification
		if songID != originalID || originalStatus != state {
			originalID = songID
			originalStatus = state
			// check that we have a filename for the current song and use it
			// to find the cover art for its album
			if file != "" {
				coverSplit := strings.Split(file, "/")
				fileDirName := coverSplit[:len(coverSplit)-1]
				coverImg = coverSearch(filepath.Join(musicDir, strings.Join(fileDirName, "/")))
				if coverImg != "" {
					thumbImage = resizeImage(coverImg, 80, 0)
				} else {
					thumbImage = ""
				}
			}
			launchNotification(statusStr, returnStr, thumbImage, 3000)
		} else {
			// sleep a couple of sec and then retry
			time.Sleep(2000 * time.Millisecond)
		}
	}
}
