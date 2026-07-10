package main

import (
	"flag"
	"fmt"
	"log"

	"sophonie/sono/internal/config"
	"sophonie/sono/internal/server"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:8420", "server listen address")
	flag.Parse()

	cfg, err := config.New()
	if err != nil {
		log.Fatal(err)
	}

	srv, err := server.New(cfg)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("sono started on http://%s\n", *addr)
	if err := srv.ListenAndServe(*addr); err != nil {
		log.Fatal(err)
	}
}
