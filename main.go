package main

import (
	"fmt"

	"github.com/alecthomas/kong"
)

type CLI struct {
	Version bool `short:"v" help:"Show version information"`
}

func main() {
	var cli CLI
	kong.Parse(&cli)

	if cli.Version {
		fmt.Println("fbs version 1.0.0")
		return
	}

	fmt.Println("Hello, World!")
}