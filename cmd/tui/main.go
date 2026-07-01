package main

import (
	"flag"
	"log"

	"codeberg.org/blckr/spejder/internal/tui"
)

func main() {
	dbPath := flag.String("db", "spejder.db", "path to sqlite database")
	flag.Parse()

	if err := tui.Run(*dbPath); err != nil {
		log.Fatal(err)
	}
}
