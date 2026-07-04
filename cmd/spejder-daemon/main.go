package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"codeberg.org/blckr/spejder/internal/classify"
	"codeberg.org/blckr/spejder/internal/db"
	"codeberg.org/blckr/spejder/internal/ebpf"
	"codeberg.org/blckr/spejder/internal/geo"
)

type connInfo struct {
	dbID      int64
	startedAt time.Time
}

var (
	activeConnsMu sync.Mutex
	activeConns   = make(map[uint64]connInfo)
)

func translateCloseReason(oldState uint8) string {
	switch oldState {
	case 1: // TCP_ESTABLISHED -> CLOSE (abrupt / reset)
		return "reset"
	case 4, 5, 6: // FIN_WAIT1, FIN_WAIT2, TIME_WAIT (active close)
		return "active"
	case 8, 9: // CLOSE_WAIT, LAST_ACK (passive close)
		return "passive"
	default:
		return "unknown"
	}
}

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
			switch event.EventType {
			case 1: // Connection established (Start)
				direction := "incoming"
				if event.OldState == 2 { // TCP_SYN_SENT
					direction = "outgoing"
				}

				// Look up GeoIP for the remote IP
				info := geoReader.Lookup(event.RemoteIP)

				// Determine traffic type using classify.
				// For incoming connections, our listening port is LocalPort.
				// For outgoing connections, the target port is RemotePort.
				var svcPort uint16
				if direction == "incoming" {
					svcPort = event.LocalPort
				} else {
					svcPort = event.RemotePort
				}
				t := classify.Classify(event.RemoteIP, svcPort, info.ASNOrg)

				// Insert starting connection into DB
				dbID, err := db.InsertConnection(
					database,
					direction,
					event.RemoteIP.String(),
					event.RemotePort,
					event.LocalPort,
					info.Country,
					info.City,
					info.ASN,
					info.ASNOrg,
					t,
				)
				if err != nil {
					log.Printf("insert connection start error: %v", err)
					return
				}

				// Save database row ID and start time in memory
				activeConnsMu.Lock()
				activeConns[event.SocketID] = connInfo{
					dbID:      dbID,
					startedAt: time.Now(),
				}
				activeConnsMu.Unlock()

			case 2: // Connection closed (End)
				// Look up matching connection in memory
				activeConnsMu.Lock()
				conn, exists := activeConns[event.SocketID]
				if exists {
					delete(activeConns, event.SocketID)
				}
				activeConnsMu.Unlock()

				// If we found it, calculate duration and update DB
				if exists {
					durationMs := time.Since(conn.startedAt).Milliseconds()
					reason := translateCloseReason(event.OldState)

					if err := db.CloseConnection(database, conn.dbID, durationMs, reason); err != nil {
						log.Printf("close connection error: %v", err)
					}
				}
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
