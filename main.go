package main

import (
	"fmt"
	"os"

	"github.com/hautajoki/rogen/v2/internal/rogen"
)

func main() {
	if err := rogen.Run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "\nBuild Failed: %v\n\n", err)
		os.Exit(1)
	}
}
