package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"codeberg.org/blckr/spejder/internal/classify"
	"codeberg.org/blckr/spejder/internal/db"
	"codeberg.org/blckr/spejder/internal/ebpf"
	"codeberg.org/blckr/spejder/internal/geo"
)

func main() {
	cityDB := flag.String("city-db", "assets/geo/GeoLite2-City.mmdb", "path to GeoLite2-City.mmdb")
	asnDB := flag.String("asn-db", "assets/geo/GeoLite2-ASN.mmdb", "path to GeoLite2-ASN.mmdb")
	dbPath := flag.String("db", "spejder.db", "path to sqlite database")
	flag.Parse()

	database, err := db.Open(*dbPath)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer database.Close()

	geoReader, err := geo.Open(*cityDB, *asnDB)
	if err != nil {
		log.Fatalf("open geo database: %v", err)
	}
	defer geoReader.Close()

	collector, err := ebpf.NewCollector()
	if err != nil {
		log.Fatalf("create collector: %v", err)
	}
	defer collector.Close()

	log.Println("collecting — press ctrl+c to stop")

	go func() {
		err := collector.Run(func(event ebpf.Event) {
			info := geoReader.Lookup(event.SrcIP)
			t := classify.Classify(event.SrcIP, event.DstPort, info.ASNOrg)
			if err := db.InsertConnection(database, event.SrcIP.String(), event.DstPort, info.Country, info.City, info.ASN, info.ASNOrg, t); err != nil {
				log.Printf("insert: %v", err)
			}
		})
		if err != nil {
			log.Fatalf("collector: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	log.Println("shutting down")
}
