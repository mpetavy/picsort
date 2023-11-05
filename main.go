package main

import (
	"crypto"
	"encoding/hex"
	"flag"
	"fmt"
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
	dry                = flag.Bool("d", true, "run dry")
	registry           = make(Registry)
	mu                 sync.RWMutex
	regexNumberPattern = "\\d+"
	regexNumber        *regexp.Regexp
)

func init() {
	common.Init("picsort", "", "", "", "2023", "picsort", "mpetavy", fmt.Sprintf("https://github.com/mpetavy/%s", common.Title()), common.APACHE, nil, nil, nil, run, 0)

	flag.Var(&inputs, "i", "input directory to scan")
}

func synchronize(fn func() error) error {
	mu.Lock()
	defer mu.Unlock()

	return fn()
}

func md5(path string) ([]byte, error) {
	hash := crypto.MD5.New()

	f, err := os.Open(path)
	if common.Error(err) {
		return nil, err
	}

	defer func() {
		common.Error(f.Close())
	}()

	_, err = io.Copy(hash, f)
	if common.Error(err) {
		return nil, err
	}

	return hash.Sum(nil), nil
}

func dateOfFile(path string, fi os.FileInfo) (time.Time, error) {
	return fi.ModTime(), nil
}

func parseDate(str string) (time.Time, error) {
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
			defer func() {
				wg.Done()
			}()

			md5, err := md5(path)
			if common.Error(err) {
				return
			}

			hash := hex.EncodeToString(md5)

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

func run() error {
	var err error

	regexNumber, err = regexp.Compile(regexNumberPattern)
	if common.Error(err) {
		return err
	}

	err = process(common.CleanPath(*output), func(path string, fi os.FileInfo, hash string) error {
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

	for _, input := range inputs {
		err := process(common.CleanPath(input), func(path string, fi os.FileInfo, hash string) error {
			dateFrom := "Filename"
			date, err := parseDate(filepath.Base(path))
			if common.DebugError(err) {
				date = fi.ModTime()
				dateFrom = "Last modified"
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

			if !*dry {
				common.IgnoreError(os.MkdirAll(targetDir, os.ModePerm))

				err = common.FileCopy(path, targetFile)
				if common.Error(err) {
					return err
				}
			}

			return nil
		})
		if common.Error(err) {
			return err
		}
	}

	return nil
}

func main() {
	common.Run([]string{"i", "o"})
}
