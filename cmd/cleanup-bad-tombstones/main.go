// cleanup-bad-tombstones masks Tombstone2 records in Things Cloud whose
// dloid field is malformed (anything other than a 22-char Base58 UUID).
// Things desktop has been observed to crash during sync on such tombstones.
//
// Workflow:
//
//	cleanup-bad-tombstones --history-id <id> --scan                  # list bad ones
//	cleanup-bad-tombstones --history-id <id> --scan > bad.txt
//	cleanup-bad-tombstones --history-id <id> --list bad.txt --dry-run
//	cleanup-bad-tombstones --history-id <id> --list bad.txt
//
// Cloud is append-only — we can't delete the bad tombstones. The tool writes
// a new valid-format Tombstone2 whose dloid targets each bad Tombstone2's
// own UUID, so a Things client that processes the new tombstone first (or
// can tolerate the bad one) will end up with the bad ones removed locally.
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math/big"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"

	thingscloud "github.com/arthursoares/things-cloud-sdk"
)

type envelope struct {
	id      string
	action  int
	kind    string
	payload any
}

func (e envelope) UUID() string { return e.id }

func (e envelope) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		T int    `json:"t"`
		E string `json:"e"`
		P any    `json:"p"`
	}{e.action, e.kind, e.payload})
}

func generateUUID() string {
	u := uuid.New()
	const alphabet = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"
	n := new(big.Int).SetBytes(u[:])
	base := big.NewInt(58)
	mod := new(big.Int)
	var encoded []byte
	for n.Sign() > 0 {
		n.DivMod(n, base, mod)
		encoded = append(encoded, alphabet[mod.Int64()])
	}
	for i, j := 0, len(encoded)-1; i < j; i, j = i+1, j-1 {
		encoded[i], encoded[j] = encoded[j], encoded[i]
	}
	for len(encoded) < 22 {
		encoded = append([]byte{alphabet[0]}, encoded...)
	}
	return string(encoded)
}

// scanForBadTombstones walks every item in the history and prints the UUID
// of any Tombstone2 whose dloid is not a 22-char string.
func scanForBadTombstones(history *thingscloud.History) error {
	startIndex := 0
	bad := 0
	for {
		items, more, err := history.Items(thingscloud.ItemsOptions{StartIndex: startIndex})
		if err != nil {
			return fmt.Errorf("fetch items at %d: %w", startIndex, err)
		}
		if len(items) == 0 {
			break
		}
		for _, item := range items {
			if item.Kind != thingscloud.ItemKindTombstone {
				continue
			}
			var p struct {
				DloID string `json:"dloid"`
			}
			if err := json.Unmarshal(item.P, &p); err != nil {
				continue
			}
			if len(p.DloID) != 22 {
				fmt.Println(item.UUID)
				bad++
			}
		}
		if !more {
			break
		}
		startIndex = history.LoadedServerIndex
	}
	log.Printf("scan complete: %d bad-dloid tombstones found", bad)
	return nil
}

func loadTargets(path string) ([]string, error) {
	var r *bufio.Scanner
	if path == "-" {
		r = bufio.NewScanner(os.Stdin)
	} else {
		f, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		r = bufio.NewScanner(f)
	}
	var out []string
	for r.Scan() {
		line := strings.TrimSpace(r.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if len(line) != 22 {
			return nil, fmt.Errorf("target %q is not 22 chars; refusing to create more bad data", line)
		}
		out = append(out, line)
	}
	return out, r.Err()
}

func main() {
	historyID := flag.String("history-id", os.Getenv("THINGS_HISTORY_ID"), "Things Cloud history ID (or set $THINGS_HISTORY_ID)")
	scan := flag.Bool("scan", false, "scan the history and print UUIDs of bad-dloid tombstones to stdout")
	listFile := flag.String("list", "", "path to file with one bad-tombstone UUID per line ('-' for stdin)")
	dryRun := flag.Bool("dry-run", false, "with --list: log what would be written without writing")
	limit := flag.Int("limit", 0, "with --list: process at most N entries (0 = all)")
	flag.Parse()

	if *historyID == "" {
		log.Fatal("--history-id (or $THINGS_HISTORY_ID) is required")
	}
	if *scan == (*listFile != "") {
		log.Fatal("specify exactly one of --scan or --list")
	}

	client := thingscloud.New(thingscloud.APIEndpoint, "x@example.com", "")
	client.Debug = os.Getenv("DEBUG") == "true"
	history := client.HistoryWithID(*historyID)
	if err := history.Sync(); err != nil {
		log.Fatalf("history.Sync: %v", err)
	}

	if *scan {
		if err := scanForBadTombstones(history); err != nil {
			log.Fatal(err)
		}
		return
	}

	targets, err := loadTargets(*listFile)
	if err != nil {
		log.Fatalf("load targets: %v", err)
	}
	if *limit > 0 && *limit < len(targets) {
		targets = targets[:*limit]
	}
	log.Printf("loaded %d target tombstone UUIDs, ancestor-index=%d", len(targets), history.LatestServerIndex)

	if *dryRun {
		for _, t := range targets {
			log.Printf("DRY-RUN would tombstone-of-tombstone target=%s", t)
		}
		return
	}

	wrote := 0
	for i, target := range targets {
		newID := generateUUID()
		env := envelope{
			id:     newID,
			action: 0,
			kind:   "Tombstone2",
			payload: map[string]any{
				"dld":   float64(time.Now().UnixNano()) / 1e9,
				"dloid": target,
			},
		}
		if err := history.Write(env); err != nil {
			log.Printf("[%d/%d] FAIL %s: %v — re-syncing and continuing", i+1, len(targets), target, err)
			if err := history.Sync(); err != nil {
				log.Fatalf("re-sync after fail: %v", err)
			}
			continue
		}
		wrote++
		log.Printf("[%d/%d] OK tombstoned %s via %s", i+1, len(targets), target, newID)
		if err := history.Sync(); err != nil {
			log.Fatalf("re-sync after success: %v", err)
		}
	}
	fmt.Printf("done: wrote %d / %d tombstones\n", wrote, len(targets))
}
