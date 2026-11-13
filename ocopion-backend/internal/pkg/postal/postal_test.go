package postal

import (
	"fmt"
	"io"
	"net"
	"net/smtp"
	"net/textproto"
	"strings"
	"testing"
	"time"
)

// Exercise the SMTP DATA acknowledgement without contacting a mail server.
func TestSendDataAcknowledgement(t *testing.T) {
	for _, status := range []string{"250 accepted", "554 message rejected"} {
		t.Run(status, func(t *testing.T) {
			clientConn, serverConn := net.Pipe()
			defer clientConn.Close()
			defer serverConn.Close()
			if err := clientConn.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
				t.Fatal(err)
			}
			if err := serverConn.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
				t.Fatal(err)
			}
			done := make(chan error, 1)
			go func() {
				conn := textproto.NewConn(serverConn)
				if err := conn.PrintfLine("220 localhost SMTP"); err != nil {
					done <- err
					return
				}
				for _, step := range []struct{ command, reply string }{
					{"DATA", "354 send message"},
				} {
					line, err := conn.ReadLine()
					if err != nil {
						done <- err
						return
					}
					if !strings.HasPrefix(line, step.command) {
						done <- fmt.Errorf("unexpected command: %s", line)
						return
					}
					if err := conn.PrintfLine("%s", step.reply); err != nil {
						done <- err
						return
					}
				}
				if _, err := io.ReadAll(conn.DotReader()); err != nil {
					done <- err
					return
				}
				done <- conn.PrintfLine("%s", status)
			}()
			client, err := smtp.NewClient(clientConn, "localhost")
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			err = sendData(client, "Subject: test\r\n\r\n<p>test</p>")
			if strings.HasPrefix(status, "250") {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil || !strings.Contains(err.Error(), "554") {
				t.Fatalf("DATA rejection must be returned, got: %v", err)
			}
			if err := <-done; err != nil {
				t.Fatal(err)
			}
		})
	}
}
