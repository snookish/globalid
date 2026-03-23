package main

import (
	"fmt"
	"log"
	"time"

	"github.com/snookish/globalid"
)

func main() {
	gen, err := globalid.NewGenerator(globalid.Config{
		WaitForTime:   true,
		AutoMachineID: true,
	})
	if err != nil {
		log.Fatal(err)
	}

	id, err := gen.Generate()
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Generated ID: %d\n", id.ID())
	fmt.Printf("Human readable: %s\n", id.GoString())

	timestamp, machineID, sequence := globalid.ParseID(*id)
	fmt.Printf("Timestamp: %s, Machine: %d, Sequence: %d\n", timestamp.Format(time.RFC3339), machineID, sequence)
}
