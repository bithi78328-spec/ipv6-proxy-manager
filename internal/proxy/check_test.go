package proxy

import (
	"bufio"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"testing"
	"time"

	"ipv6-proxy-manager/internal/model"
)

func TestCheckerValidatesAuthenticationAndEgress(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	serverErr := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			serverErr <- err
			return
		}
		defer conn.Close()
		greeting := make([]byte, 3)
		if _, err := io.ReadFull(conn, greeting); err != nil {
			serverErr <- err
			return
		}
		conn.Write([]byte{0x05, 0x02})
		head := make([]byte, 2)
		io.ReadFull(conn, head)
		username := make([]byte, int(head[1]))
		io.ReadFull(conn, username)
		length := []byte{0}
		io.ReadFull(conn, length)
		password := make([]byte, int(length[0]))
		io.ReadFull(conn, password)
		if string(username) != "user1" || string(password) != "pass1" {
			serverErr <- fmt.Errorf("unexpected credentials")
			return
		}
		conn.Write([]byte{0x01, 0x00})
		request := make([]byte, 5)
		io.ReadFull(conn, request)
		host := make([]byte, int(request[4]))
		io.ReadFull(conn, host)
		port := make([]byte, 2)
		io.ReadFull(conn, port)
		if string(host) != "test.invalid" || binary.BigEndian.Uint16(port) != 80 {
			serverErr <- fmt.Errorf("unexpected connect target")
			return
		}
		conn.Write([]byte{0x05, 0x00, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
		reader := bufio.NewReader(conn)
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				serverErr <- err
				return
			}
			if line == "\r\n" {
				break
			}
		}
		fmt.Fprint(conn, "HTTP/1.1 200 OK\r\nContent-Length: 11\r\n\r\n2001:db8::1")
		serverErr <- nil
	}()
	port := listener.Addr().(*net.TCPAddr).Port
	checker := &Checker{EndpointHost: "test.invalid", EndpointPort: 80, EndpointPath: "/", Timeout: 2 * time.Second}
	err = checker.Check(context.Background(), model.Proxy{Host: "127.0.0.1", Port: port, IPv6: "2001:db8::1", Username: "user1", Password: "pass1", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := <-serverErr; err != nil {
		t.Fatal(err)
	}
}
