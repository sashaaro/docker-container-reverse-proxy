package main

import (
	"bufio"
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
	containerProxy *ContainerProxy
}

func (ch *ContainerByAliasesTarget) HandleConn(conn net.Conn)  {
	request := &http.Request{}
	request.Write(bufio.NewWriter(conn))
	// httpHostHeader
	// fmt.Printf("Host: %s \n", request.Host)

	for _, container := range ch.containerProxy.containers {
		for _, network := range container.NetworkSettings.Networks {
			for _, alias := range network.Aliases {
				if request.Host == alias {
					hNetwork, ok := container.NetworkSettings.Networks[container.HostConfig.NetworkMode]
					if ok { // sometime while "network:disconnect" event fire
						if hNetwork.IPAddress != "" {
							addr := fmt.Sprintf("%s:80", hNetwork.IPAddress)
							dp := &tcpproxy.DialProxy{Addr: addr}
							// dp.Addr =
							dp.HandleConn(conn)
							return
						} else {
							// log
						}
					} else {
						// return nil, fmt.Errorf("unable to find network settings for the network %s", networkMode)
					}
				}
			}
		}
	}
}

func withPostfixDomain(domain string) tcpproxy.Matcher {
	return func(_ context.Context, got string) bool {
		fmt.Printf("Get: %s \n", got)
		return strings.Index(got, domain) != -1
	}
}

func (this ContainerProxy) start () {
	var p tcpproxy.Proxy
	/*for _, container := range this.containers {
		container
	}*/
	//p.AddRoute(":80", &ContainerByAliasesTarget{})
	target := &ContainerByAliasesTarget{containerProxy: &this}
	p.AddHTTPHostMatchRoute(":80", withPostfixDomain(".skyeng.loc"), target)
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