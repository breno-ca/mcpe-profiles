package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) >= 3 && (os.Args[1] == "-profile" || os.Args[1] == "--profile") {
		if err := runProfile(os.Args[2]); err != nil {
			fmt.Fprintln(os.Stderr, "mcpe-profiles:", err)
			os.Exit(1)
		}
		return
	}
	runGUI()
}
