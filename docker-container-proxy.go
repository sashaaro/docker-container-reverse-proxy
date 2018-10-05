package main

import (
	"context"
	"fmt"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/client"
	"github.com/google/tcpproxy"
	"log"
	"net"
	"os"
	"strings"
	"time"
)

type ContainerProxy struct {
	networkName string
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

func (this *ContainerProxy) loadContainers () {
	fmt.Println("Updating containers info")
	containers, err := this.cli.ContainerList(context.Background(), types.ContainerListOptions{})
	if err != nil {
		fmt.Printf(err.Error());
	} else {
		// fmt.Println("aliases %s", len(containers[0].NetworkSettings.Networks["my_network"].Aliases))

		this.containers = containers

		// get aliases and fill
		for _, container := range this.containers {
			if network, ok := container.NetworkSettings.Networks[this.networkName]; ok {
				con, _ := this.cli.ContainerInspect(context.Background(), container.ID)
				network.Aliases = con.NetworkSettings.Networks[this.networkName].Aliases
			}
		}
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
	targetPort string
}

func (ch *ContainerByAliasesTarget) HandleConn(conn net.Conn)  {
	wrap, ok := conn.(*tcpproxy.Conn);
	if !ok {
		return
	}
	if wrap.HostName == "" {
		return;
	}

	for _, container := range ch.containerProxy.containers {
		for _, network := range container.NetworkSettings.Networks {
			for _, alias := range network.Aliases {
				if strings.Index(wrap.HostName, alias) == 0 { //with any port
					hNetwork, ok := container.NetworkSettings.Networks[container.HostConfig.NetworkMode]
					if ok { // sometime while "network:disconnect" event fire
						if hNetwork.IPAddress != "" {
							addr := fmt.Sprintf("%s:%s", hNetwork.IPAddress, ch.targetPort)
							fmt.Printf("Forwarding %s to %s\n", wrap.HostName, addr)
							dp := &tcpproxy.DialProxy{Addr: addr}
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
		return strings.Index(got, domain) != -1
	}
}

func (this *ContainerProxy) start (port string, targetPort string, postfixDomain string) {
	var p tcpproxy.Proxy

	dContainersTarget := &ContainerByAliasesTarget{containerProxy: this, targetPort: targetPort}
	p.AddHTTPHostMatchRoute(fmt.Sprintf(":%s", port), withPostfixDomain(postfixDomain), dContainersTarget)
	log.Fatal(p.Run())
}

func main() {
	if len(os.Args) < 5 {
		fmt.Println("Pass 4 arguments in this order: port targetPort dockerNetowrkName postfixDomain.")
		return;
	}
	port := os.Args[1]
	targetPort := os.Args[2]
	networkName := os.Args[3]
	postfixDomain := os.Args[4]
	fmt.Println(fmt.Sprintf(":%s", port))

	containerProxy := ContainerProxy{
		networkName: networkName,
	}

	containerProxy.createClient()
	containerProxy.loadContainers()
	go func() {
		containerProxy.listen()
	}()

	containerProxy.start(port, targetPort, postfixDomain)
}