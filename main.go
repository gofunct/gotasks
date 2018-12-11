package main

import (
	"fmt"
	"github.com/gofunct/service/config"
	"os"
)

func main() {
	if err := config.RootCmd.Root.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}