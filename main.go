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
	"strconv"
	"sync"
	"time"
)

type registry = map[string]string

var (
	inputs common.MultiValueFlag
	output = flag.String("o", "", "output path")

	target = make(registry)
	mu     sync.RWMutex
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

func dateOf(path string, fi os.FileInfo) (time.Time, error) {
	return fi.ModTime(), nil
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

func copyFile(source string, fi os.FileInfo, dest string) error {
	common.Info("copy file: %s -> %s", source, dest)

	err := common.FileCopy(source, dest)
	if common.Error(err) {
		return err
	}

	err = os.Chtimes(dest, fi.ModTime(), fi.ModTime())
	if common.Error(err) {
		return err
	}

	return nil
}

func run() error {
	err := process(common.CleanPath(*output), func(path string, fi os.FileInfo, hash string) error {
		err := synchronize(func() error {
			found, ok := target[hash]
			if ok {
				return fmt.Errorf("duplicate found: %s -> %s", found, path)
			}

			target[hash] = path

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
			date, err := dateOf(path, fi)
			if common.Error(err) {
				return err
			}

			targetDir := filepath.Join(*output, strconv.Itoa(date.Year()), strconv.Itoa(int(date.Month())))
			targetFile := filepath.Join(targetDir, filepath.Base(path))

			err = synchronize(func() error {
				found, ok := target[hash]
				if ok {
					return fmt.Errorf("duplicate found: %s -> %s", found, path)
				}

				target[hash] = targetFile

				return nil
			})

			if common.WarnError(err) {
				return nil
			}

			os.MkdirAll(targetDir, os.ModePerm)

			err = copyFile(path, fi, targetFile)
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

func main() {
	common.Run(nil)
}
