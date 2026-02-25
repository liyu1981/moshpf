package protocol

import (
	"encoding/gob"
	"os"
	"strconv"
)

const Version = "0.1.0"

type Message interface{}

type Hello struct {
	Version string
}

type HelloAck struct {
	Version string
}

type ForwardRequest struct {
	ID   uint32
	Host string
	Port uint16
}

type ForwardAck struct {
	ID uint32
}

type ForwardErr struct {
	ID     uint32
	Reason string
}

type ListenRequest struct {
	LocalAddr  string
	RemoteHost string
	RemotePort uint16
}

type Heartbeat struct{}

type HeartbeatAck struct{}

type Shutdown struct {
	Reason string
}

func Register() {
	gob.Register(Hello{})
	gob.Register(HelloAck{})
	gob.Register(ForwardRequest{})
	gob.Register(ForwardAck{})
	gob.Register(ForwardErr{})
	gob.Register(ListenRequest{})
	gob.Register(Heartbeat{})
	gob.Register(HeartbeatAck{})
	gob.Register(Shutdown{})
}

func GetUnixSocketPath() string {
	return "/tmp/moshpf-" + strconv.Itoa(os.Getuid()) + ".sock"
}
