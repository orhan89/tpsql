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

const localHost = "127.0.0.1"
const localPort = 5432

type Tunnel interface {
	Connect([]string) error
	Close() error
	Flags()
}

type K8sTunnel struct {
	namespace string
	resourceType string
	resourceName string
	remotePort int

	readyChan chan struct{}
	stopChan chan struct{}
}

func (s *K8sTunnel) Connect(args []string) error {
	var kubeconfig string

	home := homedir.HomeDir()
	kubeconfig = filepath.Join(home, ".kube", "config")

	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return err
	}
	roundTripper, upgrader, err := spdy.RoundTripperFor(config)
	if err != nil {
		return err
	}

	hostURL, err := url.Parse(config.Host)
	if err != nil {
		return err
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

	ports := []string{fmt.Sprintf("%d:%d", localPort, s.remotePort)}
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
	remoteHost string
	remoteUser string

	cmd *exec.Cmd
}

func (s *SSHTunnel) Connect(args []string) error {
	postgresHost := "127.0.0.1"

	if i := slices.Index(args, "--host"); i != -1 {
		postgresHost = args[i+1]
	}

	postgresPort := 5432

	if i := slices.Index(args, "--port"); i != -1 {
		if v, err := strconv.Atoi(args[i+1]); err != nil {
			log.Fatal("Failed in parsing server port")
			return err
		} else {
			postgresPort = v
		}
	}

	portForwardingAddress := fmt.Sprintf("%s:%d:%s:%d", localHost, localPort, postgresHost, postgresPort)

	tunnelAddress := fmt.Sprintf("%s@%s", s.remoteUser, s.remoteHost)

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

	log.Print("Opening SSH tunnel")
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

var tunnelMap = map[string]Tunnel{
	"ssh": &SSHTunnel{},
	"k8s": &K8sTunnel{},
}

var tunnel Tunnel

func main() {
	tunnelType := flag.String("tunnelType", "ssh", "the type of the tunnel (default=ssh)")

	for _, v := range tunnelMap {
		v.Flags()
	}

	flag.Parse()

	tunnel, ok := tunnelMap[*tunnelType]
	if !ok {
		log.Printf("There is no implementation for %s tunnel type", *tunnelType)
		return
	}

	psqlArgs := flag.Args()

	log.Printf("Connecting to %s tunnel", *tunnelType)
	err := tunnel.Connect(psqlArgs)
	if err != nil {
		log.Printf("Error connecting to tunnel: %s", err)
		return
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

	if i := slices.Index(psqlArgs, "--host"); i != -1 {
		psqlArgs = slices.Delete(psqlArgs, i, i+2)
	}

	psqlArgs = slices.Concat([]string{"--host", localHost}, psqlArgs)

	if i := slices.Index(psqlArgs, "--port"); i != -1 {
		psqlArgs = slices.Delete(psqlArgs, i, i+2)
	}

	psqlArgs = slices.Concat([]string{"--port", strconv.Itoa(localPort)}, psqlArgs)


	log.Print("Start psql")
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
