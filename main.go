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
	"sync"
)

var (
	input  common.MultiValueFlag
	output = flag.String("o", "", "output path")

	pics     = make(map[string]string)
	syncPics = common.NewSyncOf(pics)
)

func init() {
	common.Init("picsort", "", "", "", "2023", "picsort", "mpetavy", fmt.Sprintf("https://github.com/mpetavy/%s", common.Title()), common.APACHE, nil, nil, nil, run, 0)

	flag.Var(&input, "i", "directory to include")
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

func run() error {
	wg := sync.WaitGroup{}
	err := common.WalkFiles(filepath.Join(common.CleanPath(*output), "*.jpg"), true, true, func(path string, fi os.FileInfo) error {
		if common.IsDirectory(path) {
			return nil
		}

		wg.Add(1)

		go func(path string) {
			defer func() {
				wg.Done()
			}()

			md5, err := md5(path)
			if common.Error(err) {
				return
			}

			syncPics.Run(func(m map[string]string) {
				hash := hex.EncodeToString(md5)

				existingPath, ok := pics[hash]
				if ok {
					common.Warn("duplicate found: %s -> %s", existingPath, path)

					return
				}

				m[hash] = path
			})
		}(path)

		return nil
	})
	if common.Error(err) {
		return err
	}

	wg.Wait()

	for hash, path := range pics {
		fmt.Printf("%s: %s\n", hash, path)
	}

	//for _, path := range input {
	//	common.WalkFiles(filepath.Join(common.CleanPath(path), "*.jpg"), true, true, func(path string, fi os.FileInfo) error {
	//		fmt.Printf("%s\n", path)
	//
	//		return nil
	//	})
	//}

	return nil
}

func main() {
	common.Run(nil)
}
