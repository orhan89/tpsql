package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"syscall"
	"time"
)

func main() {
	localHost := "127.0.0.1"
	localPort := 5432
	tunnelHost := flag.String("sshHost", "127.0.0.1", "(ssh) tunnel host")
	tunnelUser := flag.String("sshUser", "root", "(ssh) tunnel user")
	serverHost := "127.0.0.1"
	serverPort := 5432

	flag.Parse()
	psqlArgs := flag.Args()

	if i := slices.Index(psqlArgs, "--host"); i != -1 {
		serverHost = psqlArgs[i+1]
		log.Printf("Server Host: %s", serverHost)

		psqlArgs = slices.Delete(psqlArgs, i, i+2)
	}

	psqlArgs = slices.Concat([]string{"--host", localHost}, psqlArgs)

	if i := slices.Index(psqlArgs, "--port"); i != -1 {
		serverPort, err := strconv.Atoi(psqlArgs[i+1])
		if err != nil {
			log.Print("Failed in parsing server port")
			return
		}
		log.Printf("Server Port: %d", serverPort)

		psqlArgs = slices.Delete(psqlArgs, i, i+2)
	}

	psqlArgs = slices.Concat([]string{"--port", strconv.Itoa(localPort)}, psqlArgs)

	portForwardingAddress := fmt.Sprintf("%s:%d:%s:%d", localHost, localPort, serverHost, serverPort)
	tunnelAddress := fmt.Sprintf("%s@%s", *tunnelUser, *tunnelHost)
	log.Print(portForwardingAddress)
	log.Print(tunnelAddress)

	log.Print("Searching for ssh binary")
	_, err := exec.LookPath("ssh")
	if err != nil {
		log.Print("ssh binary is not available in PATH")
		return
	}

	cmd := exec.Command("ssh", "-N", "-L", portForwardingAddress, tunnelAddress)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	log.Print("Opening tunnel")
	err = cmd.Start()
	if err != nil {
		log.Print(err)
		return
	}

	defer cmd.Process.Signal(syscall.SIGTERM)

	log.Print("Waiting until tunnel is open")

	address := net.JoinHostPort(localHost, strconv.Itoa(localPort))

	connected := false
	for i := 0; i < 10; i++ {
		_, err = net.Dial("tcp", address)
		if err == nil {
			log.Print("Tunnel is opened")
			connected = true
			break
		}
		time.Sleep(time.Second)
	}

	if connected == false {
		log.Print("Tunnel connection timeout. Exiting.")
		return
	}

	log.Print("Searching for psql binary")
	_, err = exec.LookPath("psql")
	if err != nil {
		log.Print("psql binary is not available in PATH")
		return
	}

	log.Print("Start Psql")
	log.Printf("Arguments for psql command: %v", psqlArgs)
	cmdPsql := exec.Command("psql", psqlArgs...)
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
