package main

import (
	"fmt"
	"github.com/mpetavy/common"
)

func init() {
	common.Init("starter", "", "", "", "2018", "starter", "mpetavy", fmt.Sprintf("https://github.com/mpetavy/%s", common.Title()), common.APACHE, nil, nil, nil, run, 0)
}

func run() error {
	return nil
}

func main() {
	common.Run(nil)
}
