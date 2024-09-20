package main

import (
	"errors"
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

type SSHTunnel struct {
	localHost string
	localPort int
	remoteHost string
	remotePort int
	remoteUser string
	postgresHost string
	postgresPort int

	cmd *exec.Cmd
}

func (s *SSHTunnel) Connect() error {
	portForwardingAddress := fmt.Sprintf("%s:%d:%s:%d", s.localHost, s.localPort, s.postgresHost, s.postgresPort)
	tunnelAddress := fmt.Sprintf("%s@%s", s.remoteUser, s.remoteHost)
	log.Print(portForwardingAddress)
	log.Print(tunnelAddress)

	log.Print("Searching for ssh binary")
	_, err := exec.LookPath("ssh")
	if err != nil {
		log.Print("ssh binary is not available in PATH")
		return err
	}

	s.cmd = exec.Command("ssh", "-N", "-L", portForwardingAddress, tunnelAddress)
	s.cmd.Stdin = os.Stdin
	s.cmd.Stdout = os.Stdout
	s.cmd.Stderr = os.Stderr

	log.Print("Opening tunnel")
	err = s.cmd.Start()
	if err != nil {
		log.Print(err)
		return err
	}

	log.Print("Waiting until tunnel is open")

	address := net.JoinHostPort(s.localHost, strconv.Itoa(s.localPort))

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
		s.Close()
		return errors.New("Timeout")
	}

	return nil
}

func (s *SSHTunnel) Close() error {
	s.cmd.Process.Signal(syscall.SIGTERM)
	return nil
}

func main() {
	localHost := "127.0.0.1"
	localPort := 5432
	// tunnelType := flag.String("tunnelType", "ssh", "the type of the tunnel (default=ssh)")
	tunnelHost := flag.String("sshHost", "127.0.0.1", "(ssh) tunnel host")
	tunnelUser := flag.String("sshUser", "root", "(ssh) tunnel user")
	postgresHost := "127.0.0.1"
	postgresPort := 5432

	flag.Parse()
	psqlArgs := flag.Args()

	if i := slices.Index(psqlArgs, "--host"); i != -1 {
		postgresHost = psqlArgs[i+1]
		log.Printf("Server Host: %s", postgresHost)

		psqlArgs = slices.Delete(psqlArgs, i, i+2)
	}

	psqlArgs = slices.Concat([]string{"--host", localHost}, psqlArgs)

	if i := slices.Index(psqlArgs, "--port"); i != -1 {
		postgresPort, err := strconv.Atoi(psqlArgs[i+1])
		if err != nil {
			log.Print("Failed in parsing server port")
			return
		}
		log.Printf("Server Port: %d", postgresPort)

		psqlArgs = slices.Delete(psqlArgs, i, i+2)
	}

	psqlArgs = slices.Concat([]string{"--port", strconv.Itoa(localPort)}, psqlArgs)

	tunnel := &SSHTunnel{
		localHost: localHost,
		localPort: localPort,
		remoteHost: *tunnelHost,
		remoteUser: *tunnelUser,
		postgresHost: postgresHost,
		postgresPort: postgresPort,
	}

	err := tunnel.Connect()
	if err != nil {
		panic("error connecting to tunnel")
	}
	
	defer tunnel.Close()
	
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
