package main

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/client"
	"github.com/google/tcpproxy"
	"log"
	"net"
	"net/http"
	"strings"
	"time"
)

type ContainerProxy struct {
	containers []types.Container
	cli *client.Client
}



func (this *ContainerProxy) createClient () {
	cli, err := client.NewClientWithOpts(client.WithVersion("1.38"))

	if err != nil {
		panic(err)
		return;
	}
	this.cli = cli

}


func (this ContainerProxy) loadContainers () {
	fmt.Println("loadContainers")
	containers, err := this.cli.ContainerList(context.Background(), types.ContainerListOptions{})
	if err != nil {
		fmt.Printf(err.Error());
	} else {
		this.containers = containers
	}
}

func debounce(events <-chan events.Message, d time.Duration, callback func()) {
	var timer *time.Timer
	for _ = range events {
		go func() {
			if timer == nil {
				timer = time.NewTimer(d)
				<-timer.C
				timer.Stop()
				timer = nil
				go callback()
			}
		}()
	}
}

func (this ContainerProxy) listen () {
	events, _ := this.cli.Events(context.Background(), types.EventsOptions{})
	debounce(events, 10 * time.Second, func() {
		this.loadContainers()
	})
}

type ContainerByAliasesTarget struct {
}

func (ch *ContainerByAliasesTarget) HandleConn(conn net.Conn)  {
	dp := &tcpproxy.DialProxy{}
	host := httpHostHeader(bufio.NewReader(conn))
	fmt.Printf("Host: %s", host)
	dp.Addr = "172.17.0.2:80"
	dp.HandleConn(conn)
}

func equalsAliasDomain(domain string) tcpproxy.Matcher {
	return func(_ context.Context, got string) bool {
		fmt.Printf("Get: %s", got)
		return strings.Index(got, domain) != -1
	}
}

func (this ContainerProxy) start () {
	var p tcpproxy.Proxy
	/*for _, container := range this.containers {
		container
	}*/
	//p.AddRoute(":80", &ContainerByAliasesTarget{})
	p.AddHTTPHostMatchRoute(":80", equalsAliasDomain(".skyeng.loc"), &ContainerByAliasesTarget{})
	log.Fatal(p.Run())
}

func main() {
	containerProxy := ContainerProxy{}

	containerProxy.createClient()
	containerProxy.loadContainers()
	go func() {
		containerProxy.listen()
	}()
	containerProxy.start()
}



// httpHostHeader returns the HTTP Host header from br without
// consuming any of its bytes. It returns "" if it can't find one.
func httpHostHeader(br *bufio.Reader) string {
	const maxPeek = 4 << 10
	peekSize := 0
	for {
		peekSize++
		if peekSize > maxPeek {
			b, _ := br.Peek(br.Buffered())
			return httpHostHeaderFromBytes(b)
		}
		b, err := br.Peek(peekSize)
		if n := br.Buffered(); n > peekSize {
			b, _ = br.Peek(n)
			peekSize = n
		}
		if len(b) > 0 {
			if b[0] < 'A' || b[0] > 'Z' {
				// Doesn't look like an HTTP verb
				// (GET, POST, etc).
				return ""
			}
			if bytes.Index(b, crlfcrlf) != -1 || bytes.Index(b, lflf) != -1 {
				req, err := http.ReadRequest(bufio.NewReader(bytes.NewReader(b)))
				if err != nil {
					return ""
				}
				if len(req.Header["Host"]) > 1 {
					// TODO(bradfitz): what does
					// ReadRequest do if there are
					// multiple Host headers?
					return ""
				}
				return req.Host
			}
		}
		if err != nil {
			return httpHostHeaderFromBytes(b)
		}
	}
}

var (
	lfHostColon = []byte("\nHost:")
	lfhostColon = []byte("\nhost:")
	crlf        = []byte("\r\n")
	lf          = []byte("\n")
	crlfcrlf    = []byte("\r\n\r\n")
	lflf        = []byte("\n\n")
)

func httpHostHeaderFromBytes(b []byte) string {
	if i := bytes.Index(b, lfHostColon); i != -1 {
		return string(bytes.TrimSpace(untilEOL(b[i+len(lfHostColon):])))
	}
	if i := bytes.Index(b, lfhostColon); i != -1 {
		return string(bytes.TrimSpace(untilEOL(b[i+len(lfhostColon):])))
	}
	return ""
}

// untilEOL returns v, truncated before the first '\n' byte, if any.
// The returned slice may include a '\r' at the end.
func untilEOL(v []byte) []byte {
	if i := bytes.IndexByte(v, '\n'); i != -1 {
		return v[:i]
	}
	return v
}