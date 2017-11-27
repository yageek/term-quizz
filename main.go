package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"os"

	"golang.org/x/crypto/ssh"
)

var (
	sshPort   = flag.Int("sshPort", 2022, "The ssh port to use")
	sshConfig = &ssh.ServerConfig{
		NoClientAuth: true,
	}
	keyPath = flag.String("key", "", "The path to the SSH key")
	server  *QuizzServer
)

func main() {
	flag.Parse()

	// Configure future SSH configuration
	if *keyPath == "" {
		log.Printf("An usable key shpuld be provided")
		flag.PrintDefaults()
		os.Exit(-1)
	}

	// Decodes keys
	privateBytes, err := ioutil.ReadFile(*keyPath)
	if err != nil {
		log.Fatalln("Failed to load private key: ", err)
	}

	private, err := ssh.ParsePrivateKey(privateBytes)
	if err != nil {
		log.Fatalln("Failed to parse private key: ", err)
	}

	sshConfig.AddHostKey(private)

	// Stars a new quizz
	server = NewQuizzServer()

	quizz := &Quizz{
		Title:      "Some Core Data",
		Difficulty: 100,
	}
	server.SetQuizz(quizz)
	server.Run()

	// Starts TCP connection
	address := fmt.Sprintf("0.0.0.0:%d", *sshPort)
	fmt.Printf("Starting server at %s ... \n", address)
	listener, err := net.Listen("tcp", address)
	if err != nil {
		log.Fatalln("Impossible to start connection")
	}

	for {
		if newConnection, err := listener.Accept(); err != nil {
			log.Fatalln("Impossible to accept incomming connection")
		} else {
			go handleNewConnection(newConnection)
		}

	}
}

func handleNewConnection(conn net.Conn) {

	sshConn, chans, reqs, err := ssh.NewServerConn(conn, sshConfig)
	if err != nil {
		log.Fatalf("Impossible to start an ssh connection for %s: %v \n", conn.RemoteAddr(), err)
	}

	// See documentation
	go ssh.DiscardRequests(reqs)

	for newChannel := range chans {

		if newChannel.ChannelType() != "session" {
			newChannel.Reject(ssh.UnknownChannelType, "unknown channel type")
			continue
		}

		channel, requests, err := newChannel.Accept()
		if err != nil {
			log.Fatalf("Could not accept channel: %v", err)
		}

		// Reject all out of band requests accept for the unix defaults, pty-req and
		// shell.
		go func(in <-chan *ssh.Request) {
			for req := range in {

				username := usernameFromConn(sshConn)

				switch req.Type {
				case "pty-req":

					fmt.Printf("New session for: %s\n", username)
					ptyRequest := ptyRequestMsg{}
					if err := ssh.Unmarshal(req.Payload, &ptyRequest); err != nil {
						fmt.Printf("New session err %s: %v \n", username, err)
						req.Reply(false, nil)
						continue
					}

					startNewSession(channel, username, 30, 80)
					req.Reply(true, nil)

				case "window-change":
					request := windowsRequestMsg{}
					if err := ssh.Unmarshal(req.Payload, &request); err != nil {
						fmt.Printf("Windows update err %s: %v \n", username, err)
					} else {
						// server.UpdateWindowSize(username, int(request.RowsHeight), int(request.ColumnsWidth))
					}
					req.Reply(false, nil)
				default:
					fmt.Println("Default request:", req.Type)
					req.Reply(true, nil)
				}
			}
		}(requests)

	}
}

func usernameFromConn(conn *ssh.ServerConn) string {
	return fmt.Sprintf("%s_%s", conn.User(), conn.RemoteAddr().String())
}

func startNewSession(channel ssh.Channel, username string, rows, columns uint32) {
	go server.HandleUserConnection(channel, username, int(rows), int(columns))
}

// https://github.com/golang/crypto/blob/b080dc9a8c480b08e698fb1219160d598526310f/ssh/session.go#L179
type ptyRequestMsg struct {
	Term     string
	Columns  uint32
	Rows     uint32
	Width    uint32
	Height   uint32
	Modelist string
}

type windowsRequestMsg struct {
	ColumnsWidth uint32
	RowsHeight   uint32
	PixelWidth   uint32
	PixelHeight  uint32
}
