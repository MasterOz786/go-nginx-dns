// Prints nginx layout discovery results for the current host.
//
// Usage on EC2 or any nginx server:
//
//	go run ./cmd/nginx-layout-check/
//
// Exit code 0 when discovery succeeds and nginx -t passes; 1 otherwise.
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/MasterOz786/go-nginx-dns/internal/handlers"
)

func main() {
	report, err := handlers.InspectNginxLayout()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	encoded, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(string(encoded))

	if !report.NginxTestOK {
		os.Exit(1)
	}
}
