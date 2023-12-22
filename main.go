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

type RegistryInfo struct {
	Path     string
	Date     time.Time
	DateFrom string
}
type Registry = map[string]RegistryInfo

var (
	inputs             common.MultiValueFlag
	output             = flag.String("o", "", "output path")
	registry           = make(Registry)
	mu                 sync.RWMutex
	regexNumberPattern = "\\d+"
	regexNumber        *regexp.Regexp
)

//go:embed go.mod
var resources embed.FS

func init() {
	common.Init("", "", "", "", "picsort", "", "", "", &resources, nil, nil, run, 0)

	flag.Var(&inputs, "i", "input directory to scan")
}

func synchronize(fn func() error) error {
	mu.Lock()
	defer mu.Unlock()

	return fn()
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

func process(path string, fn func(path string, fi os.FileInfo, hash string) error) error {
	wg := sync.WaitGroup{}

	err := common.WalkFiles(path, true, false, func(path string, fi os.FileInfo) error {
		if fi.IsDir() {
			return nil
		}

		wg.Add(1)

		go func(path string, fi os.FileInfo) {
			common.DebugFunc(path)

			defer func() {
				wg.Done()
			}()

			hash, err := md5(path)
			if common.Error(err) {
				return
			}

			err = fn(path, fi, hash)
			if common.Error(err) {
				return
			}
		}(path, fi)

		return nil
	})
	if common.Error(err) {
		return err
	}

	wg.Wait()

	return nil
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

func scanOutput() error {
	if *output == "" {
		return nil
	}

	err := process(common.CleanPath(*output), func(path string, fi os.FileInfo, hash string) error {
		err := synchronize(func() error {
			found, ok := registry[hash]
			if ok {
				return fmt.Errorf("duplicate found: %s -> %s", found, path)
			}

			registry[hash] = RegistryInfo{
				Path: path,
			}

			return nil
		})
		if common.WarnError(err) {
			return nil
		}

		return nil
	})
	if common.Error(err) {
		return err
	}

	return err
}

func scanInputs() error {
	for _, input := range inputs {
		err := process(common.CleanPath(input), func(path string, fi os.FileInfo, hash string) error {
			dateFrom := "Exif"
			date, err := exifDate(path)
			if date.IsZero() || common.DebugError(err) {
				dateFrom = "Filename"
				date, err = stringDate(filepath.Base(path))
				if common.DebugError(err) {
					dateFrom = "Last modified"
					date = fi.ModTime()
				}
			}

			date = date.In(time.UTC).Truncate(time.Second)
			modtime := fi.ModTime().In(time.UTC).Truncate(time.Second)

			if date != modtime {
				common.Warn("possible wrong date: %s\n\tfile\t%v\n\tfound\t%v [%s]", path, modtime, date, dateFrom)
			}

			if *output == "" {
				return nil
			}

			targetDir := filepath.Join(*output, strconv.Itoa(date.Year()), strconv.Itoa(int(date.Month())))
			targetFile := filepath.Join(targetDir, filepath.Base(path))

			err = synchronize(func() error {
				found, ok := registry[hash]
				if ok {
					return fmt.Errorf("duplicate found: %s -> %s", found, path)
				}

				registry[hash] = RegistryInfo{
					Path:     targetFile,
					Date:     date,
					DateFrom: dateFrom,
				}

				common.Info("%s [%v][%s]\n", path, date, dateFrom)

				return nil
			})

			if common.WarnError(err) {
				return nil
			}

			common.IgnoreError(os.MkdirAll(targetDir, os.ModePerm))

			err = common.FileCopy(path, targetFile)
			if common.Error(err) {
				return err
			}

			return nil
		})
		if common.Error(err) {
			return err
		}
	}

	return nil
}

func run() error {
	var err error

	regexNumber, err = regexp.Compile(regexNumberPattern)
	if common.Error(err) {
		return err
	}

	err = scanOutput()
	if common.Error(err) {
		return err
	}

	err = scanInputs()
	if common.Error(err) {
		return err
	}

	return nil
}

func main() {
	common.Run([]string{"i"})
}
