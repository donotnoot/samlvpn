package main

import (
	"flag"
	"log"
)

var (
	flagConfigFile = flag.String("config", "", "config file")
)

func main() {
	flag.Parse()

	var s SAMLVPN
	if err := s.Configure(flagConfigFile); err != nil {
		log.Fatal(err)
	}

	if err := s.Connect(); err != nil {
		log.Fatal(err)
	}
}
