package clients

import (
	"dtail/clients/handlers"
	"dtail/clients/remote"
	"dtail/config"
	"dtail/omode"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"time"

	gossh "golang.org/x/crypto/ssh"
)

// HealthClient is used for health checking (e.g. via Nagios)
type HealthClient struct {
	// Client operating mode
	mode omode.Mode
	// The remote server address
	server string
	// SSH user name
	userName string
	// SSH auth methods to use to connect to the remote servers.
	sshAuthMethods []gossh.AuthMethod
}

// NewHealthClient returns a new healh client.
func NewHealthClient(mode omode.Mode) (*HealthClient, error) {
	c := HealthClient{
		mode:     mode,
		server:   fmt.Sprintf("%s:%d", config.Server.SSHBindAddress, config.Common.SSHPort),
		userName: config.ControlUser,
	}
	c.initSSHAuthMethods()

	return &c, nil
}

// Start the health client.
func (c *HealthClient) Start(wg *sync.WaitGroup) (status int) {
	defer wg.Done()
	receive := make(chan string)

	throttleCh := make(chan struct{}, runtime.NumCPU())
	statsCh := make(chan struct{}, 1)

	conn := remote.NewOneOffConnection(c.server, c.userName, c.sshAuthMethods)
	conn.Handler = handlers.NewHealthHandler(c.server, receive)
	conn.Commands = []string{c.mode.String()}

	go conn.Start(throttleCh, statsCh)
	defer conn.Stop()

	for {
		select {
		case data := <-receive:
			// Parse recieved data.
			s := strings.Split(data, "|")
			message := s[len(s)-1]
			if strings.HasPrefix(message, "done;") {
				return
			}

			// Set severity.
			s = strings.Split(message, ":")
			switch s[0] {
			case "OK":
			case "WARNING":
				if status < 1 {
					status = 1
				}
			case "CRITICAL":
				status = 2
			case "UNKNOWN":
				status = 3
			default:
				fmt.Printf("CRITICAL: Unexpected server response: '%s'\n", message)
				status = 2
				return
			}
			fmt.Print(message)

		case <-time.After(time.Second * 2):
			status = 2
			fmt.Println("CRITICAL: Could not communicate with DTail server")
			return
		}
	}
}

// Initialize SSH auth methods.
func (c *HealthClient) initSSHAuthMethods() {
	c.sshAuthMethods = append(c.sshAuthMethods, gossh.Password(config.ControlUser))
}
