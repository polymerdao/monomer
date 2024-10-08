package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
)

const (
	defaultPort = "9091"
	timeout     = 5 * time.Second
)

// Starts a simple HTTP server to serve the Keplr wallet integration used to deploy the Monomer devnet chain configs.
func main() {
	portFlag := flag.String("port", defaultPort, "Port to serve Keplr integration server on")
	flag.Parse()

	var port string
	if *portFlag == "" {
		port = defaultPort
	} else {
		port = *portFlag
	}

	currentDir, err := os.Getwd()
	if err != nil {
		log.Fatal(fmt.Errorf("failed to get current directory: %w", err))
	}

	address := fmt.Sprintf(":%s", port)
	server := &http.Server{
		Addr:              address,
		Handler:           http.FileServer(http.Dir(filepath.Join(currentDir, "e2e", "keplr"))),
		ReadTimeout:       5 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       120 * time.Second,
		ReadHeaderTimeout: 2 * time.Second,
	}

	serverErr := make(chan error, 1)

	// Start the keplr server in a goroutine
	go func() {
		log.Printf("Starting Keplr integration on http://127.0.0.1%s", address)
		if err = server.ListenAndServe(); err != nil {
			serverErr <- fmt.Errorf("failed to start Keplr integration server: %w", err)
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Block until a quit signal or error is received
	select {
	case <-sigCh:
		log.Println("Shutting down Keplr integration server...")

		// Create a deadline to wait for the server to shut down gracefully
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()

		if err = server.Shutdown(ctx); err != nil {
			log.Printf("Server forced to shutdown: %v", err)
		}
		log.Println("Server shutdown complete")
	case err = <-serverErr:
		log.Printf("Keplr integration server error: %v", err)
	}
}
