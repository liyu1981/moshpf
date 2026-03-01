package agent

import (
	"testing"

	"github.com/liyu1981/moshpf/pkg/protocol"
)

func TestAgentHandleMessage(t *testing.T) {
	a := &Agent{
		listChan:   make(chan protocol.ListResponse, 1),
		listenChan: make(chan protocol.ListenResponse, 1),
		closeChan:  make(chan protocol.CloseResponse, 1),
	}

	// Test ListResponse
	listResp := protocol.ListResponse{
		Entries: []protocol.ForwardEntry{
			{LocalAddr: ":8080", RemotePort: 80},
		},
	}
	a.handleMessage(nil, listResp)
	select {
	case resp := <-a.listChan:
		if len(resp.Entries) != 1 {
			t.Errorf("Expected 1 entry, got %d", len(resp.Entries))
		}
	default:
		t.Error("ListResponse not received on channel")
	}

	// Test ListenResponse
	listenResp := protocol.ListenResponse{Success: true, RemotePort: 1234}
	a.handleMessage(nil, listenResp)
	select {
	case resp := <-a.listenChan:
		if resp.RemotePort != 1234 {
			t.Errorf("Expected port 1234, got %d", resp.RemotePort)
		}
	default:
		t.Error("ListenResponse not received on channel")
	}

	// Test CloseResponse
	closeResp := protocol.CloseResponse{Success: true, Port: 5678}
	a.handleMessage(nil, closeResp)
	select {
	case resp := <-a.closeChan:
		if resp.Port != 5678 {
			t.Errorf("Expected port 5678, got %d", resp.Port)
		}
	default:
		t.Error("CloseResponse not received on channel")
	}
}
