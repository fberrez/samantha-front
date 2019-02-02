package main

import (
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/fberrez/samantha/backend"
	"github.com/fberrez/samantha/frontend"
	log "github.com/sirupsen/logrus"
)

func init() {
	env := os.Getenv("ENVIRONMENT")

	if env == "" || env == "DEV" {
		// Log as the default ASCII formatter.
		log.SetFormatter(&log.TextFormatter{})

		// Output to stdout instead of the default stderr
		log.SetOutput(os.Stdout)

		// Log all messages.
		log.SetLevel(log.DebugLevel)
	} else if env == "PROD" {
		// Log as JSON instead of the default ASCII formatter.
		log.SetFormatter(&log.JSONFormatter{})

		// Output to stdout instead of the default stderr
		log.SetOutput(os.Stdout)

		// Only log the warning severity or above.
		log.SetLevel(log.WarnLevel)
	}
}

func main() {
	// Initializes channels
	//
	// backendErrorChan is the channel handling error on the backend-side.
	// When an error occured, that is sent to the frontend and, then sent to
	// the frontend provider.
	backendErrorChan := make(chan []byte)

	// capsuleChan is the channel making the unidirectional connection between the
	// frontend and the backend. When a user input is received on the frontend-side
	// via a frontend provider, it is sent to the backend to be processed by a NLU
	// provider.
	capsuleChan := make(chan []byte)

	// Initializes frontend manager
	front, err := frontend.New(backendErrorChan, capsuleChan)
	if err != nil {
		panic(err)
	}

	back, err := backend.New(backendErrorChan, capsuleChan)
	if err != nil {
		panic(err)
	}

	// Initiliazes a new WaitGroup.
	wg := sync.WaitGroup{}

	// Starts the nlp client listening loop.
	wg.Add(1)
	go front.Start(&wg)
	wg.Add(1)
	go back.Start(&wg)

	// Initializes channel which handles SIGTERM and SIGINT
	quit := make(chan os.Signal)
	signal.Notify(quit, syscall.SIGTERM)
	signal.Notify(quit, syscall.SIGINT)
	// Wait for a SIGTERM or SIGINT
	<-quit

	// Closes channel
	close(backendErrorChan)
	close(capsuleChan)
	wg.Wait()

	log.Info("Graceful shutdown")
	os.Exit(0)
}
