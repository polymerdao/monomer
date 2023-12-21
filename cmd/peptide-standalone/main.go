package main

import (
	"fmt"
	"os"

	nodecmd "github.com/polymerdao/monomer/app/node/cmd"
)

func main() {
	cmd := nodecmd.RootStandaloneCmd()
	if err := cmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
