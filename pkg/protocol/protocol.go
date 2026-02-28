package protocol

import (
	"encoding/gob"
	"net"
	"os"
	"strconv"
)

type Message interface{}

type Hello struct {
	Version string
}

type HelloAck struct {
	Version string
	UDPPort uint16
	TLSHash string
}

type StreamHeader struct {
	Host string
	Port uint16
}

type ListenRequest struct {
	LocalAddr  string
	RemoteHost string
	RemotePort uint16
}

type ListenResponse struct {
	RemotePort uint16
	Success    bool
	Reason     string
}

type ListRequest struct{}

type ForwardEntry struct {
	LocalAddr  string
	RemoteHost string
	RemotePort uint16
	Transport  string
	Error      string
}

type ListResponse struct {
	Entries  []ForwardEntry
	MasterIP string
}

type CloseRequest struct {
	Port uint16
}

type CloseResponse struct {
	Port    uint16
	Success bool
	Reason  string
}

type Heartbeat struct{}

type HeartbeatAck struct{}

type Shutdown struct {
	Reason string
}

func Register() {
	gob.Register(Hello{})
	gob.Register(HelloAck{})
	gob.Register(StreamHeader{})
	gob.Register(ListenRequest{})
	gob.Register(ListenResponse{})
	gob.Register(ListRequest{})
	gob.Register(ListResponse{})
	gob.Register(ForwardEntry{})
	gob.Register(CloseRequest{})
	gob.Register(CloseResponse{})
	gob.Register(Heartbeat{})
	gob.Register(HeartbeatAck{})
	gob.Register(Shutdown{})
}

func GetUnixSocketPath() string {
	return "/tmp/mpf-" + strconv.Itoa(os.Getuid()) + ".sock"
}

func GetLocalIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "127.0.0.1"
	}
	for _, address := range addrs {
		if ipnet, ok := address.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				return ipnet.IP.String()
			}
		}
	}
	return "127.0.0.1"
}
