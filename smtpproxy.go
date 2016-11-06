package main

import (
	"fmt"
	"net"
	"os"

	"github.com/jorgenschaefer/smtpproxy/argerror"
	"github.com/jorgenschaefer/smtpproxy/config"
	"github.com/jorgenschaefer/smtpproxy/proxy"
	"github.com/jorgenschaefer/smtpproxy/smtpd"
)

func main() {
	ln, err := listen()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	fmt.Printf("SMTP proxy started; address=\"%s\"\n", ln.Addr())
	defer fmt.Printf("SMTP proxy stopped; address=\"%s\"\n", ln.Addr())

	for {
		conn, err := ln.Accept()
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		go handleConnection(smtpd.NewConnection(conn))
	}
}

func listen() (net.Listener, error) {
	if config.ListenMode() == "address" {
		return net.Listen("tcp", config.ListenAddress())
	} else {
		f := os.NewFile(config.ListenFD(), "LISTEN_FD")
		defer f.Close()
		return net.FileListener(f)
	}
}

func handleConnection(conn smtpd.Connection) {
	defer conn.Close()
	fmt.Println(argerror.New("New connection",
		map[string]string{"client": conn.RemoteAddr().String()}))
	defer fmt.Println(argerror.New("Connection finished",
		map[string]string{"client": conn.RemoteAddr().String()}))
	state, err := proxy.Greet(conn)
	if err != nil {
		fmt.Println(err.Error())
		maybeTarpit(err, conn)
		return
	}
	for {
		if err := state.HandleCommand(); err != nil {
			fmt.Println(err.Error())
			maybeTarpit(err, conn)
			return
		}
	}
}

func maybeTarpit(err error, conn smtpd.Connection) {
	_, ok := err.(proxy.TarpitError)
	if ok {
		args := map[string]string{
			"client": conn.RemoteAddr().String(),
		}
		bytesread, duration, err := conn.Tarpit()
		args["bytesread"] = fmt.Sprintf("%d", bytesread)
		args["duration"] = duration.String()
		args["error"] = err.Error()
		fmt.Println(argerror.New("Client escaped tarpit", args))
	}
}
