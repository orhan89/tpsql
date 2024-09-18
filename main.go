package main

import (
	"log"
	"os"
	"os/exec"
)

func main() {
	log.Print("Searching for psql binary")
	_, err := exec.LookPath("psql")
	if err != nil {
		log.Fatal("psql binary is not available in PATH")
	}

	cmd := exec.Command("psql", "postgres")
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err = cmd.Run()
	if err != nil {
		log.Fatal(err)
	}
	log.Print("Exiting")
}
