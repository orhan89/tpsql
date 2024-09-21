package main

import (
	"bytes"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"syscall"
	"time"

	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
	"k8s.io/client-go/util/homedir"
)

type Tunnel interface {
	Connect(string, int) error
	Close() error
	Flags()
}

type K8sTunnel struct {
	localHost string
	localPort int
	namespace string
	resourceType string
	resourceName string
	remotePort int
	readyChan chan struct{}
	stopChan chan struct{}
}

func (s *K8sTunnel) Connect(postgresHost string, postgresPort int) error {
	var kubeconfig string

	home := homedir.HomeDir()
	kubeconfig = filepath.Join(home, ".kube", "config")

	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		panic(err)
	}
	roundTripper, upgrader, err := spdy.RoundTripperFor(config)
	if err != nil {
		panic(err)
	}

	hostURL, err := url.Parse(config.Host)
	if err != nil {
		panic(err)
	}
	hostURL.Path = path.Join(
		"api", "v1",
		"namespaces", s.namespace,
		s.resourceType, s.resourceName,
		"portforward",
	)
	dialer := spdy.NewDialer(upgrader, &http.Client{Transport: roundTripper}, http.MethodPost, hostURL)

	s.stopChan, s.readyChan = make(chan struct{}, 1), make(chan struct{}, 1)
	out, errOut := new(bytes.Buffer), new(bytes.Buffer)

	s.localPort = 5432
	ports := []string{fmt.Sprintf("%d:%d", s.localPort, s.remotePort)}
	forwarder, err := portforward.New(dialer, ports, s.stopChan, s.readyChan, out, errOut)
	if err != nil {
		panic(err)
	}

	go func() {
		for range s.readyChan {
		}
		if len(errOut.String()) != 0 {
			panic(errOut.String())
		} else if len(out.String()) != 0 {
			log.Print(out.String())
		}
	}()

	go func() {
		if err = forwarder.ForwardPorts(); err != nil {
			panic(err)
		}
	}()

	return nil
}

func (s *K8sTunnel) Close() error {
	log.Print("Closing PortForward")
	close(s.stopChan)
	return nil
}

func (s *K8sTunnel) Flags() {
	flag.StringVar(&s.namespace, "k8sNamespace", "default", "kubernetes namespace")
	flag.StringVar(&s.resourceName, "k8sResourceName", "", "kubernetes resource to be port forwarded")
	flag.StringVar(&s.resourceType, "k8sResourceType", "pods", "kuberenetes types of resource to be port forwarded")
	flag.IntVar(&s.remotePort, "k8sRemotePort", 5432, "target port in kubernetes resource")
}

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

func (s *SSHTunnel) Connect(postgresHost string, postgresPort int) error {
	portForwardingAddress := fmt.Sprintf("%s:%d:%s:%d", s.localHost, s.localPort, postgresHost, postgresPort)
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

	return nil
}

func (s *SSHTunnel) Close() error {
	s.cmd.Process.Signal(syscall.SIGTERM)
	return nil
}

func (s *SSHTunnel) Flags() {
	flag.StringVar(&s.remoteUser, "sshUser", "root", "(ssh) tunnel user")
	flag.StringVar(&s.remoteHost, "sshHost", "127.0.0.1", "(ssh) tunnel host")
}

func main() {
	var tunnel Tunnel
	localHost := "127.0.0.1"
	localPort := 5432

	sshTunnel := &SSHTunnel{
		localHost: localHost,
		localPort: localPort,
	}
	sshTunnel.Flags()

	k8sTunnel := &K8sTunnel{}
	k8sTunnel.Flags()

	tunnelType := flag.String("tunnelType", "ssh", "the type of the tunnel (default=ssh)")

	postgresHost := "127.0.0.1"
	postgresPort := 5432

	flag.Parse()

	if *tunnelType == "ssh" {
		tunnel = sshTunnel
	} else if *tunnelType == "k8s" {
		tunnel = k8sTunnel
	}

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


	log.Print("Connecting to tunnel")
	err := tunnel.Connect(postgresHost, postgresPort)
	if err != nil {
		panic("error connecting to tunnel")
	}
	
	defer tunnel.Close()

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
		tunnel.Close()
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
