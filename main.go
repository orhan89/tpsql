package main

import (
	"log"
	"os"
	"os/exec"
)

func main() {
	serverHost := flag.String("host", "127.0.0.1", "postgresql server host")
	serverPort := flag.Int("port", 5432, "postgresql server port")
	serverUser := flag.String("username", "postgres", "postgresql server user")
	serverDatabase := flag.String("dbname", "postgres", "postgresql database name")

	flag.Parse()
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

	log.Print("Searching for psql binary")
	_, err = exec.LookPath("psql")
	if err != nil {
		log.Print("psql binary is not available in PATH")
		return
	}

	log.Print("Start Psql")
	cmdPsql := exec.Command("psql", "-h", serverHost, "-p", strconv.Itoa(serverPort), "-U", *serverUser, *serverDatabase)
	cmdPsql.Stdin = os.Stdin
	cmdPsql.Stdout = os.Stdout
	cmdPsql.Stderr = os.Stderr

	err = cmdPsql.Run()
	if err != nil {
		log.Print(err)
		return
	}

	log.Print("Exiting")
}
