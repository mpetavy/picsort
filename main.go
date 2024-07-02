package main

import (
	"crypto"
	"embed"
	"encoding/hex"
	"flag"
	"fmt"
	"github.com/evanoberholster/imagemeta"
	"github.com/mpetavy/common"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	EXIF          = "Exif"
	FILENAME      = "Filename"
	LAST_MODIFIED = "Last modified"
)

type Info struct {
	Path     string
	Date     time.Time
	DateFrom string
}

type Registry struct {
	Files map[string]Info
	sync.RWMutex
}

var (
	inputs   common.MultiValueFlag
	output   = flag.String("o", "", "output path")
	minsize  = flag.Int64("minsize", 100*1024, "minimum file length")
	registry = Registry{
		Files:   make(map[string]Info),
		RWMutex: sync.RWMutex{},
	}
	regexNumberPattern = "\\d+"
	regexNumber        *regexp.Regexp
)

//go:embed go.mod
var resources embed.FS

func init() {
	common.Init("", "", "", "", "picsort", "", "", "", &resources, nil, nil, run, 0)

	flag.Var(&inputs, "i", "input directory to scan")
}

func md5(path string) (string, error) {
	common.DebugFunc(path)

	md5 := crypto.MD5.New()

	f, err := os.Open(path)
	if common.Error(err) {
		return "", err
	}

	defer func() {
		common.Error(f.Close())
	}()

	_, err = io.Copy(md5, f)
	if common.Error(err) {
		return "", err
	}

	fingerprint := md5.Sum(nil)

	hash := hex.EncodeToString(fingerprint)

	common.DebugFunc("%s: %s", path, hash)

	return hash, nil
}

func stringDate(str string) (time.Time, error) {
	common.DebugFunc(str)

	founds := regexNumber.FindAllString(str, -1)

	for _, found := range founds {
		var t time.Time
		var err error

		switch len(found) {
		case 8:
			t, err = time.Parse(common.Year+common.Month+common.Day, found)
			if common.DebugError(err) {
				continue
			}
		case 10:
			switch {
			case strings.Contains(found, "/"):
				t, err = time.Parse(common.Month+"/"+common.Day+"/"+common.Year, found)
				if common.DebugError(err) {
					continue
				}
			case strings.Contains(found, "."):
				t, err = time.Parse(common.Day+"."+common.Month+"."+common.Year, found)
				if common.DebugError(err) {
					continue
				}
			}
		}

		if !t.IsZero() && t.Year() >= 1900 && t.Year() <= time.Now().Year() {
			common.DebugFunc("$s: %v", str, t)

			return t, nil
		}
	}

	return time.Time{}, fmt.Errorf("cannot find date")
}

func exifDate(path string) (time.Time, error) {
	f, err := os.Open(path)
	if common.Error(err) {
		return time.Time{}, err
	}
	defer func() {
		common.Error(f.Close())
	}()

	e, err := imagemeta.Decode(f)
	if common.Error(err) {
		return time.Time{}, err
	}

	return e.CreateDate(), nil
}

func processFile(path string, fi os.FileInfo) error {
	common.DebugFunc(path)

	filename := strings.ToLower(filepath.Base(path))
	isImage := strings.HasSuffix(filename, ".jpg") || strings.HasSuffix(filename, ".jpeg")
	isVideo := strings.HasSuffix(filename, ".mp4")

	if !isImage && !isVideo {
		return nil
	}

	hash, err := md5(path)
	if common.Error(err) {
		return err
	}

	dateSource := LAST_MODIFIED
	date := fi.ModTime()

	if isImage {
		dateSource = EXIF
		date, err := exifDate(path)
		if date.IsZero() || common.DebugError(err) {
			dateSource = FILENAME
			date, err = stringDate(filepath.Base(path))
			if common.DebugError(err) {
				dateSource = LAST_MODIFIED
				date = fi.ModTime()
			}
		}
	}

	common.Info("%s [%v][%s]\n", path, date, dateSource)

	//return nil

	registry.RLock()

	found, ok := registry.Files[hash]
	if ok {
		registry.RUnlock()

		common.Info(fmt.Sprintf("duplicate found: %s -> %s", found, path))

		return nil
	}

	registry.RUnlock()

	if *output == "" {
		return nil
	}

	registry.Lock()

	_, ok = registry.Files[hash]
	if ok {
		registry.Unlock()

		return nil
	}

	defer func() {
		registry.Unlock()
	}()

	ext := "jpg"
	media := "image"
	if isVideo {
		media = "video"
		ext = "mp4"
	}

	targetFile := filepath.Join(*output, media, strconv.Itoa(date.Year()), strconv.Itoa(int(date.Month())), fmt.Sprintf("%s-%s.%s", media, date.Format(common.SortedDateMask+common.Separator+common.TimeMask), ext))

	err = os.MkdirAll(filepath.Dir(targetFile), os.ModePerm)
	if common.Error(err) {
		return err
	}

	err = common.FileCopy(path, targetFile)
	if common.Error(err) {
		return err
	}

	if dateSource == EXIF {
		err = os.Chtimes(targetFile, date, date)
		if common.Error(err) {
			return err
		}
	}

	registry.Files[hash] = Info{
		Path:     targetFile,
		Date:     date,
		DateFrom: dateSource,
	}

	common.Info("%s [%v][%s]\n", path, date, dateSource)

	return nil
}

func processDir(wg *sync.WaitGroup, dir string) error {
	err := common.WalkFiles(dir, true, true, func(path string, fi os.FileInfo) error {
		if fi.IsDir() {
			return nil
		}

		if fi.Size() < *minsize {
			return nil
		}

		wg.Add(1)

		go func() {
			defer common.UnregisterGoRoutine(common.RegisterGoRoutine(1))

			defer wg.Done()

			common.Error(processFile(path, fi))
		}()

		return nil
	})
	if common.Error(err) {
		return err
	}

	return nil
}

func run() error {
	var err error

	regexNumber, err = regexp.Compile(regexNumberPattern)
	if common.Error(err) {
		return err
	}

	wg := sync.WaitGroup{}

	if *output != "" {
		common.Info("scan output: %s", *output)

		err := processDir(&wg, *output)
		if common.Error(err) {
			return err
		}
	}

	wg.Wait()

	for _, input := range inputs {
		common.Info("scan input: %s", input)

		err := processDir(&wg, input)
		if common.Error(err) {
			return err
		}
	}

	wg.Wait()

	return nil
}

func main() {
	common.Run([]string{"i"})
}
